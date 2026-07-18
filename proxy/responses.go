package proxy

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/inja-online/llm-gateway/hooks"
)

// handleResponses serves POST /v1/responses — OpenAI Responses API create
// (and streaming create). OpenAI-family passthrough only.
func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request) {
	x := s.newExchange(w, r, DialectOpenAI, writeOpenAIError)
	defer x.emit()
	x.ev.Modality = "text"
	x.ev.Transport = hooks.TransportHTTP

	body, ok := x.readBody()
	if !ok {
		return
	}

	var head struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if json.Unmarshal(body, &head) != nil || head.Model == "" {
		x.fail(http.StatusBadRequest, "invalid_request_error", "missing or invalid required field: model", hooks.StatusBadRequest)
		return
	}
	x.ev.Model = head.Model
	x.ev.Stream = head.Stream

	route, err := Resolve(s.cfg, DialectOpenAI, head.Model)
	if err != nil {
		x.fail(http.StatusNotFound, "invalid_request_error", err.Error(), hooks.StatusBadRequest)
		return
	}
	if !ensureOpenAIFamily(x, route, "Responses API") {
		return
	}
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = route.UpstreamModel

	var req map[string]any
	if json.Unmarshal(body, &req) != nil {
		x.fail(http.StatusBadRequest, "invalid_request_error", "request body is not valid JSON", hooks.StatusBadRequest)
		return
	}
	// Preserve all unknown fields; rewrite model only.
	req["model"] = route.UpstreamModel
	upstreamBody, _ := json.Marshal(req)

	resp, ok := x.sendUpstream(route, "/responses", upstreamBody)
	if !ok {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		x.forwardErrorResponse(resp)
		return
	}
	if head.Stream {
		x.passthroughStream(resp, extractResponsesUsage)
		return
	}
	s.forwardResponsesJSON(x, resp)
}

// handleResponsesGet serves GET /v1/responses/{id}.
// Provider: ?provider= | X-Provider | defaults.openai_dialect.
// Gateway does not store responses — pure proxy.
func (s *Server) handleResponsesGet(w http.ResponseWriter, r *http.Request) {
	s.handleResponsesResource(w, r, http.MethodGet)
}

// handleResponsesDelete serves DELETE /v1/responses/{id}.
func (s *Server) handleResponsesDelete(w http.ResponseWriter, r *http.Request) {
	s.handleResponsesResource(w, r, http.MethodDelete)
}

func (s *Server) handleResponsesResource(w http.ResponseWriter, r *http.Request, method string) {
	x := s.newExchange(w, r, DialectOpenAI, writeOpenAIError)
	defer x.emit()
	x.ev.Modality = "text"
	x.ev.Transport = hooks.TransportHTTP
	x.ev.Estimated = true

	id := r.PathValue("id")
	if id == "" {
		x.fail(http.StatusBadRequest, "invalid_request_error", "missing response id", hooks.StatusBadRequest)
		return
	}
	x.ev.Model = id
	x.ev.UpstreamModel = id

	route, err := s.resolveOpenAIFamilyProvider(r)
	if err != nil {
		status := http.StatusBadRequest
		if strings.HasPrefix(err.Error(), "unknown provider") {
			status = http.StatusNotFound
		}
		if strings.Contains(err.Error(), "requires an openai") {
			status = http.StatusNotImplemented
		}
		x.fail(status, "invalid_request_error", err.Error(), hooks.StatusBadRequest)
		return
	}
	x.ev.Provider = route.ProviderName

	// Forward remaining query (except provider) for future filters.
	path := "/responses/" + id
	if q := r.URL.RawQuery; q != "" {
		// Strip provider= from upstream query.
		vals := r.URL.Query()
		vals.Del("provider")
		if enc := vals.Encode(); enc != "" {
			path += "?" + enc
		}
	}

	resp, ok := x.sendUpstreamRaw(route, method, path, nil, "")
	if !ok {
		return
	}
	defer resp.Body.Close()
	s.forwardResponsesJSON(x, resp)
}

func (s *Server) forwardResponsesJSON(x *exchange, resp *http.Response) {
	if resp.StatusCode >= 400 {
		x.forwardErrorResponse(resp)
		return
	}
	body, err := readAll(resp)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to read upstream response", hooks.StatusUpstreamError)
		return
	}
	applyResponsesUsage(body, &x.ev)
	x.ev.Status = hooks.StatusOK
	x.ev.HTTPStatus = resp.StatusCode
	x.prepareResponseHeaders(resp)
	if x.w.Header().Get("Content-Type") == "" {
		x.w.Header().Set("Content-Type", "application/json")
	}
	x.w.WriteHeader(resp.StatusCode)
	x.w.Write(body)
}

// applyResponsesUsage extracts OpenAI Responses usage fields from a JSON body.
// Usage shape: { "usage": { "input_tokens": N, "output_tokens": M } }
func applyResponsesUsage(body []byte, ev *hooks.UsageEvent) {
	var parsed struct {
		Usage *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			// Some hosts also mirror chat completions names.
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(body, &parsed) != nil || parsed.Usage == nil {
		ev.Estimated = true
		return
	}
	in := parsed.Usage.InputTokens
	out := parsed.Usage.OutputTokens
	if in == 0 && out == 0 {
		in = parsed.Usage.PromptTokens
		out = parsed.Usage.CompletionTokens
	}
	if in == 0 && out == 0 {
		ev.Estimated = true
		return
	}
	ev.TokensIn = in
	ev.TokensOut = out
	ev.Estimated = false
}

// extractResponsesUsage scans Responses SSE data payloads for usage.
// Typical final event:
//
//	{"type":"response.completed","response":{"usage":{"input_tokens":…,"output_tokens":…}}}
func extractResponsesUsage(data []byte, ev *hooks.UsageEvent) bool {
	var payload struct {
		Type     string `json:"type"`
		Usage    *responsesUsageWire `json:"usage"`
		Response *struct {
			Usage *responsesUsageWire `json:"usage"`
		} `json:"response"`
	}
	if json.Unmarshal(data, &payload) != nil {
		return false
	}
	u := payload.Usage
	if u == nil && payload.Response != nil {
		u = payload.Response.Usage
	}
	if u == nil {
		return false
	}
	in, out := u.tokens()
	if in == 0 && out == 0 {
		return false
	}
	ev.TokensIn = in
	ev.TokensOut = out
	return true
}

type responsesUsageWire struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

func (u *responsesUsageWire) tokens() (in, out int) {
	in, out = u.InputTokens, u.OutputTokens
	if in == 0 && out == 0 {
		in, out = u.PromptTokens, u.CompletionTokens
	}
	return
}
