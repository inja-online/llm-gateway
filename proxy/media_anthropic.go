package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/inja-online/llm-gateway/canonical"
	"github.com/inja-online/llm-gateway/config"
	googleegress "github.com/inja-online/llm-gateway/egress/google"
	openaiegress "github.com/inja-online/llm-gateway/egress/openai"
	"github.com/inja-online/llm-gateway/hooks"
	antingress "github.com/inja-online/llm-gateway/ingress/anthropic"
)

// handleAnthropicImagesGenerate serves POST /v1/images (Gateway Media Contract v1).
// Requires anthropic-version; Anthropic error envelope.
func (s *Server) handleAnthropicImagesGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("anthropic-version") == "" {
		writeAnthropicError(w, http.StatusBadRequest, CodeInvalidRequest,
			"anthropic-version header is required for Anthropic dialect media (Gateway Media Contract v1)")
		return
	}
	s.handleAnthropicImage(w, r, canonical.ImageModeGenerate)
}

// handleAnthropicImagesEdits serves POST /v1/images/edits when anthropic-version is set
// (checked by the shared /v1/images/edits dispatcher).
func (s *Server) handleAnthropicImagesEdits(w http.ResponseWriter, r *http.Request) {
	s.handleAnthropicImage(w, r, canonical.ImageModeEdit)
}

func (s *Server) handleAnthropicImage(w http.ResponseWriter, r *http.Request, mode string) {
	x := s.newExchange(w, r, DialectAnthropic, writeAnthropicError)
	x.ev.Modality = config.ModalityImageGen
	x.ev.Transport = hooks.TransportHTTP
	defer x.emit()

	body, ok := x.readBody()
	if !ok {
		return
	}
	req, err := antingress.ParseImageRequest(body, mode)
	if err != nil {
		msg := err.Error()
		if ve, ok := err.(*antingress.ValidationError); ok {
			msg = ve.Msg
		}
		x.fail(http.StatusBadRequest, CodeInvalidRequest, msg, hooks.StatusBadRequest)
		return
	}
	x.ev.Model = req.Model

	route, err := ResolveForModality(s.cfg, DialectAnthropic, req.Model, config.ModalityImageGen)
	if err != nil {
		s.failRoute(x, err)
		return
	}
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = route.UpstreamModel

	switch providerKind(route.Provider) {
	case config.KindOpenAI, config.KindOpenAICompat:
		s.anthropicImageToOpenAI(x, route, req)
	case config.KindGoogle:
		s.anthropicImageToGoogle(x, route, req)
	default:
		x.fail(http.StatusBadRequest, CodeUnsupportedProviderCapability,
			fmt.Sprintf("provider %q (kind %s) does not support modality %q",
				route.ProviderName, route.Provider.Kind, config.ModalityImageGen),
			hooks.StatusBadRequest)
	}
}

func (s *Server) anthropicImageToOpenAI(x *exchange, route Route, req *canonical.ImageGenRequest) {
	upBody, err := openaiegress.BuildImageRequest(req, route.UpstreamModel)
	if err != nil {
		x.fail(http.StatusBadRequest, CodeInvalidMediaRequest, "failed to build upstream image request", hooks.StatusBadRequest)
		return
	}
	path := openaiegress.ImagePath(req.Mode)
	resp, ok := x.sendUpstream(route, path, upBody)
	if !ok {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		s.translateOpenAIErrorToAnthropic(x, resp)
		return
	}
	respBody, err := readAll(resp)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to read upstream response", hooks.StatusUpstreamError)
		return
	}
	canon, err := openaiegress.ParseImageResponse(respBody, route.UpstreamModel)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to parse upstream image response", hooks.StatusUpstreamError)
		return
	}
	if canon.ID == "" {
		canon.ID = "img_" + x.ev.RequestID
	}
	out, _ := antingress.SerializeImageResponse(canon)
	s.writeMediaOK(x, out, imageMediaUsage(req, canon))
}

func (s *Server) anthropicImageToGoogle(x *exchange, route Route, req *canonical.ImageGenRequest) {
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
		s.translateGoogleErrorToAnthropic(x, resp)
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
	if canon.ID == "" {
		canon.ID = "img_" + x.ev.RequestID
	}
	out, _ := antingress.SerializeImageResponse(canon)
	s.writeMediaOK(x, out, imageMediaUsage(req, canon))
}

