package proxy

import (
	"fmt"
	"net/http"

	"github.com/inja-online/llm-gateway/hooks"
)

// resolveOpenAIFamilyProvider picks an OpenAI-family provider when the request
// has no model field (Files, stored Responses GET/DELETE, video status).
//
// Precedence:
//  1. ?provider=NAME
//  2. X-Provider: NAME header
//  3. defaults.openai_dialect
func (s *Server) resolveOpenAIFamilyProvider(r *http.Request) (Route, error) {
	name := r.URL.Query().Get("provider")
	if name == "" {
		name = r.Header.Get("X-Provider")
	}
	if name == "" {
		name = s.cfg.Defaults.OpenAIDialect
	}
	if name == "" {
		return Route{}, fmt.Errorf("provider required: pass ?provider=NAME, X-Provider header, or set defaults.openai_dialect")
	}
	p, ok := s.cfg.Providers[name]
	if !ok {
		return Route{}, fmt.Errorf("unknown provider %q", name)
	}
	if !isOpenAIFamily(p) {
		return Route{}, fmt.Errorf("endpoint requires an openai or openai_compat provider (got %s)", p.Kind)
	}
	return Route{ProviderName: name, Provider: p}, nil
}

// ensureOpenAIFamily fails the exchange when the route is not openai-family.
func ensureOpenAIFamily(x *exchange, route Route, feature string) bool {
	if isOpenAIFamily(route.Provider) {
		return true
	}
	x.fail(http.StatusNotImplemented, "invalid_request_error",
		feature+" requires an openai or openai_compat provider (got "+route.Provider.Kind+")",
		hooks.StatusBadRequest)
	return false
}
