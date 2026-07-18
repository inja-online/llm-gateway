package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/inja-online/llm-gateway/canonical"
	"github.com/inja-online/llm-gateway/config"
	googleegress "github.com/inja-online/llm-gateway/egress/google"
	openaiegress "github.com/inja-online/llm-gateway/egress/openai"
	"github.com/inja-online/llm-gateway/hooks"
	antingress "github.com/inja-online/llm-gateway/ingress/anthropic"
	oaingress "github.com/inja-online/llm-gateway/ingress/openai"
)

// OpenAI-compatible media generation endpoints (image + video) with
// capability checks and cross-dialect translation via canonical models.

// handleImagesGenerations serves POST /v1/images/generations (OpenAI dialect).
func (s *Server) handleImagesGenerations(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("anthropic-version") != "" {
		writeOpenAIError(w, http.StatusBadRequest, CodeInvalidMediaRequest,
			"use POST /v1/images for Anthropic dialect media (do not send anthropic-version to /v1/images/generations)")
		return
	}
	s.handleOpenAIImage(w, r, canonical.ImageModeGenerate)
}

// handleImagesEdits serves POST /v1/images/edits.
// Anthropic-version header selects Anthropic gateway dialect; else OpenAI.
func (s *Server) handleImagesEdits(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("anthropic-version") != "" {
		s.handleAnthropicImagesEdits(w, r)
		return
	}
	s.handleOpenAIImage(w, r, canonical.ImageModeEdit)
}

// handleImagesVariations serves POST /v1/images/variations (OpenAI dialect).
func (s *Server) handleImagesVariations(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("anthropic-version") != "" {
		writeOpenAIError(w, http.StatusBadRequest, CodeInvalidMediaRequest,
			"image variations are OpenAI dialect only; remove anthropic-version or use OpenAI clients")
		return
	}
	s.handleOpenAIImage(w, r, canonical.ImageModeVariation)
}

// handleVideosCreate serves POST /v1/videos.
func (s *Server) handleVideosCreate(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("anthropic-version") != "" {
		s.handleAnthropicVideosCreate(w, r)
		return
	}
	s.handleOpenAIVideoCreate(w, r)
}

// handleVideosGet serves GET /v1/videos/{id}.
func (s *Server) handleVideosGet(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("anthropic-version") != "" {
		s.handleAnthropicVideosGet(w, r)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, CodeInvalidRequest, "missing video id")
		return
	}
	s.handleOpenAIVideoGet(w, r, id)
}

// handleOpenAIImage handles OpenAI image generate/edit/variation with translation.
func (s *Server) handleOpenAIImage(w http.ResponseWriter, r *http.Request, mode string) {
	x := s.newExchange(w, r, DialectOpenAI, writeOpenAIError)
	x.ev.Modality = config.ModalityImageGen
	x.ev.Transport = hooks.TransportHTTP
	defer x.emit()

	ct := r.Header.Get("Content-Type")
	isMultipart := strings.Contains(ct, "multipart/")

	body, ok := x.readBody()
	if !ok {
		return
	}

	if isMultipart {
		s.openaiImageMultipart(x, body, ct, mode)
		return
	}

	req, err := oaingress.ParseImageRequest(body, mode)
	if err != nil {
		msg := err.Error()
		if ve, ok := err.(*oaingress.ValidationError); ok {
			msg = ve.Msg
		}
		x.fail(http.StatusBadRequest, CodeInvalidRequest, msg, hooks.StatusBadRequest)
		return
	}
	x.ev.Model = req.Model

	route, err := ResolveForModality(s.cfg, DialectOpenAI, req.Model, config.ModalityImageGen)
	if err != nil {
		s.failRoute(x, err)
		return
	}
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = route.UpstreamModel

	switch providerKind(route.Provider) {
	case config.KindOpenAI, config.KindOpenAICompat:
		// Same-family passthrough with model rewrite (preserves unknown fields).
		s.openaiImagePassthroughJSON(x, route, body, mode)
	case config.KindGoogle:
		s.imageTranslateToGoogle(x, route, req)
	default:
		x.fail(http.StatusBadRequest, CodeUnsupportedProviderCapability,
			fmt.Sprintf("image generation is not supported for provider kind %s", route.Provider.Kind),
			hooks.StatusBadRequest)
	}
}

