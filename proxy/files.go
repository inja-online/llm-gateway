package proxy

import (
	"io"
	"net/http"
	"strings"

	"github.com/inja-online/llm-gateway/hooks"
)

// OpenAI Files API proxy — no gateway persistence. Files live on the upstream.
// Provider resolution: ?provider= | X-Provider | defaults.openai_dialect.

// handleFilesUpload serves POST /v1/files (multipart or raw).
func (s *Server) handleFilesUpload(w http.ResponseWriter, r *http.Request) {
	x := s.newExchange(w, r, DialectOpenAI, writeOpenAIError)
	defer x.emit()
	x.ev.Modality = "text"
	x.ev.Transport = hooks.TransportHTTP
	x.ev.Estimated = true
	x.ev.Model = "files"

	route, err := s.resolveOpenAIFamilyProvider(r)
	if err != nil {
		s.failProviderResolve(x, err)
		return
	}
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = "files"

	body, ok := x.readBody()
	if !ok {
		return
	}
	ct := r.Header.Get("Content-Type")
	resp, ok := x.sendUpstreamRaw(route, http.MethodPost, "/files"+stripProviderQuery(r), body, ct)
	if !ok {
		return
	}
	defer resp.Body.Close()
	s.forwardFilesResponse(x, resp)
}

// handleFilesList serves GET /v1/files.
func (s *Server) handleFilesList(w http.ResponseWriter, r *http.Request) {
	s.handleFilesID(w, r, http.MethodGet, "/files", false)
}

// handleFilesGet serves GET /v1/files/{id}.
func (s *Server) handleFilesGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing file id")
		return
	}
	s.handleFilesID(w, r, http.MethodGet, "/files/"+id, false)
}

// handleFilesDelete serves DELETE /v1/files/{id}.
func (s *Server) handleFilesDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing file id")
		return
	}
	s.handleFilesID(w, r, http.MethodDelete, "/files/"+id, false)
}

// handleFilesContent serves GET /v1/files/{id}/content (streamed).
func (s *Server) handleFilesContent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing file id")
		return
	}
	s.handleFilesID(w, r, http.MethodGet, "/files/"+id+"/content", true)
}

func (s *Server) handleFilesID(w http.ResponseWriter, r *http.Request, method, path string, streamBody bool) {
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
	if streamBody {
		s.forwardFilesStream(x, resp)
		return
	}
	s.forwardFilesResponse(x, resp)
}

func (s *Server) forwardFilesResponse(x *exchange, resp *http.Response) {
	if resp.StatusCode >= 400 {
		x.forwardErrorResponse(resp)
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

// forwardFilesStream relays file content without full buffering when possible.
func (s *Server) forwardFilesStream(x *exchange, resp *http.Response) {
	if resp.StatusCode >= 400 {
		x.forwardErrorResponse(resp)
		return
	}
	x.ev.Status = hooks.StatusOK
	x.ev.HTTPStatus = resp.StatusCode
	x.prepareResponseHeaders(resp)
	x.w.WriteHeader(resp.StatusCode)
	// Stream through; still size-capped for safety.
	_, err := io.Copy(x.w, io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		x.ev.Status = hooks.StatusUpstreamError
	}
}

func (s *Server) failProviderResolve(x *exchange, err error) {
	status := http.StatusBadRequest
	if strings.HasPrefix(err.Error(), "unknown provider") {
		status = http.StatusNotFound
	}
	if strings.Contains(err.Error(), "requires an openai") {
		status = http.StatusNotImplemented
	}
	x.fail(status, "invalid_request_error", err.Error(), hooks.StatusBadRequest)
}

// stripProviderQuery returns "?"+encoded query without provider=, or "".
func stripProviderQuery(r *http.Request) string {
	vals := r.URL.Query()
	vals.Del("provider")
	if enc := vals.Encode(); enc != "" {
		return "?" + enc
	}
	return ""
}
