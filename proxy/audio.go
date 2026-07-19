package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/inja-online/llm-gateway/canonical"
	"github.com/inja-online/llm-gateway/config"
	googleegress "github.com/inja-online/llm-gateway/egress/google"
	openaiegress "github.com/inja-online/llm-gateway/egress/openai"
	"github.com/inja-online/llm-gateway/hooks"
	antingress "github.com/inja-online/llm-gateway/ingress/anthropic"
	oaingress "github.com/inja-online/llm-gateway/ingress/openai"
)

// OpenAI / Anthropic / Google voice endpoints (TTS + STT).
//
// Path disambiguation for /v1/audio/*:
//   - anthropic-version header present → Anthropic gateway dialect (errors + resolve)
//   - otherwise → OpenAI dialect
//
// Google native: POST /v1beta/models/{model}:generateSpeech (see audio_google.go).
//
// Fidelity: same-family TTS binary and multipart STT bodies are byte-passthrough
// (model JSON rewrite only). Re-encode only when translating dialects (e.g.
// Google generateContent JSON → OpenAI raw audio bytes via base64 decode only;
// no codec conversion).

// handleAudioSpeech serves POST /v1/audio/speech (TTS).
func (s *Server) handleAudioSpeech(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("anthropic-version") != "" {
		s.handleAnthropicAudioSpeech(w, r)
		return
	}
	s.handleOpenAIAudioSpeech(w, r)
}

// handleAudioTranscriptions serves POST /v1/audio/transcriptions (STT).
func (s *Server) handleAudioTranscriptions(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("anthropic-version") != "" {
		s.handleAnthropicAudioMultipart(w, r, openaiegress.TranscriptionsPath(), false)
		return
	}
	s.handleOpenAIAudioMultipart(w, r, openaiegress.TranscriptionsPath())
}

// handleAudioTranslations serves POST /v1/audio/translations (STT → English).
func (s *Server) handleAudioTranslations(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("anthropic-version") != "" {
		s.handleAnthropicAudioMultipart(w, r, openaiegress.TranslationsPath(), true)
		return
	}
	s.handleOpenAIAudioMultipart(w, r, openaiegress.TranslationsPath())
}

// handleOpenAIAudioSpeech serves OpenAI-dialect TTS.
func (s *Server) handleOpenAIAudioSpeech(w http.ResponseWriter, r *http.Request) {
	x := s.newExchange(w, r, DialectOpenAI, writeOpenAIError)
	x.ev.Modality = config.ModalityAudioSpeech
	x.ev.Transport = hooks.TransportHTTP
	defer x.emit()

	body, ok := x.readBody()
	if !ok {
		return
	}
	req, err := oaingress.ParseSpeechRequest(body)
	if err != nil {
		msg := err.Error()
		if ve, ok := err.(*oaingress.ValidationError); ok {
			msg = ve.Msg
		}
		x.fail(http.StatusBadRequest, "invalid_request_error", msg, hooks.StatusBadRequest)
		return
	}
	x.ev.Model = req.Model

	route, err := ResolveForModality(s.cfg, DialectOpenAI, req.Model, config.ModalityAudioSpeech)
	if err != nil {
		s.failRoute(x, err)
		return
	}
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = route.UpstreamModel
	x.ev.Media = speechMediaUsage(req)

	switch providerKind(route.Provider) {
	case config.KindOpenAI, config.KindOpenAICompat:
		s.openaiSpeechPassthrough(x, route, body, req)
	case config.KindGoogle:
		s.speechTranslateToGoogleBinary(x, route, req)
	default:
		x.fail(http.StatusNotImplemented, "invalid_request_error",
			"audio speech requires an openai, openai_compat, or google provider (got "+route.Provider.Kind+")",
			hooks.StatusBadRequest)
	}
}

