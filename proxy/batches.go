package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

// Anthropic Message Batches API proxy — pure passthrough. Batches and results
// live on the upstream Anthropic provider; the gateway does not store them.
//
// Provider: ?provider= | X-Provider | defaults.anthropic_dialect (must be kind: anthropic).
// Create rewrites nested requests[].params.model (aliases / provider/model).

// handleBatchesCreate serves POST /v1/messages/batches.
func (s *Server) handleBatchesCreate(w http.ResponseWriter, r *http.Request) {
	x := s.newExchange(w, r, DialectAnthropic, writeAnthropicError)
	defer x.emit()
	x.ev.Modality = "text"
	x.ev.Transport = hooks.TransportHTTP
	x.ev.Estimated = true
	x.ev.Model = "batches"

	route, err := s.resolveAnthropicProvider(r)
	if err != nil {
		s.failProviderResolve(x, err)
		return
	}
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = "batches"

	body, ok := x.readBody()
	if !ok {
		return
	}

	rewritten, firstModel, firstUpstream, n, rerr := rewriteBatchCreateModels(s.cfg, body, route.ProviderName)
	if rerr != nil {
		status := http.StatusBadRequest
		if strings.HasPrefix(rerr.Error(), "unknown provider") {
			status = http.StatusNotFound
		}
		x.fail(status, "invalid_request_error", rerr.Error(), hooks.StatusBadRequest)
		return
	}
	if firstModel != "" {
		x.ev.Model = firstModel
		x.ev.UpstreamModel = firstUpstream
	}
	// Coarse estimate from request body size (no tokenizer; create is pre-billing).
	if n > 0 {
		est := len(rewritten) / charsPerTokenEstimate
		if est < 1 {
			est = 1
		}
		x.ev.TokensIn = est
	}

	resp, ok := x.sendUpstreamRaw(route, http.MethodPost, "/messages/batches", rewritten, "application/json")
	if !ok {
		return
	}
	defer resp.Body.Close()
	s.forwardBatchResponse(x, resp, false)
}

// handleBatchesList serves GET /v1/messages/batches.
func (s *Server) handleBatchesList(w http.ResponseWriter, r *http.Request) {
	s.handleBatchesPath(w, r, http.MethodGet, "/messages/batches", false)
}

// handleBatchesGet serves GET /v1/messages/batches/{id}.
func (s *Server) handleBatchesGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "missing batch id")
		return
	}
	s.handleBatchesPath(w, r, http.MethodGet, "/messages/batches/"+id, false)
}

// handleBatchesCancel serves POST /v1/messages/batches/{id}/cancel.
func (s *Server) handleBatchesCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "missing batch id")
		return
	}
	s.handleBatchesPath(w, r, http.MethodPost, "/messages/batches/"+id+"/cancel", false)
}

// handleBatchesResults serves GET /v1/messages/batches/{id}/results (JSONL stream).
func (s *Server) handleBatchesResults(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "missing batch id")
		return
	}
	s.handleBatchesPath(w, r, http.MethodGet, "/messages/batches/"+id+"/results", true)
}

// handleBatchesPath proxies list/get/cancel/results with a light operational usage event.
func (s *Server) handleBatchesPath(w http.ResponseWriter, r *http.Request, method, path string, streamBody bool) {
	x := s.newExchange(w, r, DialectAnthropic, writeAnthropicError)
	defer x.emit()
	x.ev.Modality = "text"
	x.ev.Transport = hooks.TransportHTTP
	x.ev.Estimated = true
	x.ev.Model = path
	x.ev.UpstreamModel = path

	route, err := s.resolveAnthropicProvider(r)
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

func (s *Server) forwardBatchResponse(x *exchange, resp *http.Response, streamBody bool) {
	if resp.StatusCode >= 400 {
		x.forwardErrorResponse(resp)
		return
	}
	if streamBody {
		x.ev.Status = hooks.StatusOK
		x.ev.HTTPStatus = resp.StatusCode
		x.prepareResponseHeaders(resp)
		x.w.WriteHeader(resp.StatusCode)
		_, err := io.Copy(x.w, io.LimitReader(resp.Body, maxBodyBytes))
		if err != nil {
			x.ev.Status = hooks.StatusUpstreamError
		}
		return
	}
	body, err := readAll(resp)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to read upstream response", hooks.StatusUpstreamError)
		return
	}
	x.ev.Status = hooks.StatusOK
	x.ev.HTTPStatus = resp.StatusCode
	x.prepareResponseHeaders(resp)
	if x.w.Header().Get("Content-Type") == "" {
		x.w.Header().Set("Content-Type", "application/json")
	}
	x.w.WriteHeader(resp.StatusCode)
	x.w.Write(body)
}

// rewriteBatchCreateModels walks requests[].params.model and rewrites gateway
// public ids (aliases, provider/model) to upstream model ids for batchProvider.
// Returns the rewritten body, first public model, first upstream model, request count.
func rewriteBatchCreateModels(cfg *config.Config, body []byte, batchProvider string) (out []byte, firstModel, firstUpstream string, n int, err error) {
	var root map[string]any
	if json.Unmarshal(body, &root) != nil {
		return nil, "", "", 0, fmt.Errorf("request body is not valid JSON")
	}
	reqs, ok := root["requests"].([]any)
	if !ok {
		// Let upstream validate missing/invalid requests; no model rewrite needed.
		return body, "", "", 0, nil
	}
	n = len(reqs)
	for i, item := range reqs {
		reqObj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		params, ok := reqObj["params"].(map[string]any)
		if !ok {
			continue
		}
		model, _ := params["model"].(string)
		if model == "" {
			continue
		}
		upstream, pub, rerr := rewriteBatchModel(cfg, model, batchProvider)
		if rerr != nil {
			return nil, "", "", 0, fmt.Errorf("requests[%d].params.model: %w", i, rerr)
		}
		params["model"] = upstream
		if firstModel == "" {
			firstModel = pub
			firstUpstream = upstream
		}
	}
	out, merr := json.Marshal(root)
	if merr != nil {
		return nil, "", "", 0, fmt.Errorf("failed to re-encode batch body")
	}
	return out, firstModel, firstUpstream, n, nil
}

// rewriteBatchModel resolves a public model id for a batch routed to batchProvider.
// Uses the same alias + provider/model rules as chat Resolve, then requires the
// resolved provider to match the batch target and be kind: anthropic.
func rewriteBatchModel(cfg *config.Config, model, batchProvider string) (upstream, public string, err error) {
	public = model
	route, err := Resolve(cfg, DialectAnthropic, model)
	if err != nil {
		return "", public, err
	}
	if route.Provider.Kind != config.KindAnthropic {
		return "", public, fmt.Errorf("batch models require an anthropic provider (got %s)", route.Provider.Kind)
	}
	if route.ProviderName != batchProvider {
		return "", public, fmt.Errorf("model %q resolves to provider %q but batch is routed to %q", model, route.ProviderName, batchProvider)
	}
	return route.UpstreamModel, public, nil
}
