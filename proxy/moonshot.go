package proxy

import (
	"net/http"

	"github.com/inja-online/llm-gateway/hooks"
)

// Moonshot / Kimi helper APIs (openai_compat family) — thin passthrough.
//
//	POST /v1/tokenizers/estimate-token-count  → {base}/tokenizers/estimate-token-count
//	GET  /v1/users/me/balance                → {base}/users/me/balance
//
// Provider: ?provider= | X-Provider | defaults.openai_dialect (must be openai family).
// Typically provider name is moonshot / moonshot_cn with matching regional base_url.
// No gateway storage; usage events are estimated/operational.

// handleMoonshotEstimateTokens proxies Moonshot token estimate.
func (s *Server) handleMoonshotEstimateTokens(w http.ResponseWriter, r *http.Request) {
	x := s.newExchange(w, r, DialectOpenAI, writeOpenAIError)
	defer x.emit()
	x.ev.Modality = "text"
	x.ev.Transport = hooks.TransportHTTP
	x.ev.Estimated = true
	x.ev.Model = "tokenizers/estimate-token-count"
	x.ev.UpstreamModel = "tokenizers/estimate-token-count"

	route, err := s.resolveOpenAIFamilyProvider(r)
	if err != nil {
		s.failProviderResolve(x, err)
		return
	}
	x.ev.Provider = route.ProviderName

	body, ok := x.readBody()
	if !ok {
		return
	}
	resp, ok := x.sendUpstreamRaw(route, http.MethodPost, "/tokenizers/estimate-token-count"+stripProviderQuery(r), body, "application/json")
	if !ok {
		return
	}
	defer resp.Body.Close()
	s.forwardFilesResponse(x, resp)
}

// handleMoonshotBalance proxies Moonshot account balance helper.
func (s *Server) handleMoonshotBalance(w http.ResponseWriter, r *http.Request) {
	x := s.newExchange(w, r, DialectOpenAI, writeOpenAIError)
	defer x.emit()
	x.ev.Modality = "text"
	x.ev.Transport = hooks.TransportHTTP
	x.ev.Estimated = true
	x.ev.Model = "users/me/balance"
	x.ev.UpstreamModel = "users/me/balance"

	route, err := s.resolveOpenAIFamilyProvider(r)
	if err != nil {
		s.failProviderResolve(x, err)
		return
	}
	x.ev.Provider = route.ProviderName

	resp, ok := x.sendUpstreamRaw(route, http.MethodGet, "/users/me/balance"+stripProviderQuery(r), nil, "")
	if !ok {
		return
	}
	defer resp.Body.Close()
	s.forwardFilesResponse(x, resp)
}