// openaiSpeechPassthrough rewrites model and forwards JSON; response is raw audio.
func (s *Server) openaiSpeechPassthrough(x *exchange, route Route, body []byte, req *canonical.AudioSpeechRequest) {
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		// Rebuild from canonical if raw parse failed (should not happen after ParseSpeechRequest).
		up, err := openaiegress.BuildSpeechRequest(req, route.UpstreamModel)
		if err != nil {
			x.fail(http.StatusBadRequest, "invalid_request_error", "failed to build speech request", hooks.StatusBadRequest)
			return
		}
		body = up
	} else {
		m["model"] = route.UpstreamModel
		body, _ = json.Marshal(m)
	}
	resp, ok := x.sendUpstream(route, openaiegress.SpeechPath(), body)
	if !ok {
		return
	}
	defer resp.Body.Close()
	// Preserve Content-Type and raw body bytes (no re-encode).
	s.forwardMediaResponse(x, resp, speechMediaUsage(req))
}

// speechTranslateToGoogleBinary calls Gemini generateContent AUDIO and returns
// decoded audio bytes (base64 decode only — no codec re-encode).
func (s *Server) speechTranslateToGoogleBinary(x *exchange, route Route, req *canonical.AudioSpeechRequest) {
	upBody, err := googleegress.BuildSpeechGenerateContentRequest(req)
	if err != nil {
		x.fail(http.StatusBadRequest, CodeInvalidMediaRequest, err.Error(), hooks.StatusBadRequest)
		return
	}
	path := googleegress.SpeechGenerateContentPath(route.UpstreamModel)
	resp, ok := x.sendUpstream(route, path, upBody)
	if !ok {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		s.translateGoogleErrorToOpenAI(x, resp)
		return
	}
	respBody, err := readAll(resp)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to read upstream response", hooks.StatusUpstreamError)
		return
	}
	audio, err := googleegress.ParseSpeechGenerateContentResponse(respBody)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to parse upstream speech response: "+err.Error(), hooks.StatusUpstreamError)
		return
	}
	x.ev.Estimated = true
	x.ev.Status = hooks.StatusOK
	x.ev.HTTPStatus = http.StatusOK
	x.ev.Media = speechMediaUsage(req)
	ct := audio.MIMEType
	if ct == "" {
		ct = contentTypeForSpeechFormat(req.Format)
	}
	x.w.Header().Set("Content-Type", ct)
	x.w.WriteHeader(http.StatusOK)
	x.w.Write(audio.Data)
}

// handleOpenAIAudioMultipart forwards STT multipart (or rare JSON) bodies.
// Model is peeked best-effort for routing; multipart body bytes are not rewritten.
func (s *Server) handleOpenAIAudioMultipart(w http.ResponseWriter, r *http.Request, upstreamPath string) {
	x := s.newExchange(w, r, DialectOpenAI, writeOpenAIError)
	x.ev.Modality = config.ModalityAudioTranscribe
	x.ev.Transport = hooks.TransportHTTP
	defer x.emit()

	ct := r.Header.Get("Content-Type")
	body, ok := x.readBody()
	if !ok {
		return
	}

	model := ""
	if strings.Contains(ct, "multipart/") {
		model = peekMultipartModel(body)
	} else if strings.Contains(ct, "application/json") || ct == "" {
		var req map[string]any
		if json.Unmarshal(body, &req) == nil {
			model, _ = req["model"].(string)
		}
	}
	if model == "" {
		model = "unknown"
	}
	x.ev.Model = model

	route, err := Resolve(s.cfg, DialectOpenAI, model)
	if err != nil {
		// Fall back to openai dialect default (multipart may omit model).
		if def := s.cfg.Defaults.OpenAIDialect; def != "" {
			route = Route{ProviderName: def, Provider: s.cfg.Providers[def], UpstreamModel: model}
		} else {
			x.fail(http.StatusNotFound, "invalid_request_error", err.Error(), hooks.StatusBadRequest)
			return
		}
	}
	if err := CheckCapability(route.Provider, route.ProviderName, config.ModalityAudioTranscribe); err != nil {
		x.fail(http.StatusBadRequest, "unsupported_provider_capability", err.Error(), hooks.StatusBadRequest)
		return
	}
	if !isOpenAIFamily(route.Provider) {
		x.fail(http.StatusNotImplemented, "invalid_request_error",
			"audio transcription requires an openai or openai_compat provider (got "+route.Provider.Kind+")",
			hooks.StatusBadRequest)
		return
	}
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = route.UpstreamModel
	x.ev.Media = &hooks.MediaUsage{UnitKind: hooks.MediaUnitAudioMinute}

	// JSON path: rewrite model like speech when body is JSON.
	// Multipart: forward bytes and Content-Type boundary intact (no rebuild).
	upstreamBody := body
	sendCT := ct
	if !strings.Contains(ct, "multipart/") {
		var req map[string]any
		if json.Unmarshal(body, &req) == nil && model != "unknown" {
			req["model"] = route.UpstreamModel
			upstreamBody, _ = json.Marshal(req)
			sendCT = "application/json"
		}
	}

	resp, ok := x.sendUpstreamRaw(route, http.MethodPost, upstreamPath, upstreamBody, sendCT)
	if !ok {
		return
	}
	defer resp.Body.Close()
	s.forwardMediaResponse(x, resp)
}

