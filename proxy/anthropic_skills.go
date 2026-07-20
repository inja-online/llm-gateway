package proxy

import "net/http"

// Anthropic Skills Management API — pure passthrough (#127).
// Gateway path /v1/skills* → upstream /skills* (base includes /v1).

func (s *Server) handleAnthropicSkillsRoot(w http.ResponseWriter, r *http.Request) {
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.anthropicFamilyProxy(w, r, r.Method, "/skills", withBody)
}

func (s *Server) handleAnthropicSkillsID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "missing skill id")
		return
	}
	rest := r.PathValue("rest")
	path := "/skills/" + id
	if rest != "" {
		path += "/" + rest
	}
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.anthropicFamilyProxy(w, r, r.Method, path, withBody)
}