func (s *Server) handleAnthropicVideosCreate(w http.ResponseWriter, r *http.Request) {
	// anthropic-version is required; enforced by the /v1/videos dispatcher.
	x := s.newExchange(w, r, DialectAnthropic, writeAnthropicError)
	x.ev.Modality = config.ModalityVideoGen
	x.ev.Transport = hooks.TransportHTTP
	defer x.emit()

	body, ok := x.readBody()
	if !ok {
		return
	}
	req, err := antingress.ParseVideoCreateRequest(body)
	if err != nil {
		msg := err.Error()
		if ve, ok := err.(*antingress.ValidationError); ok {
			msg = ve.Msg
		}
		x.fail(http.StatusBadRequest, CodeInvalidRequest, msg, hooks.StatusBadRequest)
		return
	}
	x.ev.Model = req.Model

	route, err := ResolveForModality(s.cfg, DialectAnthropic, req.Model, config.ModalityVideoGen)
	if err != nil {
		s.failRoute(x, err)
		return
	}
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = route.UpstreamModel

	switch providerKind(route.Provider) {
	case config.KindOpenAI, config.KindOpenAICompat:
		upBody, err := openaiegress.BuildVideoCreateRequest(req, route.UpstreamModel)
		if err != nil {
			x.fail(http.StatusBadRequest, CodeInvalidMediaRequest, "failed to build upstream video request", hooks.StatusBadRequest)
			return
		}
		resp, ok := x.sendUpstream(route, openaiegress.VideoCreatePath(), upBody)
		if !ok {
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			s.translateOpenAIErrorToAnthropic(x, resp)
			return
		}
		respBody, err := readAll(resp)
		if err != nil {
			x.fail(http.StatusBadGateway, "api_error", "failed to read upstream response", hooks.StatusUpstreamError)
			return
		}
		canon, err := openaiegress.ParseVideoResponse(respBody)
		if err != nil {
			x.fail(http.StatusBadGateway, "api_error", "failed to parse upstream video response", hooks.StatusUpstreamError)
			return
		}
		if canon.Model == "" {
			canon.Model = route.UpstreamModel
		}
		out, _ := antingress.SerializeVideoResponse(canon)
		s.writeMediaOK(x, out, videoCreateMediaUsage(req))
	case config.KindGoogle:
		s.videoTranslateToGoogle(x, route, req, DialectAnthropic)
	default:
		x.fail(http.StatusBadRequest, CodeUnsupportedProviderCapability,
			fmt.Sprintf("provider %q (kind %s) does not support modality %q",
				route.ProviderName, route.Provider.Kind, config.ModalityVideoGen),
			hooks.StatusBadRequest)
	}
}

func (s *Server) handleAnthropicVideosGet(w http.ResponseWriter, r *http.Request) {
	// anthropic-version is required; enforced by the /v1/videos/{id} dispatcher.
	id := r.PathValue("id")
	if id == "" {
		writeAnthropicError(w, http.StatusBadRequest, CodeInvalidRequest, "missing video id")
		return
	}
	x := s.newExchange(w, r, DialectAnthropic, writeAnthropicError)
	x.ev.Modality = config.ModalityVideoGen
	x.ev.Transport = hooks.TransportHTTP
	x.ev.Model = id
	x.ev.UpstreamModel = id
	defer x.emit()

	provName := r.URL.Query().Get("provider")
	if provName == "" {
		provName = s.cfg.Defaults.AnthropicDialect
	}
	if provName == "" {
		provName = s.cfg.Defaults.OpenAIDialect
	}
	if provName == "" {
		x.fail(http.StatusBadRequest, CodeInvalidRequest,
			"video status requires ?provider=NAME or defaults.anthropic_dialect", hooks.StatusBadRequest)
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
		if resp.StatusCode >= 400 {
			s.translateOpenAIErrorToAnthropic(x, resp)
			return
		}
		respBody, err := readAll(resp)
		if err != nil {
			x.fail(http.StatusBadGateway, "api_error", "failed to read upstream response", hooks.StatusUpstreamError)
			return
		}
		canon, err := openaiegress.ParseVideoResponse(respBody)
		if err != nil {
			x.fail(http.StatusBadGateway, "api_error", "failed to parse upstream video response", hooks.StatusUpstreamError)
			return
		}
		if canon.ID == "" {
			canon.ID = id
		}
		out, _ := antingress.SerializeVideoResponse(canon)
		s.writeMediaOK(x, out, &hooks.MediaUsage{Units: 0, UnitKind: hooks.MediaUnitVideoSecond})
	case config.KindGoogle:
		resp, ok := x.sendUpstreamRaw(route, http.MethodGet, googleegress.VideoPollPath(id), nil, "")
		if !ok {
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			s.translateGoogleErrorToAnthropic(x, resp)
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
		out, _ := antingress.SerializeVideoResponse(canon)
		s.writeMediaOK(x, out, &hooks.MediaUsage{Units: 0, UnitKind: hooks.MediaUnitVideoSecond})
	default:
		x.fail(http.StatusBadRequest, CodeUnsupportedProviderCapability,
			"video status requires openai, openai_compat, or google provider", hooks.StatusBadRequest)
	}
}

func (s *Server) translateOpenAIErrorToAnthropic(x *exchange, resp *http.Response) {
	body, _ := readAll(resp)
	msg, code := "upstream error", "api_error"
	var in struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &in) == nil {
		if in.Error.Message != "" {
			msg = in.Error.Message
		}
		if in.Error.Type != "" {
			code = in.Error.Type
		}
	}
	x.ev.Status = hooks.StatusUpstreamError
	x.ev.HTTPStatus = writeAnthropicError(x.w, resp.StatusCode, code, msg)
}
