package proxy

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/inja-online/llm-gateway/canonical"
	"github.com/inja-online/llm-gateway/config"
	googleegress "github.com/inja-online/llm-gateway/egress/google"
	openaiegress "github.com/inja-online/llm-gateway/egress/openai"
	"github.com/inja-online/llm-gateway/hooks"
	googleingress "github.com/inja-online/llm-gateway/ingress/google"
)

// handleGoogleMedia is invoked from handleGoogle when the path action is an
// image/video method (generateImages, predict, generateVideos, predictLongRunning).
func (s *Server) handleGoogleMedia(w http.ResponseWriter, r *http.Request, model, method string) {
	switch method {
	case "generateImages", "predict":
		s.handleGoogleImage(w, r, model, method)
	default: // generateVideos, predictLongRunning (validated by parseGoogleAction)
		s.handleGoogleVideoCreate(w, r, model, method)
	}
}

// handleGoogleVideoPoll serves GET /v1beta/videos/{name} (LRO / job poll).
func (s *Server) handleGoogleVideoPoll(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeGoogleError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "missing video/operation name")
		return
	}
	x := s.newExchange(w, r, DialectGoogle, writeGoogleError)
	x.ev.Modality = config.ModalityVideoGen
	x.ev.Transport = hooks.TransportHTTP
	x.ev.Model = name
	x.ev.UpstreamModel = name
	defer x.emit()

	provName := r.URL.Query().Get("provider")
	if provName == "" {
		provName = s.cfg.Defaults.GoogleDialect
	}
	if provName == "" {
		x.fail(http.StatusBadRequest, "INVALID_ARGUMENT",
			"video status requires ?provider=NAME or defaults.google_dialect", hooks.StatusBadRequest)
		return
	}
	route, err := ResolveProvider(s.cfg, provName)
	if err != nil {
		x.fail(http.StatusNotFound, "INVALID_ARGUMENT", err.Error(), hooks.StatusBadRequest)
		return
	}
	if err := CheckCapability(route.Provider, route.ProviderName, config.ModalityVideoGen); err != nil {
		s.failRoute(x, err)
		return
	}
	x.ev.Provider = route.ProviderName

	switch providerKind(route.Provider) {
	case config.KindGoogle:
		path := googleegress.VideoPollPath(name)
		resp, ok := x.sendUpstreamRaw(route, http.MethodGet, path, nil, "")
		if !ok {
			return
		}
		defer resp.Body.Close()
		s.forwardMediaResponse(x, resp, &hooks.MediaUsage{Units: 0, UnitKind: hooks.MediaUnitVideoSecond})
	case config.KindOpenAI, config.KindOpenAICompat:
		// Strip operations/videos prefix for OpenAI-style ids when present.
		id := name
		if i := strings.LastIndex(name, "/"); i >= 0 {
			id = name[i+1:]
		}
		resp, ok := x.sendUpstreamRaw(route, http.MethodGet, openaiegress.VideoGetPath(id), nil, "")
		if !ok {
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			s.translateOpenAIErrorToGoogle(x, resp)
			return
		}
		respBody, err := readAll(resp)
		if err != nil {
			x.fail(http.StatusBadGateway, "INTERNAL", "failed to read upstream response", hooks.StatusUpstreamError)
			return
		}
		canon, err := openaiegress.ParseVideoResponse(respBody)
		if err != nil {
			x.fail(http.StatusBadGateway, "INTERNAL", "failed to parse upstream video response", hooks.StatusUpstreamError)
			return
		}
		if canon.ID == "" {
			canon.ID = name
		}
		out, _ := googleingress.SerializeVideoPollResponse(canon)
		s.writeMediaOK(x, out, &hooks.MediaUsage{Units: 0, UnitKind: hooks.MediaUnitVideoSecond})
	default:
		x.fail(http.StatusBadRequest, CodeUnsupportedProviderCapability,
			"video poll requires google, openai, or openai_compat provider", hooks.StatusBadRequest)
	}
}

