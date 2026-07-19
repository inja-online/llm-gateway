package proxy

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/inja-online/llm-gateway/hooks"
)

// handleCompletions serves legacy OpenAI Completions and DeepSeek FIM
// (fill-in-the-middle) prefix/suffix completion.
//
// Routes (both experimental for vendor FIM; same handler):
//
//	POST /v1/completions  → {base}/completions
//	POST /beta/completions → DeepSeek beta base + /completions
//
// OpenAI-family only (openai / openai_compat). No Anthropic/Google FIM twins.
// Not multi-dialect translated — wire passthrough with model rewrite + one usage event.
//
// DeepSeek FIM requires the beta host prefix (docs: base_url=https://api.deepseek.com/beta).
// When the client hits /beta/completions, the gateway rewrites a normal chat base
// (…/v1 or host root) to …/beta before appending /completions.
func (s *Server) handleCompletions(w http.ResponseWriter, r *http.Request) {
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
	if !ensureOpenAIFamily(x, route, "Completions/FIM") {
		return
	}
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = route.UpstreamModel

	var req map[string]any
	if json.Unmarshal(body, &req) != nil {
		x.fail(http.StatusBadRequest, "invalid_request_error", "request body is not valid JSON", hooks.StatusBadRequest)
		return
	}
	req["model"] = route.UpstreamModel
	if head.Stream {
		ensureIncludeUsage(req)
	}
	upstreamBody, _ := json.Marshal(req)

	// /beta/completions → DeepSeek-style beta base rewrite; /v1/completions stays on configured base.
	upRoute := route
	if strings.HasPrefix(r.URL.Path, "/beta/") {
		upRoute = rewriteCompletionsBetaBase(route)
	}

	resp, ok := x.sendUpstream(upRoute, "/completions", upstreamBody)
	if !ok {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		x.forwardErrorResponse(resp)
		return
	}
	if head.Stream {
		x.passthroughStream(resp, extractOpenAIUsage)
		return
	}
	// Reuse chat JSON forwarder: same usage shape (prompt_tokens / completion_tokens).
	s.forwardOpenAIJSON(x, resp)
}

// rewriteCompletionsBetaBase maps a chat-oriented base_url onto DeepSeek's beta
// Completions host so FIM works without a separate provider block.
//
//	…/v1     → …/beta
//	…/beta   → unchanged
//	host root → host/beta
func rewriteCompletionsBetaBase(route Route) Route {
	out := route
	base := strings.TrimRight(route.Provider.BaseURL, "/")
	p := out.Provider
	switch {
	case strings.HasSuffix(base, "/beta"):
		p.BaseURL = base
	case strings.HasSuffix(base, "/v1"):
		p.BaseURL = strings.TrimSuffix(base, "/v1") + "/beta"
	default:
		p.BaseURL = base + "/beta"
	}
	out.Provider = p
	return out
}
