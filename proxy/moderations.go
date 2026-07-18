package proxy

import (
	"encoding/json"
	"net/http"

	"github.com/inja-online/llm-gateway/hooks"
)

// handleModerations serves POST /v1/moderations — OpenAI-family passthrough.
// Model is optional on some hosts; when present it is rewritten like chat.
func (s *Server) handleModerations(w http.ResponseWriter, r *http.Request) {
	x := s.newExchange(w, r, DialectOpenAI, writeOpenAIError)
	defer x.emit()
	x.ev.Modality = "text"
	x.ev.Transport = hooks.TransportHTTP

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
	var route Route
	var err error
	if model != "" {
		x.ev.Model = model
		route, err = Resolve(s.cfg, DialectOpenAI, model)
		if err != nil {
			x.fail(http.StatusNotFound, "invalid_request_error", err.Error(), hooks.StatusBadRequest)
			return
		}
		if !ensureOpenAIFamily(x, route, "Moderations") {
			return
		}
		req["model"] = route.UpstreamModel
		x.ev.UpstreamModel = route.UpstreamModel
	} else {
		// No model: route via default openai dialect / provider selector.
		route, err = s.resolveOpenAIFamilyProvider(r)
		if err != nil {
			s.failProviderResolve(x, err)
			return
		}
		x.ev.Model = "omni-moderation-latest"
		x.ev.UpstreamModel = "omni-moderation-latest"
	}
	x.ev.Provider = route.ProviderName

	upstreamBody, _ := json.Marshal(req)
	resp, ok := x.sendUpstream(route, "/moderations", upstreamBody)
	if !ok {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		x.forwardErrorResponse(resp)
		return
	}
	out, err := readAll(resp)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to read upstream response", hooks.StatusUpstreamError)
		return
	}
	// Moderations rarely report token usage.
	x.ev.Estimated = true
	x.ev.Status = hooks.StatusOK
	x.ev.HTTPStatus = resp.StatusCode
	x.prepareResponseHeaders(resp)
	if x.w.Header().Get("Content-Type") == "" {
		x.w.Header().Set("Content-Type", "application/json")
	}
	x.w.WriteHeader(resp.StatusCode)
	x.w.Write(out)
}