func (s *Server) openaiImagePassthroughJSON(x *exchange, route Route, body []byte, mode string) {
	var req map[string]any
	if json.Unmarshal(body, &req) != nil {
		x.fail(http.StatusBadRequest, CodeInvalidRequest, "request body is not valid JSON", hooks.StatusBadRequest)
		return
	}
	req["model"] = route.UpstreamModel
	upstreamBody, _ := json.Marshal(req)
	path := openaiegress.ImagePath(mode)
	resp, ok := x.sendUpstream(route, path, upstreamBody)
	if !ok {
		return
	}
	defer resp.Body.Close()
	s.forwardMediaResponse(x, resp, mediaUnitsFromOpenAIImageBody(upstreamBody))
}

func (s *Server) openaiImageMultipart(x *exchange, body []byte, ct, mode string) {
	model := peekMultipartModel(body)
	if model == "" {
		model = "unknown"
	}
	x.ev.Model = model

	var route Route
	var err error
	if model != "unknown" {
		route, err = ResolveForModality(s.cfg, DialectOpenAI, model, config.ModalityImageGen)
	} else {
		// Fall back to openai dialect default.
		if def := s.cfg.Defaults.OpenAIDialect; def != "" {
			route = Route{ProviderName: def, Provider: s.cfg.Providers[def], UpstreamModel: model}
			err = CheckCapability(route.Provider, route.ProviderName, config.ModalityImageGen)
		} else {
			err = fmt.Errorf("missing model and no defaults.openai_dialect")
		}
	}
	if err != nil {
		s.failRoute(x, err)
		return
	}
	if !isOpenAIFamily(route.Provider) {
		x.fail(http.StatusBadRequest, CodeUnsupportedProviderCapability,
			"multipart image edit/variation requires an openai or openai_compat provider",
			hooks.StatusBadRequest)
		return
	}
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = route.UpstreamModel

	path := openaiegress.ImagePath(mode)
	resp, ok := x.sendUpstreamRaw(route, http.MethodPost, path, body, ct)
	if !ok {
		return
	}
	defer resp.Body.Close()
	s.forwardMediaResponse(x, resp, &hooks.MediaUsage{Units: 1, UnitKind: hooks.MediaUnitImage})
}

func (s *Server) imageTranslateToGoogle(x *exchange, route Route, req *canonical.ImageGenRequest) {
	upBody, err := googleegress.BuildImagePredictRequest(req)
	if err != nil {
		x.fail(http.StatusBadRequest, CodeInvalidMediaRequest, "failed to build upstream image request", hooks.StatusBadRequest)
		return
	}
	path := googleegress.ImagePredictPath(route.UpstreamModel)
	resp, ok := x.sendUpstream(route, path, upBody)
	if !ok {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		s.translateGoogleErrorToOpenAI(x, resp)
		return
	}
	respBody, err := readAll(resp)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to read upstream response", hooks.StatusUpstreamError)
		return
	}
	canon, err := googleegress.ParseImageResponse(respBody, route.UpstreamModel)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to parse upstream image response", hooks.StatusUpstreamError)
		return
	}
	out, _ := oaingress.SerializeImageResponse(canon)
	s.writeMediaOK(x, out, imageMediaUsage(req, canon))
}

