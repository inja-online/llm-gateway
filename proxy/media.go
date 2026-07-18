package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

// OpenAI-compatible media generation endpoints (image + video).
// These are passthrough-only: client and upstream share the Chat Completions
// family's auth (Bearer). Native Gemini image/video generation via
// generateContent still uses the Google dialect path.

// handleImagesGenerations serves POST /v1/images/generations.
func (s *Server) handleImagesGenerations(w http.ResponseWriter, r *http.Request) {
	s.handleOpenAIMedia(w, r, "/images/generations", true)
}

// handleImagesEdits serves POST /v1/images/edits (JSON or multipart passthrough).
func (s *Server) handleImagesEdits(w http.ResponseWriter, r *http.Request) {
	s.handleOpenAIMedia(w, r, "/images/edits", true)
}

// handleImagesVariations serves POST /v1/images/variations.
func (s *Server) handleImagesVariations(w http.ResponseWriter, r *http.Request) {
	s.handleOpenAIMedia(w, r, "/images/variations", true)
}

// handleVideosCreate serves POST /v1/videos (async job create on OpenAI / Gemini OpenAI-compat).
func (s *Server) handleVideosCreate(w http.ResponseWriter, r *http.Request) {
	s.handleOpenAIMedia(w, r, "/videos", true)
}

// handleVideosGet serves GET /v1/videos/{id} (poll job status).
func (s *Server) handleVideosGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing video id")
		return
	}
	s.handleOpenAIMediaGET(w, r, "/videos/"+id)
}

// handleOpenAIMedia is a JSON (or raw-body) passthrough for OpenAI-family media APIs.
// requireModel rewrites the JSON "model" field when the body is application/json.
func (s *Server) handleOpenAIMedia(w http.ResponseWriter, r *http.Request, upstreamPath string, requireModel bool) {
	x := s.newExchange(w, r, DialectOpenAI, writeOpenAIError)
	defer x.emit()

	ct := r.Header.Get("Content-Type")
	isMultipart := strings.Contains(ct, "multipart/")

	body, ok := x.readBody()
	if !ok {
		return
	}

	var model string
	var upstreamBody []byte
	if isMultipart {
		// Multipart: cannot rewrite model without re-encoding; forward bytes.
		// Clients should send bare upstream model ids or rely on provider default.
		upstreamBody = body
		model = peekMultipartModel(body) // best-effort; may be empty
		if model == "" {
			model = "unknown"
		}
	} else {
		var req map[string]any
		if json.Unmarshal(body, &req) != nil {
			x.fail(http.StatusBadRequest, "invalid_request_error", "request body is not valid JSON", hooks.StatusBadRequest)
			return
		}
		if m, _ := req["model"].(string); m != "" {
			model = m
		}
		if requireModel && model == "" {
			x.fail(http.StatusBadRequest, "invalid_request_error", "missing or invalid required field: model", hooks.StatusBadRequest)
			return
		}
		if model == "" {
			// Some video endpoints allow default model; still need a route.
			model = "default"
		}
		x.ev.Model = model

		route, err := Resolve(s.cfg, DialectOpenAI, model)
		if err != nil {
			x.fail(http.StatusNotFound, "invalid_request_error", err.Error(), hooks.StatusBadRequest)
			return
		}
		if !isOpenAIFamily(route.Provider) {
			x.fail(http.StatusNotImplemented, "invalid_request_error",
				"image/video generation requires an openai or openai_compat provider (got "+route.Provider.Kind+")",
				hooks.StatusBadRequest)
			return
		}
		x.ev.Provider = route.ProviderName
		x.ev.UpstreamModel = route.UpstreamModel
		if model != "default" && model != "unknown" {
			req["model"] = route.UpstreamModel
		}
		upstreamBody, _ = json.Marshal(req)

		resp, ok := x.sendUpstream(route, upstreamPath, upstreamBody)
		if !ok {
			return
		}
		defer resp.Body.Close()
		s.forwardMediaResponse(x, resp)
		return
	}

	// Multipart branch
	x.ev.Model = model
	route, err := Resolve(s.cfg, DialectOpenAI, model)
	if err != nil {
		// Fall back to openai dialect default with a synthetic bare id.
		if def := s.cfg.Defaults.OpenAIDialect; def != "" {
			route = Route{ProviderName: def, Provider: s.cfg.Providers[def], UpstreamModel: model}
		} else {
			x.fail(http.StatusNotFound, "invalid_request_error", err.Error(), hooks.StatusBadRequest)
			return
		}
	}
	if !isOpenAIFamily(route.Provider) {
		x.fail(http.StatusNotImplemented, "invalid_request_error",
			"image/video generation requires an openai or openai_compat provider",
			hooks.StatusBadRequest)
		return
	}
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = route.UpstreamModel

	resp, ok := x.sendUpstreamRaw(route, http.MethodPost, upstreamPath, upstreamBody, ct)
	if !ok {
		return
	}
	defer resp.Body.Close()
	s.forwardMediaResponse(x, resp)
}

