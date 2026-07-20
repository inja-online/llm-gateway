package proxy

import "net/http"

// OpenAI Vector stores, Uploads, and Containers — upstream-owned passthrough (#113).
// Nested paths use {rest...} so file attachments and parts are covered.

func (s *Server) handleVectorStoresRoot(w http.ResponseWriter, r *http.Request) {
	s.openAIFamilyProxy(w, r, r.Method, "/vector_stores", r.Method != http.MethodGet && r.Method != http.MethodDelete)
}

func (s *Server) handleVectorStoresID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing vector store id")
		return
	}
	rest := r.PathValue("rest")
	path := "/vector_stores/" + id
	if rest != "" {
		path += "/" + rest
	}
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.openAIFamilyProxy(w, r, r.Method, path, withBody)
}

func (s *Server) handleUploadsRoot(w http.ResponseWriter, r *http.Request) {
	s.openAIFamilyProxy(w, r, r.Method, "/uploads", r.Method == http.MethodPost)
}

func (s *Server) handleUploadsID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing upload id")
		return
	}
	rest := r.PathValue("rest")
	path := "/uploads/" + id
	if rest != "" {
		path += "/" + rest
	}
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut
	s.openAIFamilyProxy(w, r, r.Method, path, withBody)
}

func (s *Server) handleContainersRoot(w http.ResponseWriter, r *http.Request) {
	s.openAIFamilyProxy(w, r, r.Method, "/containers", r.Method == http.MethodPost)
}

func (s *Server) handleContainersID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing container id")
		return
	}
	rest := r.PathValue("rest")
	path := "/containers/" + id
	if rest != "" {
		path += "/" + rest
	}
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut
	s.openAIFamilyProxy(w, r, r.Method, path, withBody)
}
