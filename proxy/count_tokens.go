package proxy

import (
	"encoding/json"
	"net/http"
	"time"

	antingress "github.com/inja-online/llm-gateway/ingress/anthropic"
)

// charsPerTokenEstimate is a coarse average across English and code. The
// gateway ships no tokenizer (see README): count_tokens exists so clients that
// call it before a request — Claude Code among them — get a usable number
// instead of a 404. Treat the result as an estimate, not billing input.
const charsPerTokenEstimate = 4

// handleCountTokens serves POST /v1/messages/count_tokens.
//
// When the resolved provider is Anthropic-kind we proxy to the real upstream
// endpoint for an exact count. Otherwise we estimate locally, since no other
// provider exposes an equivalent endpoint.
func (s *Server) handleCountTokens(w http.ResponseWriter, r *http.Request) {
	body, err := readAllLimited(r)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "failed to read request body")
		return
	}
	var head struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &head) != nil || head.Model == "" {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "missing or invalid required field: model")
		return
	}
	route, rerr := Resolve(s.cfg, DialectAnthropic, head.Model)
	if rerr != nil {
		writeAnthropicError(w, http.StatusNotFound, "invalid_request_error", rerr.Error())
		return
	}

	if route.Provider.Kind == "anthropic" {
		if s.proxyCountTokens(w, r, route, body) {
			return
		}
		// fall through to the estimate if the upstream call failed
	}
	writeJSON(w, http.StatusOK, map[string]any{"input_tokens": estimateTokens(body)})
}

// proxyCountTokens forwards to a real Anthropic count_tokens endpoint.
// It reports whether the response was served.
func (s *Server) proxyCountTokens(w http.ResponseWriter, r *http.Request, route Route, body []byte) bool {
	var req map[string]any
	if json.Unmarshal(body, &req) != nil {
		return false
	}
	req["model"] = route.UpstreamModel
	upBody, _ := json.Marshal(req)

	ctx, cancel := contextWithTimeout(r, 15*time.Second)
	defer cancel()
	upReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		route.Provider.BaseURL+"/messages/count_tokens", bytesReader(upBody))
	if err != nil {
		return false
	}
	upReq.Header.Set("Content-Type", "application/json")
	key, errMsg := s.resolveUpstreamKey(r, route.ProviderName, route.Provider)
	if errMsg != "" {
		return false
	}
	applyAuth(upReq, route.Provider, key)
	copyForwardHeaders(upReq, r)

	resp, err := s.client.Do(upReq)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return false
	}
	out, err := readAll(resp)
	if err != nil {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(out)
	return true
}

// estimateTokens produces a character-based estimate over the request's text.
func estimateTokens(body []byte) int {
	req, err := antingress.ParseRequest(body)
	if err != nil {
		// Fall back to raw body size rather than failing the call.
		return len(body)/charsPerTokenEstimate + 1
	}
	chars := 0
	for _, b := range req.System {
		chars += len(b.Text)
	}
	for _, m := range req.Messages {
		for _, b := range m.Content {
			chars += len(b.Text) + len(b.Result) + len(b.Input)
		}
	}
	for _, t := range req.Tools {
		chars += len(t.Name) + len(t.Description) + len(t.Schema)
	}
	tokens := chars / charsPerTokenEstimate
	if tokens < 1 {
		tokens = 1
	}
	return tokens
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