func (s *Server) handleOpenAIVideoCreate(w http.ResponseWriter, r *http.Request) {
	x := s.newExchange(w, r, DialectOpenAI, writeOpenAIError)
	x.ev.Modality = config.ModalityVideoGen
	x.ev.Transport = hooks.TransportHTTP
	defer x.emit()

	body, ok := x.readBody()
	if !ok {
		return
	}
	req, err := oaingress.ParseVideoCreateRequest(body)
	if err != nil {
		msg := err.Error()
		if ve, ok := err.(*oaingress.ValidationError); ok {
			msg = ve.Msg
		}
		x.fail(http.StatusBadRequest, CodeInvalidRequest, msg, hooks.StatusBadRequest)
		return
	}
	x.ev.Model = req.Model

	route, err := ResolveForModality(s.cfg, DialectOpenAI, req.Model, config.ModalityVideoGen)
	if err != nil {
		s.failRoute(x, err)
		return
	}
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = route.UpstreamModel

	switch providerKind(route.Provider) {
	case config.KindOpenAI, config.KindOpenAICompat:
		var m map[string]any
		if json.Unmarshal(body, &m) != nil {
			x.fail(http.StatusBadRequest, CodeInvalidRequest, "request body is not valid JSON", hooks.StatusBadRequest)
			return
		}
		m["model"] = route.UpstreamModel
		up, _ := json.Marshal(m)
		resp, ok := x.sendUpstream(route, openaiegress.VideoCreatePath(), up)
		if !ok {
			return
		}
		defer resp.Body.Close()
		s.forwardMediaResponse(x, resp, videoCreateMediaUsage(req))
	case config.KindGoogle:
		s.videoTranslateToGoogle(x, route, req, DialectOpenAI)
	default:
		x.fail(http.StatusBadRequest, CodeUnsupportedProviderCapability,
			fmt.Sprintf("video generation is not supported for provider kind %s", route.Provider.Kind),
			hooks.StatusBadRequest)
	}
}

func (s *Server) handleOpenAIVideoGet(w http.ResponseWriter, r *http.Request, id string) {
	x := s.newExchange(w, r, DialectOpenAI, writeOpenAIError)
	x.ev.Modality = config.ModalityVideoGen
	x.ev.Transport = hooks.TransportHTTP
	x.ev.Model = id
	x.ev.UpstreamModel = id
	defer x.emit()

	provName := r.URL.Query().Get("provider")
	if provName == "" {
		provName = s.cfg.Defaults.OpenAIDialect
	}
	if provName == "" {
		x.fail(http.StatusBadRequest, CodeInvalidRequest,
			"video status requires ?provider=NAME or defaults.openai_dialect", hooks.StatusBadRequest)
		return
	}
	route, err := ResolveProvider(s.cfg, provName)
	if err != nil {
		x.fail(http.StatusNotFound, CodeInvalidRequest, err.Error(), hooks.StatusBadRequest)
		return
	}
	if err := CheckCapability(route.Provider, route.ProviderName, config.ModalityVideoGen); err != nil {
		s.failRoute(x, err)
		return
	}
	x.ev.Provider = route.ProviderName

	switch providerKind(route.Provider) {
	case config.KindOpenAI, config.KindOpenAICompat:
		resp, ok := x.sendUpstreamRaw(route, http.MethodGet, openaiegress.VideoGetPath(id), nil, "")
		if !ok {
			return
		}
		defer resp.Body.Close()
		// Poll is operational — zero media units.
		s.forwardMediaResponse(x, resp, &hooks.MediaUsage{Units: 0, UnitKind: hooks.MediaUnitVideoSecond})
	case config.KindGoogle:
		path := googleegress.VideoPollPath(id)
		resp, ok := x.sendUpstreamRaw(route, http.MethodGet, path, nil, "")
		if !ok {
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			s.translateGoogleErrorToOpenAI(x, resp)
			return
		}
		respBody, err := readAll(resp)
		if err != nil {
			x.fail(http.StatusBadGateway, "api_error", "failed to read upstream response", hooks.StatusUpstreamError)
			return
		}
		canon, err := googleegress.ParseVideoResponse(respBody)
		if err != nil {
			x.fail(http.StatusBadGateway, "api_error", "failed to parse upstream video response", hooks.StatusUpstreamError)
			return
		}
		if canon.ID == "" {
			canon.ID = id
		}
		out, _ := oaingress.SerializeVideoResponse(canon)
		s.writeMediaOK(x, out, &hooks.MediaUsage{Units: 0, UnitKind: hooks.MediaUnitVideoSecond})
	default:
		x.fail(http.StatusBadRequest, CodeUnsupportedProviderCapability,
			"video status requires openai, openai_compat, or google provider", hooks.StatusBadRequest)
	}
}

