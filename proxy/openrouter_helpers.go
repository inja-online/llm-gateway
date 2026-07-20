package proxy

import "net/http"

// OpenRouter helper routes beyond chat (#139).
// Paths are relative to openai_compat base (https://openrouter.ai/api/v1).
//
//	GET  /v1/credits              → /credits (or /auth/key depending on host)
//	GET  /v1/generation           → /generation?id=…
//	GET  /v1/key                  → /key (API key metadata)
//
// Provider: defaults.openai_dialect or ?provider=openrouter.

func (s *Server) handleOpenRouterCredits(w http.ResponseWriter, r *http.Request) {
	s.openAIFamilyProxy(w, r, http.MethodGet, "/credits", false)
}

func (s *Server) handleOpenRouterKey(w http.ResponseWriter, r *http.Request) {
	s.openAIFamilyProxy(w, r, http.MethodGet, "/key", false)
}

func (s *Server) handleOpenRouterGeneration(w http.ResponseWriter, r *http.Request) {
	// Query (id=…) is preserved via stripProviderQuery.
	s.openAIFamilyProxy(w, r, http.MethodGet, "/generation", false)
}