func (s *Server) handleOpenAIMediaGET(w http.ResponseWriter, r *http.Request, upstreamPath string) {
	x := s.newExchange(w, r, DialectOpenAI, writeOpenAIError)
	defer x.emit()

	// Prefer explicit provider query; else openai dialect default.
	provName := r.URL.Query().Get("provider")
	if provName == "" {
		provName = s.cfg.Defaults.OpenAIDialect
	}
	if provName == "" {
		x.fail(http.StatusBadRequest, "invalid_request_error",
			"video status requires ?provider=NAME or defaults.openai_dialect", hooks.StatusBadRequest)
		return
	}
	p, ok := s.cfg.Providers[provName]
	if !ok {
		x.fail(http.StatusNotFound, "invalid_request_error", "unknown provider "+provName, hooks.StatusBadRequest)
		return
	}
	if !isOpenAIFamily(p) {
		x.fail(http.StatusNotImplemented, "invalid_request_error",
			"video status requires an openai or openai_compat provider", hooks.StatusBadRequest)
		return
	}
	route := Route{ProviderName: provName, Provider: p, UpstreamModel: ""}
	x.ev.Provider = provName
	x.ev.Model = r.PathValue("id")
	x.ev.UpstreamModel = r.PathValue("id")

	resp, ok := x.sendUpstreamRaw(route, http.MethodGet, upstreamPath, nil, "")
	if !ok {
		return
	}
	defer resp.Body.Close()
	s.forwardMediaResponse(x, resp)
}

func (s *Server) forwardMediaResponse(x *exchange, resp *http.Response) {
	if resp.StatusCode >= 400 {
		x.forwardErrorResponse(resp)
		return
	}
	body, err := readAll(resp)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to read upstream response", hooks.StatusUpstreamError)
		return
	}
	// Image/video responses rarely include token usage; mark estimated.
	x.ev.Estimated = true
	x.ev.Status = hooks.StatusOK
	x.ev.HTTPStatus = resp.StatusCode
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		x.w.Header().Set("Content-Type", ct)
	} else {
		x.w.Header().Set("Content-Type", "application/json")
	}
	x.w.WriteHeader(resp.StatusCode)
	x.w.Write(body)
}

func isOpenAIFamily(p config.Provider) bool {
	return p.Kind == config.KindOpenAI || p.Kind == config.KindOpenAICompat
}

// sendUpstreamRaw posts/gets with an optional Content-Type (for multipart).
func (x *exchange) sendUpstreamRaw(route Route, method, path string, body []byte, contentType string) (*http.Response, bool) {
	key, errMsg := x.s.resolveUpstreamKey(x.r, route.ProviderName, route.Provider)
	if errMsg != "" {
		x.fail(http.StatusBadGateway, "api_error", errMsg, hooks.StatusUpstreamError)
		return nil, false
	}
	hashSrc := key
	if route.Provider.AuthMode() == config.AuthAPIKey || route.Provider.AuthMode() == config.AuthBearer {
		if route.Provider.APIKeyEnv != "" {
			if env := envLookup(route.Provider.APIKeyEnv); env != "" {
				hashSrc = env
			}
		}
	}
	x.ev.KeyHash = hashKey(hashSrc)

	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	upReq, err := http.NewRequestWithContext(x.r.Context(), method, route.Provider.BaseURL+path, rdr)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to build upstream request", hooks.StatusUpstreamError)
		return nil, false
	}
	if contentType != "" {
		upReq.Header.Set("Content-Type", contentType)
	} else if method != http.MethodGet && body != nil {
		upReq.Header.Set("Content-Type", "application/json")
	}
	applyAuth(upReq, route.Provider, key)
	copyForwardHeaders(upReq, x.r)

	resp, err := x.s.client.Do(upReq)
	if err != nil {
		if errors.Is(x.r.Context().Err(), context.Canceled) {
			x.ev.Status = hooks.StatusClientAbort
			x.ev.HTTPStatus = 499
			return nil, false
		}
		x.fail(http.StatusBadGateway, "api_error", "upstream request failed: "+err.Error(), hooks.StatusUpstreamError)
		return nil, false
	}
	return resp, true
}

// peekMultipartModel looks for name="model" in a multipart body (best-effort).
func peekMultipartModel(body []byte) string {
	// Crude scan: model\r\n\r\nVALUE
	const marker = `name="model"`
	i := bytes.Index(body, []byte(marker))
	if i < 0 {
		return ""
	}
	rest := body[i+len(marker):]
	// skip to double newline
	j := bytes.Index(rest, []byte("\r\n\r\n"))
	if j < 0 {
		j = bytes.Index(rest, []byte("\n\n"))
		if j < 0 {
			return ""
		}
		rest = rest[j+2:]
	} else {
		rest = rest[j+4:]
	}
	end := bytes.IndexAny(rest, "\r\n")
	if end < 0 {
		end = len(rest)
	}
	return string(bytes.TrimSpace(rest[:end]))
}