func (s *Server) videoTranslateToGoogle(x *exchange, route Route, req *canonical.VideoGenRequest, dialect string) {
	upBody, err := googleegress.BuildVideoCreateRequest(req)
	if err != nil {
		x.fail(http.StatusBadRequest, CodeInvalidMediaRequest, "failed to build upstream video request", hooks.StatusBadRequest)
		return
	}
	path := googleegress.VideoGeneratePath(route.UpstreamModel)
	resp, ok := x.sendUpstream(route, path, upBody)
	if !ok {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		if dialect == DialectAnthropic {
			s.translateGoogleErrorToAnthropic(x, resp)
		} else {
			s.translateGoogleErrorToOpenAI(x, resp)
		}
		return
	}
	respBody, err := readAll(resp)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to read upstream response", hooks.StatusUpstreamError)
		return
	}
	canon, err := googleegress.ParseVideoResponse(respBody)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to parse upstream video response", hooks.StatusUpstreamError)
		return
	}
	if canon.Model == "" {
		canon.Model = route.UpstreamModel
	}
	var out []byte
	if dialect == DialectAnthropic {
		out, _ = antingress.SerializeVideoResponse(canon)
	} else {
		out, _ = oaingress.SerializeVideoResponse(canon)
	}
	s.writeMediaOK(x, out, videoCreateMediaUsage(req))
}

// --- shared media helpers ---

func (s *Server) failRoute(x *exchange, err error) {
	if err == nil {
		return
	}
	if _, ok := err.(*CapabilityError); ok || strings.Contains(err.Error(), "does not support modality") {
		x.fail(http.StatusBadRequest, CodeUnsupportedProviderCapability, err.Error(), hooks.StatusBadRequest)
		return
	}
	x.fail(http.StatusNotFound, CodeInvalidRequest, err.Error(), hooks.StatusBadRequest)
}

func (s *Server) forwardMediaResponse(x *exchange, resp *http.Response, media ...*hooks.MediaUsage) {
	if resp.StatusCode >= 400 {
		x.forwardErrorResponse(resp)
		return
	}
	body, err := readAll(resp)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to read upstream response", hooks.StatusUpstreamError)
		return
	}
	x.ev.Estimated = true
	x.ev.Status = hooks.StatusOK
	x.ev.HTTPStatus = resp.StatusCode
	if len(media) > 0 && media[0] != nil {
		x.ev.Media = media[0]
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		x.w.Header().Set("Content-Type", ct)
	} else {
		x.w.Header().Set("Content-Type", "application/json")
	}
	x.w.WriteHeader(resp.StatusCode)
	x.w.Write(body)
}

func (s *Server) writeMediaOK(x *exchange, body []byte, media *hooks.MediaUsage) {
	x.ev.Estimated = true
	x.ev.Status = hooks.StatusOK
	x.ev.HTTPStatus = http.StatusOK
	if media != nil {
		x.ev.Media = media
	}
	x.w.Header().Set("Content-Type", "application/json")
	x.w.WriteHeader(http.StatusOK)
	x.w.Write(body)
}

func isOpenAIFamily(p config.Provider) bool {
	return p.Kind == config.KindOpenAI || p.Kind == config.KindOpenAICompat
}

