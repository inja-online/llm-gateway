package proxy

import (
	"net/http"

	"github.com/inja-online/llm-gateway/hooks"
)

// openAIFamilyProxy is a thin openai/openai_compat passthrough for resource APIs
// that have no model field (storage, fine-tuning, vector stores, uploads, …).
// Provider: ?provider= | X-Provider | defaults.openai_dialect.
func (s *Server) openAIFamilyProxy(w http.ResponseWriter, r *http.Request, method, path string, withBody bool) {
	x := s.newExchange(w, r, DialectOpenAI, writeOpenAIError)
	defer x.emit()
	x.ev.Modality = "text"
	x.ev.Transport = hooks.TransportHTTP
	x.ev.Estimated = true
	x.ev.Model = path
	x.ev.UpstreamModel = path

	route, err := s.resolveOpenAIFamilyProvider(r)
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
