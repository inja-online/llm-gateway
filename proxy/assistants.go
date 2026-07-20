package proxy

import "net/http"

// OpenAI Assistants v2 legacy API — pure openai-family passthrough (#120).
// Clients should send OpenAI-Beta: assistants=v2 (forwarded via OpenAI headers).
// Prefer Responses for new work; this keeps SDK compatibility for existing agents.
//
// Surfaces: /v1/assistants, /v1/threads, /v1/threads/{id}/runs, /v1/threads/{id}/messages.

func (s *Server) handleAssistantsRoot(w http.ResponseWriter, r *http.Request) {
	withBody := r.Method != http.MethodGet && r.Method != http.MethodDelete && r.Method != http.MethodHead
	s.openAIFamilyProxy(w, r, r.Method, "/assistants", withBody)
}

func (s *Server) handleAssistantsID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing assistant id")
		return
	}
	rest := r.PathValue("rest")
	path := "/assistants/" + id
	if rest != "" {
		path += "/" + rest
	}
	withBody := r.Method != http.MethodGet && r.Method != http.MethodDelete && r.Method != http.MethodHead
	s.openAIFamilyProxy(w, r, r.Method, path, withBody)
}

func (s *Server) handleThreadsRoot(w http.ResponseWriter, r *http.Request) {
	withBody := r.Method != http.MethodGet && r.Method != http.MethodDelete && r.Method != http.MethodHead
	s.openAIFamilyProxy(w, r, r.Method, "/threads", withBody)
}

func (s *Server) handleThreadsID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing thread id")
		return
	}
	rest := r.PathValue("rest")
	path := "/threads/" + id
	if rest != "" {
		path += "/" + rest
	}
	withBody := r.Method != http.MethodGet && r.Method != http.MethodDelete && r.Method != http.MethodHead
	s.openAIFamilyProxy(w, r, r.Method, path, withBody)
}
