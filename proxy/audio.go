package proxy

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

// OpenAI-family audio endpoints (TTS + STT). Passthrough-only: rewrite model
// (JSON) or best-effort multipart model peek for routing, auth, metering.
// Capability is fail-closed before any upstream call.

// handleAudioSpeech serves POST /v1/audio/speech (TTS; binary audio response).
func (s *Server) handleAudioSpeech(w http.ResponseWriter, r *http.Request) {
	x := s.newExchange(w, r, DialectOpenAI, writeOpenAIError)
	x.ev.Modality = config.ModalityAudioSpeech
	x.ev.Transport = hooks.TransportHTTP
	defer x.emit()

	body, ok := x.readBody()
	if !ok {
		return
	}
	var req map[string]any
	if json.Unmarshal(body, &req) != nil {
		x.fail(http.StatusBadRequest, "invalid_request_error", "request body is not valid JSON", hooks.StatusBadRequest)
		return
	}
	model, _ := req["model"].(string)
	if model == "" {
		x.fail(http.StatusBadRequest, "invalid_request_error", "missing or invalid required field: model", hooks.StatusBadRequest)
		return
	}
	x.ev.Model = model

	route, err := Resolve(s.cfg, DialectOpenAI, model)
	if err != nil {
		x.fail(http.StatusNotFound, "invalid_request_error", err.Error(), hooks.StatusBadRequest)
		return
	}
	if !isOpenAIFamily(route.Provider) {
		x.fail(http.StatusNotImplemented, "invalid_request_error",
			"audio speech requires an openai or openai_compat provider (got "+route.Provider.Kind+")",
			hooks.StatusBadRequest)
		return
	}
	if err := CheckCapability(route.Provider, route.ProviderName, config.ModalityAudioSpeech); err != nil {
		x.fail(http.StatusBadRequest, "unsupported_provider_capability", err.Error(), hooks.StatusBadRequest)
		return
	}
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = route.UpstreamModel
	req["model"] = route.UpstreamModel
	upstreamBody, _ := json.Marshal(req)

	input, _ := req["input"].(string)
	format, _ := req["response_format"].(string)
	if format == "" {
		format = "mp3"
	}
	x.ev.Media = &hooks.MediaUsage{
		Units:    utf8.RuneCountInString(input),
		UnitKind: hooks.MediaUnitAudioCharacter,
		Format:   format,
	}

	resp, ok := x.sendUpstream(route, "/audio/speech", upstreamBody)
	if !ok {
		return
	}
	defer resp.Body.Close()
	s.forwardMediaResponse(x, resp)
}

// handleAudioTranscriptions serves POST /v1/audio/transcriptions (STT).
func (s *Server) handleAudioTranscriptions(w http.ResponseWriter, r *http.Request) {
	s.handleOpenAIAudioMultipart(w, r, "/audio/transcriptions")
}

// handleAudioTranslations serves POST /v1/audio/translations (STT → English).
func (s *Server) handleAudioTranslations(w http.ResponseWriter, r *http.Request) {
	s.handleOpenAIAudioMultipart(w, r, "/audio/translations")
}

// handleOpenAIAudioMultipart forwards STT multipart (or rare JSON) bodies.
// Model is peeked best-effort for routing; body bytes are not rewritten.
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
	if !isOpenAIFamily(route.Provider) {
		x.fail(http.StatusNotImplemented, "invalid_request_error",
			"audio transcription requires an openai or openai_compat provider (got "+route.Provider.Kind+")",
			hooks.StatusBadRequest)
		return
	}
	if err := CheckCapability(route.Provider, route.ProviderName, config.ModalityAudioTranscribe); err != nil {
		x.fail(http.StatusBadRequest, "unsupported_provider_capability", err.Error(), hooks.StatusBadRequest)
		return
	}
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = route.UpstreamModel
	x.ev.Media = &hooks.MediaUsage{
		UnitKind: hooks.MediaUnitAudioMinute,
	}

	// JSON path: rewrite model like speech when body is JSON.
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