// sendUpstreamRaw posts/gets with an optional Content-Type (for multipart).
func (x *exchange) sendUpstreamRaw(route Route, method, path string, body []byte, contentType string) (*http.Response, bool) {
	key := clientKey(x.r)
	x.ev.KeyHash = hashKey(key)

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
	// Prefer ADC/env override when configured.
	if x.s != nil {
		if k, errMsg := x.s.resolveUpstreamKey(x.r, route.ProviderName, route.Provider); errMsg != "" {
			x.fail(http.StatusBadGateway, "api_error", errMsg, hooks.StatusUpstreamError)
			return nil, false
		} else if k != "" {
			key = k
			x.ev.KeyHash = hashKey(key)
		}
	}
	applyAuth(upReq, route.Provider, key)
	copyForwardHeaders(upReq, x.r)
	forwardOpenAIRequestHeaders(upReq, x.r, route.Provider)

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
	const marker = `name="model"`
	i := bytes.Index(body, []byte(marker))
	if i < 0 {
		return ""
	}
	rest := body[i+len(marker):]
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

func imageMediaUsage(req *canonical.ImageGenRequest, resp *canonical.ImageGenResponse) *hooks.MediaUsage {
	n := 1
	if resp != nil && len(resp.Images) > 0 {
		n = len(resp.Images)
	} else if req != nil && req.N > 0 {
		n = req.N
	}
	mu := &hooks.MediaUsage{Units: n, UnitKind: hooks.MediaUnitImage}
	if req != nil && req.Size != "" {
		mu.Size = req.Size
	}
	return mu
}

func mediaUnitsFromOpenAIImageBody(body []byte) *hooks.MediaUsage {
	var m struct {
		N    int    `json:"n"`
		Size string `json:"size"`
	}
	_ = json.Unmarshal(body, &m)
	n := m.N
	if n <= 0 {
		n = 1
	}
	return &hooks.MediaUsage{Units: n, UnitKind: hooks.MediaUnitImage, Size: m.Size}
}

func videoCreateMediaUsage(req *canonical.VideoGenRequest) *hooks.MediaUsage {
	units := 1
	if req != nil && req.Duration > 0 {
		units = int(req.Duration)
		if units < 1 {
			units = 1
		}
	}
	return &hooks.MediaUsage{Units: units, UnitKind: hooks.MediaUnitVideoSecond}
}

// handleVideosContent serves GET /v1/videos/{id}/content (binary download).
func (s *Server) handleVideosContent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing video id")
		return
	}
	// Reuse status poll path logic with /content suffix.
	s.handleOpenAIMediaContent(w, r, "/videos/"+id+"/content")
}

func rewriteMultipartModel(body []byte, contentType, upstreamModel string) (newBody []byte, newCT, originalModel string, err error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, "", "", fmt.Errorf("parse content-type: %w", err)
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, "", "", errors.New("multipart: missing boundary")
	}

	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	wroteModel := false

	for {
		part, pErr := mr.NextPart()
		if pErr == io.EOF {
			break
		}
		if pErr != nil {
			return nil, "", "", fmt.Errorf("multipart read: %w", pErr)
		}
		name := part.FormName()
		filename := part.FileName()
		data, rErr := io.ReadAll(part)
		_ = part.Close()
		if rErr != nil {
			return nil, "", "", fmt.Errorf("multipart part %q: %w", name, rErr)
		}

		if name == "model" && filename == "" {
			originalModel = string(bytes.TrimSpace(data))
			if wErr := mw.WriteField("model", upstreamModel); wErr != nil {
				return nil, "", "", wErr
			}
			wroteModel = true
			continue
		}

		// Preserve file and non-model fields, including part Content-Type.
		hdr := textproto.MIMEHeader{}
		if filename != "" {
			hdr.Set("Content-Disposition",
				fmt.Sprintf(`form-data; name="%s"; filename="%s"`, escapeQuotes(name), escapeQuotes(filename)))
		} else {
			hdr.Set("Content-Disposition",
				fmt.Sprintf(`form-data; name="%s"`, escapeQuotes(name)))
		}
		if pct := part.Header.Get("Content-Type"); pct != "" {
			hdr.Set("Content-Type", pct)
		} else if filename != "" {
			hdr.Set("Content-Type", "application/octet-stream")
		}
		pw, cErr := mw.CreatePart(hdr)
		if cErr != nil {
			return nil, "", "", cErr
		}
		if _, wErr := pw.Write(data); wErr != nil {
			return nil, "", "", wErr
		}
	}

	if !wroteModel && upstreamModel != "" {
		if wErr := mw.WriteField("model", upstreamModel); wErr != nil {
			return nil, "", "", wErr
		}
	}
	if cErr := mw.Close(); cErr != nil {
		return nil, "", "", cErr
	}
	return buf.Bytes(), mw.FormDataContentType(), originalModel, nil
}