// --- Anthropic gateway dialect ---

// handleAnthropicAudioSpeech serves Anthropic-shaped TTS (raw audio response).
func (s *Server) handleAnthropicAudioSpeech(w http.ResponseWriter, r *http.Request) {
	x := s.newExchange(w, r, DialectAnthropic, writeAnthropicError)
	x.ev.Modality = config.ModalityAudioSpeech
	x.ev.Transport = hooks.TransportHTTP
	defer x.emit()

	body, ok := x.readBody()
	if !ok {
		return
	}
	req, err := antingress.ParseSpeechRequest(body)
	if err != nil {
		msg := err.Error()
		if ve, ok := err.(*antingress.ValidationError); ok {
			msg = ve.Msg
		}
		x.fail(http.StatusBadRequest, CodeInvalidRequest, msg, hooks.StatusBadRequest)
		return
	}
	x.ev.Model = req.Model

	route, err := ResolveForModality(s.cfg, DialectAnthropic, req.Model, config.ModalityAudioSpeech)
	if err != nil {
		s.failRoute(x, err)
		return
	}
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = route.UpstreamModel
	x.ev.Media = speechMediaUsage(req)

	switch providerKind(route.Provider) {
	case config.KindOpenAI, config.KindOpenAICompat:
		s.anthropicSpeechToOpenAI(x, route, req)
	case config.KindGoogle:
		s.speechTranslateToGoogleBinary(x, route, req)
	default:
		x.fail(http.StatusBadRequest, CodeUnsupportedProviderCapability,
			fmt.Sprintf("provider %q (kind %s) does not support modality %q",
				route.ProviderName, route.Provider.Kind, config.ModalityAudioSpeech),
			hooks.StatusBadRequest)
	}
}

func (s *Server) anthropicSpeechToOpenAI(x *exchange, route Route, req *canonical.AudioSpeechRequest) {
	upBody, err := openaiegress.BuildSpeechRequest(req, route.UpstreamModel)
	if err != nil {
		x.fail(http.StatusBadRequest, CodeInvalidMediaRequest, "failed to build upstream speech request", hooks.StatusBadRequest)
		return
	}
	resp, ok := x.sendUpstream(route, openaiegress.SpeechPath(), upBody)
	if !ok {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		s.translateOpenAIErrorToAnthropic(x, resp)
		return
	}
	// Raw audio passthrough (no re-encode).
	s.forwardMediaResponse(x, resp, speechMediaUsage(req))
}

