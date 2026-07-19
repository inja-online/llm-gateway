package proxy

import (
	"net/http"
)

// Conversations / Assistants-style stateful thread APIs are intentionally not
// implemented. The gateway is stateless and prefers Responses + client-side
// state (and Files for provider-side assets). See README "Conversations API".
//
// Common routes are registered so SDKs receive an OpenAI-shaped 501
// (not_implemented) instead of a bare 404.

const conversationsNotImplementedMsg = "Conversations API is not implemented on this gateway (stateless). " +
	"Use POST /v1/responses with client-side conversation state, and/or the Files API for stored assets. " +
	"See README (Conversations API)."

// handleConversationsNotImplemented returns HTTP 501 with an OpenAI error envelope.
func (s *Server) handleConversationsNotImplemented(w http.ResponseWriter, r *http.Request) {
	_ = s
	_ = r
	writeOpenAIError(w, http.StatusNotImplemented, "not_implemented", conversationsNotImplementedMsg)
}
