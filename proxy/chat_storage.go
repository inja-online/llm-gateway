package proxy

import "net/http"

// OpenAI Chat Completions storage API (store=true resources) — pure passthrough.
// POST create remains handleOpenAI; these manage stored completions (#122).
//
//	GET    /v1/chat/completions
//	GET    /v1/chat/completions/{completion_id}
//	POST   /v1/chat/completions/{completion_id}
//	DELETE /v1/chat/completions/{completion_id}

func (s *Server) handleChatCompletionsList(w http.ResponseWriter, r *http.Request) {
	s.openAIFamilyProxy(w, r, http.MethodGet, "/chat/completions", false)
}

func (s *Server) handleChatCompletionsGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing completion id")
		return
	}
	s.openAIFamilyProxy(w, r, http.MethodGet, "/chat/completions/"+id, false)
}

func (s *Server) handleChatCompletionsUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing completion id")
		return
	}
	s.openAIFamilyProxy(w, r, http.MethodPost, "/chat/completions/"+id, true)
}

func (s *Server) handleChatCompletionsDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing completion id")
		return
	}
	s.openAIFamilyProxy(w, r, http.MethodDelete, "/chat/completions/"+id, false)
}
