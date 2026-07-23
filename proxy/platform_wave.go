package proxy

import "net/http"

// Platform API wave: thin family proxies for remaining M6 endpoints.
// Upstream-owned resources; gateway does not store state (#121–#198 slice).

// --- Google Files API (#197) ---

func (s *Server) handleGoogleFilesRoot(w http.ResponseWriter, r *http.Request) {
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.googleFamilyProxy(w, r, r.Method, "/files", withBody)
}

func (s *Server) handleGoogleFilesID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeGoogleError(w, http.StatusBadRequest, "invalid_request_error", "missing file id")
		return
	}
	rest := r.PathValue("rest")
	path := "/files/" + id
	if rest != "" {
		path += "/" + rest
	}
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.googleFamilyProxy(w, r, r.Method, path, withBody)
}

// --- Google Interactions API (#134) ---

func (s *Server) handleGoogleInteractionsRoot(w http.ResponseWriter, r *http.Request) {
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.googleFamilyProxy(w, r, r.Method, "/interactions", withBody)
}

func (s *Server) handleGoogleInteractionsID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeGoogleError(w, http.StatusBadRequest, "invalid_request_error", "missing interaction id")
		return
	}
	rest := r.PathValue("rest")
	path := "/interactions/" + id
	if rest != "" {
		path += "/" + rest
	}
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.googleFamilyProxy(w, r, r.Method, path, withBody)
}

// --- Google batches / async jobs platform (#198, #135) ---

func (s *Server) handleGoogleBatchesRoot(w http.ResponseWriter, r *http.Request) {
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.googleFamilyProxy(w, r, r.Method, "/batches", withBody)
}

func (s *Server) handleGoogleBatchesID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeGoogleError(w, http.StatusBadRequest, "invalid_request_error", "missing batch id")
		return
	}
	rest := r.PathValue("rest")
	path := "/batches/" + id
	if rest != "" {
		path += "/" + rest
	}
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.googleFamilyProxy(w, r, r.Method, path, withBody)
}

// --- Anthropic Managed Agents (#128) ---

func (s *Server) handleAnthropicAgentsRoot(w http.ResponseWriter, r *http.Request) {
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.anthropicFamilyProxy(w, r, r.Method, "/agents", withBody)
}

func (s *Server) handleAnthropicAgentsID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "missing agent id")
		return
	}
	rest := r.PathValue("rest")
	path := "/agents/" + id
	if rest != "" {
		path += "/" + rest
	}
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.anthropicFamilyProxy(w, r, r.Method, path, withBody)
}

func (s *Server) handleAnthropicSessionsRoot(w http.ResponseWriter, r *http.Request) {
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.anthropicFamilyProxy(w, r, r.Method, "/sessions", withBody)
}

func (s *Server) handleAnthropicSessionsID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "missing session id")
		return
	}
	rest := r.PathValue("rest")
	path := "/sessions/" + id
	if rest != "" {
		path += "/" + rest
	}
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.anthropicFamilyProxy(w, r, r.Method, path, withBody)
}

func (s *Server) handleAnthropicEnvironmentsRoot(w http.ResponseWriter, r *http.Request) {
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.anthropicFamilyProxy(w, r, r.Method, "/environments", withBody)
}

func (s *Server) handleAnthropicEnvironmentsID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "missing environment id")
		return
	}
	rest := r.PathValue("rest")
	path := "/environments/" + id
	if rest != "" {
		path += "/" + rest
	}
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.anthropicFamilyProxy(w, r, r.Method, path, withBody)
}

// --- OpenAI Realtime extras (#123) ---

func (s *Server) handleRealtimeClientSecrets(w http.ResponseWriter, r *http.Request) {
	s.openAIFamilyProxy(w, r, r.Method, "/realtime/client_secrets", r.Method == http.MethodPost)
}

func (s *Server) handleRealtimeCalls(w http.ResponseWriter, r *http.Request) {
	s.openAIFamilyProxy(w, r, r.Method, "/realtime/calls", r.Method == http.MethodPost)
}