func (s *Server) handleGoogleImage(w http.ResponseWriter, r *http.Request, pathModel, method string) {
	x := s.newExchange(w, r, DialectGoogle, writeGoogleError)
	x.ev.Modality = config.ModalityImageGen
	x.ev.Transport = hooks.TransportHTTP
	defer x.emit()

	body, ok := x.readBody()
	if !ok {
		return
	}
	req, err := googleingress.ParseImageRequest(body, pathModel)
	if err != nil {
		msg := err.Error()
		if ve, ok := err.(*googleingress.ValidationError); ok {
			msg = ve.Msg
		}
		x.fail(http.StatusBadRequest, "INVALID_ARGUMENT", msg, hooks.StatusBadRequest)
		return
	}
	// Prefer body model for routing when it has provider/ prefix.
	publicModel := req.Model
	x.ev.Model = publicModel

	route, err := ResolveForModality(s.cfg, DialectGoogle, publicModel, config.ModalityImageGen)
	if err != nil {
		// Retry with path model alone.
		route, err = ResolveForModality(s.cfg, DialectGoogle, pathModel, config.ModalityImageGen)
		if err != nil {
			s.failRoute(x, err)
			return
		}
	}
	// Bare path models: keep upstream as path model when resolve used default.
	if route.UpstreamModel == publicModel && !strings.Contains(publicModel, "/") {
		// ok
	}
	if strings.Contains(publicModel, "/") {
		// resolved correctly
	} else if pathModel != "" && route.UpstreamModel == publicModel {
		route.UpstreamModel = pathModel
		if route.UpstreamModel == "" {
			route.UpstreamModel = publicModel
		}
	}
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = route.UpstreamModel

	switch providerKind(route.Provider) {
	case config.KindGoogle:
		s.googleImagePassthrough(x, route, body, method)
	case config.KindOpenAI, config.KindOpenAICompat:
		s.googleImageToOpenAI(x, route, req)
	default:
		x.fail(http.StatusBadRequest, CodeUnsupportedProviderCapability,
			fmt.Sprintf("provider %q (kind %s) does not support modality %q",
				route.ProviderName, route.Provider.Kind, config.ModalityImageGen),
			hooks.StatusBadRequest)
	}
}

func (s *Server) googleImagePassthrough(x *exchange, route Route, body []byte, method string) {
	// Prefer real Imagen :predict upstream; generateImages is gateway alias.
	path := googleegress.ImagePredictPath(route.UpstreamModel)
	if method == "predict" {
		// Passthrough client body as-is for native predict.
		resp, ok := x.sendUpstream(route, path, body)
		if !ok {
			return
		}
		defer resp.Body.Close()
		s.forwardMediaResponse(x, resp, &hooks.MediaUsage{Units: 1, UnitKind: hooks.MediaUnitImage})
		return
	}
	// generateImages: rewrite to Imagen predict instances/parameters.
	// Caller already validated prompt via ParseImageRequest.
	req, err := googleingress.ParseImageRequest(body, route.UpstreamModel)
	if err != nil {
		x.fail(http.StatusBadRequest, "INVALID_ARGUMENT", err.Error(), hooks.StatusBadRequest)
		return
	}
	upBody, err := googleegress.BuildImagePredictRequest(req)
	if err != nil {
		x.fail(http.StatusBadRequest, "INVALID_ARGUMENT", "failed to build predict body", hooks.StatusBadRequest)
		return
	}
	resp, ok := x.sendUpstream(route, path, upBody)
	if !ok {
		return
	}
	defer resp.Body.Close()
	s.forwardMediaResponse(x, resp, imageMediaUsage(req, nil))
}

func (s *Server) googleImageToOpenAI(x *exchange, route Route, req *canonical.ImageGenRequest) {
	upBody, err := openaiegress.BuildImageRequest(req, route.UpstreamModel)
	if err != nil {
		x.fail(http.StatusBadRequest, "INVALID_ARGUMENT", "failed to build upstream image request", hooks.StatusBadRequest)
		return
	}
	resp, ok := x.sendUpstream(route, openaiegress.ImagePath(canonical.ImageModeGenerate), upBody)
	if !ok {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		s.translateOpenAIErrorToGoogle(x, resp)
		return
	}
	respBody, err := readAll(resp)
	if err != nil {
		x.fail(http.StatusBadGateway, "INTERNAL", "failed to read upstream response", hooks.StatusUpstreamError)
		return
	}
	canon, err := openaiegress.ParseImageResponse(respBody, route.UpstreamModel)
	if err != nil {
		x.fail(http.StatusBadGateway, "INTERNAL", "failed to parse upstream image response", hooks.StatusUpstreamError)
		return
	}
	out, _ := googleingress.SerializeImageResponse(canon)
	s.writeMediaOK(x, out, imageMediaUsage(req, canon))
}

