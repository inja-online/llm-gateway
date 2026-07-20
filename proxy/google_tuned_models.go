package proxy

import "net/http"

// Google Tuned models API — pure kind:google passthrough (#133).

func (s *Server) handleGoogleTunedModelsRoot(w http.ResponseWriter, r *http.Request) {
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.googleFamilyProxy(w, r, r.Method, "/tunedModels", withBody)
}

func (s *Server) handleGoogleTunedModelsID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeGoogleError(w, http.StatusBadRequest, "invalid_request_error", "missing tuned model id")
		return
	}
	rest := r.PathValue("rest")
	path := "/tunedModels/" + id
	if rest != "" {
		path += "/" + rest
	}
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.googleFamilyProxy(w, r, r.Method, path, withBody)
}