func extractMultipartModel(body []byte, contentType string) (string, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", err
	}
	boundary := params["boundary"]
	if boundary == "" {
		return "", errors.New("multipart: missing boundary")
	}
	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, pErr := mr.NextPart()
		if pErr == io.EOF {
			return "", nil
		}
		if pErr != nil {
			return "", pErr
		}
		if part.FormName() == "model" && part.FileName() == "" {
			data, rErr := io.ReadAll(part)
			_ = part.Close()
			if rErr != nil {
				return "", rErr
			}
			return string(bytes.TrimSpace(data)), nil
		}
		_ = part.Close()
	}
}

func escapeQuotes(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

func (s *Server) handleOpenAIMediaContent(w http.ResponseWriter, r *http.Request, upstreamPath string) {
	x := s.newExchange(w, r, DialectOpenAI, writeOpenAIError)
	defer x.emit()
	x.ev.Modality = config.ModalityVideoGen
	x.ev.Transport = hooks.TransportHTTP
	// Poll is operational: zero media units (create bills duration).
	x.ev.Media = &hooks.MediaUsage{Units: 0, UnitKind: hooks.MediaUnitVideoSecond}

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
	x.ev.Provider = provName
	if err := CheckCapability(p, provName, config.ModalityVideoGen); err != nil {
		x.fail(http.StatusBadRequest, "unsupported_provider_capability", err.Error(), hooks.StatusBadRequest)
		return
	}
	if !isOpenAIFamily(p) {
		x.fail(http.StatusNotImplemented, "invalid_request_error",
			"video status requires an openai or openai_compat provider", hooks.StatusBadRequest)
		return
	}
	route := Route{ProviderName: provName, Provider: p, UpstreamModel: ""}
	x.ev.Model = r.PathValue("id")
	x.ev.UpstreamModel = r.PathValue("id")

	resp, ok := x.sendUpstreamRaw(route, http.MethodGet, upstreamPath, nil, "")
	if !ok {
		return
	}
	defer resp.Body.Close()
	s.forwardMediaResponse(x, resp)
}

func mediaUsageFromRequest(modality string, req map[string]any) *hooks.MediaUsage {
	m := &hooks.MediaUsage{}
	switch modality {
	case config.ModalityImageGen:
		m.UnitKind = hooks.MediaUnitImage
		if v, ok := asPositiveInt(req["n"]); ok {
			m.Units = v
		} else {
			m.Units = 1
		}
		if s, ok := req["size"].(string); ok {
			m.Size = s
		}
		if s, ok := req["response_format"].(string); ok {
			m.Format = s
		}
	case config.ModalityVideoGen:
		m.UnitKind = hooks.MediaUnitVideoSecond
		if v, ok := asPositiveInt(req["seconds"]); ok {
			m.Units = v
		} else if v, ok := asPositiveInt(req["duration"]); ok {
			m.Units = v
		} else {
			m.Units = 1
		}
	}
	return m
}

func asPositiveInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		if n > 0 {
			return int(n), true
		}
	case int:
		if n > 0 {
			return n, true
		}
	case string:
		var x int
		if _, err := fmt.Sscanf(n, "%d", &x); err == nil && x > 0 {
			return x, true
		}
	case json.Number:
		i, err := n.Int64()
		if err == nil && i > 0 {
			return int(i), true
		}
	}
	return 0, false
}