// handleAnthropicAudioMultipart serves Anthropic-dialect STT (multipart or JSON).
// Translates to OpenAI-family transcriptions/translations only.
func (s *Server) handleAnthropicAudioMultipart(w http.ResponseWriter, r *http.Request, upstreamPath string, translate bool) {
	x := s.newExchange(w, r, DialectAnthropic, writeAnthropicError)
	x.ev.Modality = config.ModalityAudioTranscribe
	x.ev.Transport = hooks.TransportHTTP
	defer x.emit()

	ct := r.Header.Get("Content-Type")
	body, ok := x.readBody()
	if !ok {
		return
	}

	model := ""
	if strings.Contains(ct, "multipart/") {
		model = peekMultipartModel(body)
	} else {
		req, err := antingress.ParseTranscribeJSONRequest(body)
		if err != nil {
			msg := err.Error()
			if ve, ok := err.(*antingress.ValidationError); ok {
				msg = ve.Msg
			}
			x.fail(http.StatusBadRequest, CodeInvalidRequest, msg, hooks.StatusBadRequest)
			return
		}
		model = req.Model
		_ = translate
	}
	if model == "" {
		model = "unknown"
	}
	x.ev.Model = model

	route, err := Resolve(s.cfg, DialectAnthropic, model)
	if err != nil {
		if def := s.cfg.Defaults.AnthropicDialect; def != "" {
			route = Route{ProviderName: def, Provider: s.cfg.Providers[def], UpstreamModel: model}
		} else if def := s.cfg.Defaults.OpenAIDialect; def != "" {
			// Prefer openai_dialect for audio when anthropic default is pure Messages host.
			route = Route{ProviderName: def, Provider: s.cfg.Providers[def], UpstreamModel: model}
		} else {
			x.fail(http.StatusNotFound, CodeInvalidRequest, err.Error(), hooks.StatusBadRequest)
			return
		}
	}
	if err := CheckCapability(route.Provider, route.ProviderName, config.ModalityAudioTranscribe); err != nil {
		x.fail(http.StatusBadRequest, CodeUnsupportedProviderCapability, err.Error(), hooks.StatusBadRequest)
		return
	}
	if !isOpenAIFamily(route.Provider) {
		x.fail(http.StatusBadRequest, CodeUnsupportedProviderCapability,
			fmt.Sprintf("audio transcription requires an openai or openai_compat provider (got %s); pure anthropic has no STT API",
				route.Provider.Kind),
			hooks.StatusBadRequest)
		return
	}
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = route.UpstreamModel
	x.ev.Media = &hooks.MediaUsage{UnitKind: hooks.MediaUnitAudioMinute}

	upstreamBody := body
	sendCT := ct
	if !strings.Contains(ct, "multipart/") {
		var m map[string]any
		if json.Unmarshal(body, &m) == nil && model != "unknown" {
			m["model"] = route.UpstreamModel
			upstreamBody, _ = json.Marshal(m)
			sendCT = "application/json"
		}
	}

	resp, ok := x.sendUpstreamRaw(route, http.MethodPost, upstreamPath, upstreamBody, sendCT)
	if !ok {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		s.translateOpenAIErrorToAnthropic(x, resp)
		return
	}
	s.forwardMediaResponse(x, resp)
}

func speechMediaUsage(req *canonical.AudioSpeechRequest) *hooks.MediaUsage {
	if req == nil {
		return &hooks.MediaUsage{UnitKind: hooks.MediaUnitAudioCharacter}
	}
	format := req.Format
	if format == "" {
		format = "mp3"
	}
	return &hooks.MediaUsage{
		Units:    utf8.RuneCountInString(req.Input),
		UnitKind: hooks.MediaUnitAudioCharacter,
		Format:   format,
	}
}

func contentTypeForSpeechFormat(format string) string {
	switch strings.ToLower(format) {
	case "mp3":
		return "audio/mpeg"
	case "opus":
		return "audio/opus"
	case "aac":
		return "audio/aac"
	case "flac":
		return "audio/flac"
	case "wav":
		return "audio/wav"
	case "pcm":
		return "audio/L16"
	default:
		if format != "" {
			return "audio/" + format
		}
		return "application/octet-stream"
	}
}
