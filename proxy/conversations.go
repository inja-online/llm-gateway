package proxy

import (
	"net/http"
)

// Conversations / Assistants-style stateful thread APIs are intentionally not
// implemented. The gateway is stateless and prefers Responses + client-side
// state (and Files for provider-side assets). See README "Conversations (not
// supported)" and docs/conversations-decision.md — permanent 501 (Option A).
//
// Common routes are registered so SDKs receive an OpenAI-shaped 501
// (not_implemented) instead of a bare 404.

const conversationsNotImplementedMsg = "Conversations API is not implemented on this gateway (stateless; no conversation store). " +
	"Use POST /v1/responses with client-side conversation/history state, and/or the Files API for stored assets. " +
	"Do not add gateway-persisted threads. See README (Conversations)."

// handleConversationsNotImplemented returns HTTP 501 with an OpenAI error envelope.
func (s *Server) handleConversationsNotImplemented(w http.ResponseWriter, r *http.Request) {
	_ = s
	_ = r
	writeOpenAIError(w, http.StatusNotImplemented, "not_implemented", conversationsNotImplementedMsg)
}
