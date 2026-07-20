package proxy

import "net/http"

// Anthropic Agent memory stores API — pure passthrough (#130).

func (s *Server) handleAnthropicMemoryStoresRoot(w http.ResponseWriter, r *http.Request) {
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.anthropicFamilyProxy(w, r, r.Method, "/memory_stores", withBody)
}

func (s *Server) handleAnthropicMemoryStoresID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "missing memory store id")
		return
	}
	rest := r.PathValue("rest")
	path := "/memory_stores/" + id
	if rest != "" {
		path += "/" + rest
	}
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.anthropicFamilyProxy(w, r, r.Method, path, withBody)
}