func (s *Server) handleRealtimeTranslations(w http.ResponseWriter, r *http.Request) {
	// Session create/list for translation realtime (HTTP surface; WS uses /v1/realtime).
	s.openAIFamilyProxy(w, r, r.Method, "/realtime/translations", r.Method != http.MethodGet && r.Method != http.MethodDelete)
}

// --- OpenAI Evals / prompts (#124) ---

func (s *Server) handleEvalsRoot(w http.ResponseWriter, r *http.Request) {
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.openAIFamilyProxy(w, r, r.Method, "/evals", withBody)
}

func (s *Server) handleEvalsID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing eval id")
		return
	}
	rest := r.PathValue("rest")
	path := "/evals/" + id
	if rest != "" {
		path += "/" + rest
	}
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.openAIFamilyProxy(w, r, r.Method, path, withBody)
}

// --- OpenAI Admin APIs (#121) ---

func (s *Server) handleOrganizationRoot(w http.ResponseWriter, r *http.Request) {
	// /v1/organization/* and /v1/organizations/* admin surface.
	rest := r.PathValue("rest")
	path := "/organization"
	if rest != "" {
		path += "/" + rest
	}
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.openAIFamilyProxy(w, r, r.Method, path, withBody)
}

func (s *Server) handleOrganizationsRoot(w http.ResponseWriter, r *http.Request) {
	rest := r.PathValue("rest")
	path := "/organizations"
	if rest != "" {
		path += "/" + rest
	}
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.openAIFamilyProxy(w, r, r.Method, path, withBody)
}

// --- Responses depth: compact + input_items (#110) ---

func (s *Server) handleResponsesCompact(w http.ResponseWriter, r *http.Request) {
	s.openAIFamilyProxy(w, r, http.MethodPost, "/responses/compact", true)
}

func (s *Server) handleResponsesInputItems(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing response id")
		return
	}
	rest := r.PathValue("rest")
	path := "/responses/" + id + "/input_items"
	if rest != "" {
		path += "/" + rest
	}
	withBody := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	s.openAIFamilyProxy(w, r, r.Method, path, withBody)
}

// --- Models DELETE fine-tuned (#150) ---

func (s *Server) handleModelsDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing model id")
		return
	}
	// Nested id... may include path segments; PathValue("id...") already joined.
	s.openAIFamilyProxy(w, r, http.MethodDelete, "/models/"+id, false)
}

// --- Video full parity extras (#147) ---

func (s *Server) handleVideosList(w http.ResponseWriter, r *http.Request) {
	s.openAIFamilyProxy(w, r, http.MethodGet, "/videos", false)
}

func (s *Server) handleVideosDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing video id")
		return
	}
	s.openAIFamilyProxy(w, r, http.MethodDelete, "/videos/"+id, false)
}

func (s *Server) handleVideosRemix(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing video id")
		return
	}
	s.openAIFamilyProxy(w, r, http.MethodPost, "/videos/"+id+"/remix", true)
}

// --- xAI deferred completions (#138) ---

func (s *Server) handleDeferredCompletion(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing deferred completion id")
		return
	}
	s.openAIFamilyProxy(w, r, http.MethodGet, "/chat/deferred-completion/"+id, false)
}

// --- Cohere-style rerank via openai_compat hosts (#143) ---

func (s *Server) handleRerank(w http.ResponseWriter, r *http.Request) {
	// Common paths: /v1/rerank (Cohere compat) or /rerank
	s.openAIFamilyProxy(w, r, http.MethodPost, "/rerank", true)
}

// --- Mistral OCR / agents-style (#142) when exposed as openai_compat paths ---

func (s *Server) handleOCR(w http.ResponseWriter, r *http.Request) {
	s.openAIFamilyProxy(w, r, http.MethodPost, "/ocr", true)
}


