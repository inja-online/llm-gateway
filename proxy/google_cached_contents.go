package proxy

import "net/http"

// Google context caching (cachedContents) platform API — pure kind:google
// passthrough (#112 cache slice). Paths relative to base_url (…/v1beta).
//
// Chat generateContent continues to reference caches via cachedContent name
// on the request body (#108). This surface creates/lists/updates/deletes those
// resources; the gateway does not store cache state.

func (s *Server) handleGoogleCachedContentsRoot(w http.ResponseWriter, r *http.Request) {
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.googleFamilyProxy(w, r, r.Method, "/cachedContents", withBody)
}

func (s *Server) handleGoogleCachedContentsID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeGoogleError(w, http.StatusBadRequest, "invalid_request_error", "missing cachedContent id")
		return
	}
	rest := r.PathValue("rest")
	path := "/cachedContents/" + id
	if rest != "" {
		path += "/" + rest
	}
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.googleFamilyProxy(w, r, r.Method, path, withBody)
}
