package proxy

import "net/http"

// Google File Search stores API — pure kind:google passthrough (#132).
// Gateway /v1beta/fileSearchStores* → upstream /fileSearchStores* (base …/v1beta).

func (s *Server) handleGoogleFileSearchStoresRoot(w http.ResponseWriter, r *http.Request) {
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.googleFamilyProxy(w, r, r.Method, "/fileSearchStores", withBody)
}

func (s *Server) handleGoogleFileSearchStoresID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeGoogleError(w, http.StatusBadRequest, "invalid_request_error", "missing fileSearchStore id")
		return
	}
	rest := r.PathValue("rest")
	path := "/fileSearchStores/" + id
	if rest != "" {
		path += "/" + rest
	}
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.googleFamilyProxy(w, r, r.Method, path, withBody)
}
