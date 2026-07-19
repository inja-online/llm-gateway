package proxy

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/inja-online/llm-gateway/canonical"
	"github.com/inja-online/llm-gateway/config"
	googleegress "github.com/inja-online/llm-gateway/egress/google"
	openaiegress "github.com/inja-online/llm-gateway/egress/openai"
	"github.com/inja-online/llm-gateway/hooks"
	googleingress "github.com/inja-online/llm-gateway/ingress/google"
)

// handleGoogleSpeech serves POST /v1beta/models/{model}:generateSpeech.
//
// Gateway Media Contract: Google-shaped TTS. When the resolved provider is
// kind:google, the request is translated to native Gemini generateContent with
// responseModalities=["AUDIO"] (real public TTS path). Response is the
// generateContent JSON (base64 inlineData) — no codec re-encode.
//
// When openai/openai_compat: translates to OpenAI /audio/speech; binary audio
// is wrapped as a generateContent-like JSON for Google clients.
func (s *Server) handleGoogleSpeech(w http.ResponseWriter, r *http.Request, pathModel string) {
	x := s.newExchange(w, r, DialectGoogle, writeGoogleError)
	x.ev.Modality = config.ModalityAudioSpeech
	x.ev.Transport = hooks.TransportHTTP
	defer x.emit()

	body, ok := x.readBody()
	if !ok {
		return
	}
	req, err := googleingress.ParseSpeechRequest(body, pathModel)
	if err != nil {
		msg := err.Error()
		if ve, ok := err.(*googleingress.ValidationError); ok {
			msg = ve.Msg
		}
		x.fail(http.StatusBadRequest, "INVALID_ARGUMENT", msg, hooks.StatusBadRequest)
		return
	}
	publicModel := req.Model
	x.ev.Model = publicModel

	route, err := ResolveForModality(s.cfg, DialectGoogle, publicModel, config.ModalityAudioSpeech)
	if err != nil {
		route, err = ResolveForModality(s.cfg, DialectGoogle, pathModel, config.ModalityAudioSpeech)
		if err != nil {
			s.failRoute(x, err)
			return
		}
	}
	if !strings.Contains(publicModel, "/") && pathModel != "" {
		if route.UpstreamModel == publicModel || route.UpstreamModel == "" {
			route.UpstreamModel = pathModel
		}
	}
	route.UpstreamModel = strings.TrimPrefix(route.UpstreamModel, "models/")
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = route.UpstreamModel
	x.ev.Media = speechMediaUsage(req)

	switch providerKind(route.Provider) {
	case config.KindGoogle:
		s.googleSpeechToGenerateContent(x, route, req)
	case config.KindOpenAI, config.KindOpenAICompat:
		s.googleSpeechToOpenAI(x, route, req)
	default:
		x.fail(http.StatusBadRequest, CodeUnsupportedProviderCapability,
			fmt.Sprintf("provider %q (kind %s) does not support modality %q",
				route.ProviderName, route.Provider.Kind, config.ModalityAudioSpeech),
			hooks.StatusBadRequest)
	}
}

// googleSpeechToGenerateContent → Gemini generateContent AUDIO; forward JSON as-is.
func (s *Server) googleSpeechToGenerateContent(x *exchange, route Route, req *canonical.AudioSpeechRequest) {
	upBody, err := googleegress.BuildSpeechGenerateContentRequest(req)
	if err != nil {
		x.fail(http.StatusBadRequest, "INVALID_ARGUMENT", err.Error(), hooks.StatusBadRequest)
		return
	}
	path := googleegress.SpeechGenerateContentPath(route.UpstreamModel)
	resp, ok := x.sendUpstream(route, path, upBody)
	if !ok {
		return
	}
	defer resp.Body.Close()
	// Forward generateContent JSON (or error envelope) without re-encoding audio.
	s.forwardMediaResponse(x, resp, speechMediaUsage(req))
}

// googleSpeechToOpenAI translates Google dialect TTS → OpenAI speech binary,
// then wraps bytes as generateContent-shaped JSON for Google clients.
func (s *Server) googleSpeechToOpenAI(x *exchange, route Route, req *canonical.AudioSpeechRequest) {
	upBody, err := openaiegress.BuildSpeechRequest(req, route.UpstreamModel)
	if err != nil {
		x.fail(http.StatusBadRequest, "INVALID_ARGUMENT", "failed to build upstream speech request", hooks.StatusBadRequest)
		return
	}
	resp, ok := x.sendUpstream(route, openaiegress.SpeechPath(), upBody)
	if !ok {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		s.translateOpenAIErrorToGoogle(x, resp)
		return
	}
	audioBytes, err := readAll(resp)
	if err != nil {
		x.fail(http.StatusBadGateway, "INTERNAL", "failed to read upstream response", hooks.StatusUpstreamError)
		return
	}
	// Wrap binary as generateContent-like JSON (base64 only — no codec change).
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = contentTypeForSpeechFormat(req.Format)
	}
	// Strip parameters from Content-Type for mimeType field.
	mime := ct
	if i := strings.Index(mime, ";"); i >= 0 {
		mime = strings.TrimSpace(mime[:i])
	}
	out, _ := json.Marshal(map[string]any{
		"candidates": []map[string]any{
			{
				"content": map[string]any{
					"parts": []map[string]any{
						{
							"inlineData": map[string]any{
								"mimeType": mime,
								"data":     base64.StdEncoding.EncodeToString(audioBytes),
							},
						},
					},
				},
			},
		},
		"modelVersion": route.UpstreamModel,
	})
	s.writeMediaOK(x, out, speechMediaUsage(req))
}
