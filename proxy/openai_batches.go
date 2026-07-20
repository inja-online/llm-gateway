package proxy

import (
	"net/http"

	"github.com/inja-online/llm-gateway/hooks"
)

// OpenAI Batches API proxy — pure passthrough for openai / openai_compat.
// Batches and output files live on the upstream; the gateway does not store them.
//
//	POST   /v1/batches
//	GET    /v1/batches
//	GET    /v1/batches/{id}
//	POST   /v1/batches/{id}/cancel
//
// Provider: ?provider= | X-Provider | defaults.openai_dialect.
// Create body typically references input_file_id + endpoint; JSONL model ids
// inside the file are opaque (not rewritten). Usage event on create is estimated.

// handleOpenAIBatchesCreate serves POST /v1/batches.
func (s *Server) handleOpenAIBatchesCreate(w http.ResponseWriter, r *http.Request) {
	x := s.newExchange(w, r, DialectOpenAI, writeOpenAIError)
	defer x.emit()
	x.ev.Modality = "text"
	x.ev.Transport = hooks.TransportHTTP
	x.ev.Estimated = true
	x.ev.Model = "batches"
	x.ev.UpstreamModel = "batches"

	route, err := s.resolveOpenAIFamilyProvider(r)
	if err != nil {
		s.failProviderResolve(x, err)
		return
	}
	x.ev.Provider = route.ProviderName

	body, ok := x.readBody()
	if !ok {
		return
	}
	if len(body) > 0 {
		est := len(body) / charsPerTokenEstimate
		if est < 1 {
			est = 1
		}
		x.ev.TokensIn = est
	}

	resp, ok := x.sendUpstreamRaw(route, http.MethodPost, "/batches"+stripProviderQuery(r), body, "application/json")
	if !ok {
		return
	}
	defer resp.Body.Close()
	s.forwardBatchResponse(x, resp, false)
}

// handleOpenAIBatchesList serves GET /v1/batches.
func (s *Server) handleOpenAIBatchesList(w http.ResponseWriter, r *http.Request) {
	s.handleOpenAIBatchesPath(w, r, http.MethodGet, "/batches", false)
}

// handleOpenAIBatchesGet serves GET /v1/batches/{id}.
func (s *Server) handleOpenAIBatchesGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing batch id")
		return
	}
	s.handleOpenAIBatchesPath(w, r, http.MethodGet, "/batches/"+id, false)
}

// handleOpenAIBatchesCancel serves POST /v1/batches/{id}/cancel.
func (s *Server) handleOpenAIBatchesCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing batch id")
		return
	}
	s.handleOpenAIBatchesPath(w, r, http.MethodPost, "/batches/"+id+"/cancel", false)
}

func (s *Server) handleOpenAIBatchesPath(w http.ResponseWriter, r *http.Request, method, path string, streamBody bool) {
	x := s.newExchange(w, r, DialectOpenAI, writeOpenAIError)
	defer x.emit()
	x.ev.Modality = "text"
	x.ev.Transport = hooks.TransportHTTP
	x.ev.Estimated = true
	x.ev.Model = path
	x.ev.UpstreamModel = path

	route, err := s.resolveOpenAIFamilyProvider(r)
	if err != nil {
		s.failProviderResolve(x, err)
		return
	}
	x.ev.Provider = route.ProviderName

	resp, ok := x.sendUpstreamRaw(route, method, path+stripProviderQuery(r), nil, "")
	if !ok {
		return
	}
	defer resp.Body.Close()
	s.forwardBatchResponse(x, resp, streamBody)
}
