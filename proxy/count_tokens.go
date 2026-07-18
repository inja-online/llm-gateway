package proxy

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/inja-online/llm-gateway/config"
	googleegress "github.com/inja-online/llm-gateway/egress/google"
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
// endpoint for an exact count. When kind is google we translate the Anthropic
// body to Gemini :countTokens and map totalTokens → input_tokens. Otherwise
// we estimate locally. No usage event is emitted.
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

	switch route.Provider.Kind {
	case config.KindAnthropic:
		if s.proxyCountTokens(w, r, route, body) {
			return
		}
		// fall through to the estimate if the upstream call failed
	case config.KindGoogle:
		if s.proxyCountTokensViaGoogle(w, r, route, body) {
			return
		}
		// fall through to the estimate if the upstream call failed
	}
	writeJSON(w, http.StatusOK, map[string]any{"input_tokens": estimateTokens(body)})
}

// proxyCountTokensViaGoogle translates an Anthropic count_tokens body to Gemini
// :countTokens and returns Anthropic-shaped {input_tokens}.
func (s *Server) proxyCountTokensViaGoogle(w http.ResponseWriter, r *http.Request, route Route, body []byte) bool {
	// Anthropic count_tokens often omits max_tokens; ParseRequest requires it.
	parseBody := ensureMaxTokens(body)
	req, err := antingress.ParseRequest(parseBody)
	if err != nil {
		return false
	}
	upBody, err := googleegress.BuildRequest(req, route.UpstreamModel)
	if err != nil {
		return false
	}

	ctx, cancel := contextWithTimeout(r, 15*time.Second)
	defer cancel()
	upReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		route.Provider.BaseURL+googleegress.CountTokensPath(route.UpstreamModel), bytesReader(upBody))
	if err != nil {
		return false
	}
	upReq.Header.Set("Content-Type", "application/json")
	applyAuth(upReq, route.Provider, clientKey(r))

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
	tokens, ok := parseGoogleTotalTokens(out)
	if !ok {
		return false
	}
	writeJSON(w, http.StatusOK, map[string]any{"input_tokens": tokens})
	return true
}

// parseGoogleTotalTokens reads totalTokens (or snake_case) from a countTokens response.
func parseGoogleTotalTokens(body []byte) (int, bool) {
	var env struct {
		TotalTokens      int `json:"totalTokens"`
		TotalTokensSnake int `json:"total_tokens"`
	}
	if json.Unmarshal(body, &env) != nil {
		return 0, false
	}
	if env.TotalTokens > 0 {
		return env.TotalTokens, true
	}
	if env.TotalTokensSnake > 0 {
		return env.TotalTokensSnake, true
	}
	// Zero is a valid count for empty prompts.
	if env.TotalTokens == 0 && env.TotalTokensSnake == 0 {
		// Distinguish "field missing" from "zero": accept if body parsed as object with known shape.
		var raw map[string]any
		if json.Unmarshal(body, &raw) != nil {
			return 0, false
		}
		if _, ok := raw["totalTokens"]; ok {
			return 0, true
		}
		if _, ok := raw["total_tokens"]; ok {
			return 0, true
		}
	}
	return 0, false
}

// ensureMaxTokens injects max_tokens:1 when missing so Anthropic ParseRequest accepts the body.
func ensureMaxTokens(body []byte) []byte {
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		return body
	}
	switch v := m["max_tokens"].(type) {
	case float64:
		if v > 0 {
			return body
		}
	case int:
		if v > 0 {
			return body
		}
	case json.Number:
		if n, err := v.Int64(); err == nil && n > 0 {
			return body
		}
	}
	m["max_tokens"] = 1
	b, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return b
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
	forwardOpenAIRequestHeaders(upReq, r, route.Provider)

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
