package proxy

import (
	"net/http"

	"github.com/inja-online/llm-gateway/hooks"
)

// anthropicFamilyProxy is a thin kind:anthropic passthrough for resource APIs
// without a messages body model (skills, tunnels, memory stores, …).
// Provider: ?provider= | X-Provider | defaults.anthropic_dialect.
// Upstream path is relative to provider base_url (typically …/v1).
func (s *Server) anthropicFamilyProxy(w http.ResponseWriter, r *http.Request, method, path string, withBody bool) {
	x := s.newExchange(w, r, DialectAnthropic, writeAnthropicError)
	defer x.emit()
	x.ev.Modality = "text"
	x.ev.Transport = hooks.TransportHTTP
	x.ev.Estimated = true
	x.ev.Model = path
	x.ev.UpstreamModel = path

	route, err := s.resolveAnthropicProvider(r)
	if err != nil {
		s.failProviderResolve(x, err)
		return
	}
	x.ev.Provider = route.ProviderName

	var body []byte
	if withBody {
		var ok bool
		body, ok = x.readBody()
		if !ok {
			return
		}
	}
	ct := ""
	if withBody {
		ct = r.Header.Get("Content-Type")
		if ct == "" {
			ct = "application/json"
		}
	}
	resp, ok := x.sendUpstreamRaw(route, method, path+stripProviderQuery(r), body, ct)
	if !ok {
		return
	}
	defer resp.Body.Close()
	s.forwardFilesResponse(x, resp)
}