func (s *Server) handleGoogleVideoCreate(w http.ResponseWriter, r *http.Request, pathModel, method string) {
	x := s.newExchange(w, r, DialectGoogle, writeGoogleError)
	x.ev.Modality = config.ModalityVideoGen
	x.ev.Transport = hooks.TransportHTTP
	defer x.emit()

	body, ok := x.readBody()
	if !ok {
		return
	}
	req, err := googleingress.ParseVideoCreateRequest(body, pathModel)
	if err != nil {
		msg := err.Error()
		if ve, ok := err.(*googleingress.ValidationError); ok {
			msg = ve.Msg
		}
		x.fail(http.StatusBadRequest, "INVALID_ARGUMENT", msg, hooks.StatusBadRequest)
		return
	}
	publicModel := req.Model
	x.ev.Model = publicModel

	route, err := ResolveForModality(s.cfg, DialectGoogle, publicModel, config.ModalityVideoGen)
	if err != nil {
		route, err = ResolveForModality(s.cfg, DialectGoogle, pathModel, config.ModalityVideoGen)
		if err != nil {
			s.failRoute(x, err)
			return
		}
	}
	if !strings.Contains(publicModel, "/") && pathModel != "" {
		// Prefer path model as upstream id for bare models.
		if route.UpstreamModel == publicModel || route.UpstreamModel == "" {
			route.UpstreamModel = pathModel
		}
	}
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = route.UpstreamModel

	switch providerKind(route.Provider) {
	case config.KindGoogle:
		path := googleegress.VideoGeneratePath(route.UpstreamModel)
		if method == "predictLongRunning" {
			path = googleegress.VideoPredictLongRunningPath(route.UpstreamModel)
			// native body passthrough for predictLongRunning
			resp, ok := x.sendUpstream(route, path, body)
			if !ok {
				return
			}
			defer resp.Body.Close()
			s.forwardMediaResponse(x, resp, videoCreateMediaUsage(req))
			return
		}
		// generateVideos → build instances/parameters (or forward if already shaped)
		upBody := body
		if built, err := googleegress.BuildVideoCreateRequest(req); err == nil {
			upBody = built
		}
		resp, ok := x.sendUpstream(route, path, upBody)
		if !ok {
			return
		}
		defer resp.Body.Close()
		s.forwardMediaResponse(x, resp, videoCreateMediaUsage(req))
	case config.KindOpenAI, config.KindOpenAICompat:
		upBody, err := openaiegress.BuildVideoCreateRequest(req, route.UpstreamModel)
		if err != nil {
			x.fail(http.StatusBadRequest, "INVALID_ARGUMENT", "failed to build upstream video request", hooks.StatusBadRequest)
			return
		}
		resp, ok := x.sendUpstream(route, openaiegress.VideoCreatePath(), upBody)
		if !ok {
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			s.translateOpenAIErrorToGoogle(x, resp)
			return
		}
		respBody, err := readAll(resp)
		if err != nil {
			x.fail(http.StatusBadGateway, "INTERNAL", "failed to read upstream response", hooks.StatusUpstreamError)
			return
		}
		canon, err := openaiegress.ParseVideoResponse(respBody)
		if err != nil {
			x.fail(http.StatusBadGateway, "INTERNAL", "failed to parse upstream video response", hooks.StatusUpstreamError)
			return
		}
		if canon.Model == "" {
			canon.Model = route.UpstreamModel
		}
		out, _ := googleingress.SerializeVideoCreateResponse(canon)
		s.writeMediaOK(x, out, videoCreateMediaUsage(req))
	default:
		x.fail(http.StatusBadRequest, CodeUnsupportedProviderCapability,
			fmt.Sprintf("provider %q (kind %s) does not support modality %q",
				route.ProviderName, route.Provider.Kind, config.ModalityVideoGen),
			hooks.StatusBadRequest)
	}
}
