package proxy

import "net/http"

// Anthropic MCP tunnels API — pure passthrough (#129).

func (s *Server) handleAnthropicTunnelsRoot(w http.ResponseWriter, r *http.Request) {
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.anthropicFamilyProxy(w, r, r.Method, "/tunnels", withBody)
}

func (s *Server) handleAnthropicTunnelsID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "missing tunnel id")
		return
	}
	rest := r.PathValue("rest")
	path := "/tunnels/" + id
	if rest != "" {
		path += "/" + rest
	}
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.anthropicFamilyProxy(w, r, r.Method, path, withBody)
}
