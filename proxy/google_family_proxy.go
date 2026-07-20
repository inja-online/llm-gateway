package proxy

import (
	"net/http"

	"github.com/inja-online/llm-gateway/hooks"
)

// googleFamilyProxy is a thin kind:google passthrough for platform APIs
// (fileSearchStores, tunedModels, …). Paths are relative to base_url
// (typically …/v1beta). Provider: ?provider= | defaults.google_dialect.
func (s *Server) googleFamilyProxy(w http.ResponseWriter, r *http.Request, method, path string, withBody bool) {
	x := s.newExchange(w, r, DialectGoogle, writeGoogleError)
	defer x.emit()
	x.ev.Modality = "text"
	x.ev.Transport = hooks.TransportHTTP
	x.ev.Estimated = true
	x.ev.Model = path
	x.ev.UpstreamModel = path

	route, err := s.resolveGoogleProvider(r)
	if err != nil {
		// Use OpenAI-shaped fail path only if exchange writeErr is Google.
		x.fail(http.StatusBadRequest, "invalid_request_error", err.Error(), hooks.StatusBadRequest)
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
