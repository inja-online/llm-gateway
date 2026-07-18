package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/inja-online/llm-gateway/canonical"
	"github.com/inja-online/llm-gateway/config"
	anthropicegress "github.com/inja-online/llm-gateway/egress/anthropic"
	googleegress "github.com/inja-online/llm-gateway/egress/google"
	openaiegress "github.com/inja-online/llm-gateway/egress/openai"
	"github.com/inja-online/llm-gateway/hooks"
	antingress "github.com/inja-online/llm-gateway/ingress/anthropic"
	googleingress "github.com/inja-online/llm-gateway/ingress/google"
	oaingress "github.com/inja-online/llm-gateway/ingress/openai"
	"github.com/inja-online/llm-gateway/internal/sse"
)

// handleGoogle serves native Gemini paths:
//
//	POST /v1beta/models/{action}
//
// where action is "{model}:generateContent", ":streamGenerateContent",
// ":countTokens", ":embedContent", or ":batchEmbedContents".
// countTokens does not emit a usage event.
func (s *Server) handleGoogle(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	model, method, ok := parseGoogleAction(action)
	if ok && method == "countTokens" {
		s.handleGoogleCountTokens(w, r, model)
		return
	}
	if ok && (method == "generateImages" || method == "predict" || method == "generateVideos" || method == "predictLongRunning") {
		s.handleGoogleMedia(w, r, model, method)
		return
	}

	x := s.newExchange(w, r, DialectGoogle, writeGoogleError)
	defer x.emit()

	if !ok {
		x.fail(http.StatusNotFound, "invalid_request_error",
			"unknown google path; want models/{model}:generateContent, :streamGenerateContent, :countTokens, :embedContent, or :batchEmbedContents",
			hooks.StatusBadRequest)
		return
	}
	stream := method == "streamGenerateContent"
	embed := isGoogleEmbedMethod(method)
	if embed {
		x.ev.Modality = ModalityEmbedding
	}

	body, ok := x.readBody()
	if !ok {
		return
	}

	// Prefer model in body when present; path is the native source of truth.
	var head struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &head)
	publicModel := model
	if head.Model != "" {
		// Body model may be provider/model for gateway routing.
		// Strip models/ prefix for Resolve (aliases use bare / provider ids).
		publicModel = strings.TrimPrefix(head.Model, "models/")
	}
	x.ev.Model = publicModel
	x.ev.Stream = stream

	route, err := Resolve(s.cfg, DialectGoogle, publicModel)
	if err != nil {
		// Fall back to path model for provider/model resolution.
		route, err = Resolve(s.cfg, DialectGoogle, model)
		if err != nil {
			x.fail(http.StatusNotFound, "invalid_request_error", err.Error(), hooks.StatusBadRequest)
			return
		}
		// Keep path model as upstream id when public was unresolved.
		if route.UpstreamModel == "" {
			route.UpstreamModel = model
		}
	}
	// If client used bare path model and resolved via default, upstream model is path model.
	if !strings.Contains(publicModel, "/") && s.cfg.Aliases[publicModel] == "" {
		// Resolve already set UpstreamModel correctly for bare ids.
	}
	// When routing used body "provider/model" but path had bare model, prefer route.
	x.ev.Provider = route.ProviderName
	if route.UpstreamModel == "" {
		route.UpstreamModel = model
	}
	// Path/body models never keep the Gemini "models/" resource prefix upstream.
	route.UpstreamModel = strings.TrimPrefix(route.UpstreamModel, "models/")
	x.ev.UpstreamModel = route.UpstreamModel

	switch providerKind(route.Provider) {
	case config.KindGoogle:
		if embed {
			s.googleEmbedPassthrough(x, route, body, method)
			return
		}
		s.googlePassthrough(x, route, body, stream)
	case config.KindOpenAI, config.KindOpenAICompat:
		if embed {
			x.fail(http.StatusNotImplemented, "invalid_request_error",
				"native google embeddings translation to openai is not implemented; use POST /v1/embeddings",
				hooks.StatusBadRequest)
			return
		}
		s.googleToOpenAI(x, route, body, model, stream)
	case config.KindAnthropic:
		if embed {
			x.fail(http.StatusNotImplemented, "invalid_request_error",
				"embeddings are not supported for anthropic providers",
				hooks.StatusBadRequest)
			return
		}
		s.googleToAnthropic(x, route, body, model, stream)
	default:
		x.fail(http.StatusNotImplemented, "invalid_request_error",
			"translation to provider kind "+route.Provider.Kind+" is not implemented", hooks.StatusBadRequest)
	}
}

func parseGoogleAction(action string) (model, method string, ok bool) {
	const stream = ":streamGenerateContent"
	const count = ":countTokens"
	const gen = ":generateContent"
	const batchEmbed = ":batchEmbedContents"
	const embed = ":embedContent"
	switch {
	case strings.HasSuffix(action, stream):
		return strings.TrimSuffix(action, stream), "streamGenerateContent", true
	case strings.HasSuffix(action, gen):
		return strings.TrimSuffix(action, gen), "generateContent", true
	case strings.HasSuffix(action, count):
		return strings.TrimSuffix(action, count), "countTokens", true
	case strings.HasSuffix(action, batchEmbed):
		return strings.TrimSuffix(action, batchEmbed), "batchEmbedContents", true
	case strings.HasSuffix(action, embed):
		return strings.TrimSuffix(action, embed), "embedContent", true
	case strings.HasSuffix(action, ":generateImages"):
		return strings.TrimSuffix(action, ":generateImages"), "generateImages", true
	case strings.HasSuffix(action, ":predictLongRunning"):
		return strings.TrimSuffix(action, ":predictLongRunning"), "predictLongRunning", true
	case strings.HasSuffix(action, ":predict"):
		return strings.TrimSuffix(action, ":predict"), "predict", true
	case strings.HasSuffix(action, ":generateVideos"):
		return strings.TrimSuffix(action, ":generateVideos"), "generateVideos", true
	default:
		return "", "", false
	}
}

// handleGoogleCountTokens proxies POST …/models/{model}:countTokens to kind:google.
// No usage event (same policy as Anthropic count_tokens).
func (s *Server) handleGoogleCountTokens(w http.ResponseWriter, r *http.Request, pathModel string) {
	body, err := readAllLimited(r)
	if err != nil {
		writeGoogleError(w, http.StatusBadRequest, "invalid_request_error", "failed to read request body")
		return
	}
	var head struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &head)
	publicModel := pathModel
	if head.Model != "" {
		publicModel = head.Model
	}
	route, rerr := Resolve(s.cfg, DialectGoogle, publicModel)
	if rerr != nil {
		route, rerr = Resolve(s.cfg, DialectGoogle, pathModel)
		if rerr != nil {
			writeGoogleError(w, http.StatusNotFound, "invalid_request_error", rerr.Error())
			return
		}
	}
	if route.UpstreamModel == "" {
		route.UpstreamModel = pathModel
	}
	if providerKind(route.Provider) != config.KindGoogle {
		writeGoogleError(w, http.StatusNotImplemented, "invalid_request_error",
			"countTokens requires a kind:google provider (got "+route.Provider.Kind+")")
		return
	}
	// Body is Google-shaped already; model lives in the path only.
	if !s.proxyGoogleCountTokens(w, r, route, body) {
		writeGoogleError(w, http.StatusBadGateway, "api_error", "upstream countTokens failed")
	}
}

// proxyGoogleCountTokens POSTs to the provider's :countTokens endpoint and
// relays a successful JSON response. Returns false on transport/4xx+/errors.
func (s *Server) proxyGoogleCountTokens(w http.ResponseWriter, r *http.Request, route Route, body []byte) bool {
	ctx, cancel := contextWithTimeout(r, 15*time.Second)
	defer cancel()
	upReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		route.Provider.BaseURL+googleegress.CountTokensPath(route.UpstreamModel), bytesReader(body))
	if err != nil {
		return false
	}
	upReq.Header.Set("Content-Type", "application/json")
	applyAuth(upReq, route.Provider, clientKey(r))

	resp, err := s.client.Do(upReq)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return false
	}
	out, err := readAll(resp)
	if err != nil {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(out)
	return true
}

// resolveGoogleProvider picks kind:google for discovery GETs via ?provider= or defaults.google_dialect.
func (s *Server) resolveGoogleProvider(r *http.Request) (Route, error) {
	provName := r.URL.Query().Get("provider")
	if provName == "" {
		provName = s.cfg.Defaults.GoogleDialect
	}
	if provName == "" {
		return Route{}, fmt.Errorf("models discovery requires ?provider=NAME or defaults.google_dialect")
	}
	p, ok := s.cfg.Providers[provName]
	if !ok {
		return Route{}, fmt.Errorf("unknown provider %q", provName)
	}
	if providerKind(p) != config.KindGoogle {
		return Route{}, fmt.Errorf("models discovery requires a kind:google provider (got %s)", p.Kind)
	}
	return Route{ProviderName: provName, Provider: p}, nil
}

// handleGoogleModelsList serves GET /v1beta/models — passthrough, no usage event.
func (s *Server) handleGoogleModelsList(w http.ResponseWriter, r *http.Request) {
	route, err := s.resolveGoogleProvider(r)
	if err != nil {
		writeGoogleError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	s.proxyGoogleGET(w, r, route, googleegress.ModelsPath())
}

// handleGoogleModelGet serves GET /v1beta/models/{model} — passthrough, no usage event.
func (s *Server) handleGoogleModelGet(w http.ResponseWriter, r *http.Request) {
	model := r.PathValue("model")
	if model == "" {
		writeGoogleError(w, http.StatusBadRequest, "invalid_request_error", "missing model id")
		return
	}
	// Allow gateway-style "provider/model" in the path segment by resolving first.
	if strings.Contains(model, "/") {
		if route, rerr := Resolve(s.cfg, DialectGoogle, model); rerr == nil && providerKind(route.Provider) == config.KindGoogle {
			s.proxyGoogleGET(w, r, route, googleegress.ModelPath(route.UpstreamModel))
			return
		}
	}
	route, err := s.resolveGoogleProvider(r)
	if err != nil {
		writeGoogleError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	// Strip optional "models/" resource prefix clients sometimes send.
	model = strings.TrimPrefix(model, "models/")
	s.proxyGoogleGET(w, r, route, googleegress.ModelPath(model))
}

// proxyGoogleGET forwards a GET to a kind:google provider and relays the response.
func (s *Server) proxyGoogleGET(w http.ResponseWriter, r *http.Request, route Route, path string) {
	ctx, cancel := contextWithTimeout(r, 15*time.Second)
	defer cancel()
	// Preserve query string except our routing-only "provider" key.
	q := r.URL.Query()
	q.Del("provider")
	upURL := route.Provider.BaseURL + path
	if enc := q.Encode(); enc != "" {
		upURL += "?" + enc
	}
	upReq, err := http.NewRequestWithContext(ctx, http.MethodGet, upURL, nil)
	if err != nil {
		writeGoogleError(w, http.StatusBadGateway, "api_error", "failed to build upstream request")
		return
	}
	applyAuth(upReq, route.Provider, clientKey(r))

	resp, err := s.client.Do(upReq)
	if err != nil {
		writeGoogleError(w, http.StatusBadGateway, "api_error", "upstream request failed: "+err.Error())
		return
	}
	defer resp.Body.Close()
	out, err := readAll(resp)
	if err != nil {
		writeGoogleError(w, http.StatusBadGateway, "api_error", "failed to read upstream response")
		return
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(resp.StatusCode)
	w.Write(out)
}

func (s *Server) googlePassthrough(x *exchange, route Route, body []byte, stream bool) {
	// Body is forwarded as-is (no model field required). Path carries upstream model.
	resp, ok := x.sendUpstream(route, googleegress.Path(route.UpstreamModel, stream), body)
	if !ok {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		x.forwardErrorResponse(resp)
		return
	}
	if stream {
		x.passthroughStream(resp, extractGoogleUsage)
		return
	}
	s.forwardGoogleJSON(x, resp)
}

func (s *Server) forwardGoogleJSON(x *exchange, resp *http.Response) {
	body, err := readAll(resp)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to read upstream response", hooks.StatusUpstreamError)
		return
	}
	canon, err := googleegress.ParseResponse(body)
	if err == nil && canon.Usage.HasUsage {
		x.applyCanonUsage(canon.Usage)
	} else {
		x.ev.Estimated = true
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

func (s *Server) googleToOpenAI(x *exchange, route Route, body []byte, pathModel string, stream bool) {
	req, err := googleingress.ParseRequest(body, pathModel)
	if err != nil {
		x.fail(http.StatusBadRequest, "invalid_request_error", err.Error(), hooks.StatusBadRequest)
		return
	}
	req.Stream = stream
	upstreamBody, err := openaiegress.BuildRequest(req, route.UpstreamModel)
	if err != nil {
		x.fail(http.StatusBadRequest, "invalid_request_error", "failed to build upstream request", hooks.StatusBadRequest)
		return
	}
	resp, ok := x.sendUpstream(route, "/chat/completions", upstreamBody)
	if !ok {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		s.translateOpenAIErrorToGoogle(x, resp)
		return
	}
	if stream {
		s.streamOpenAIToGoogle(x, resp)
		return
	}
	respBody, err := readAll(resp)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to read upstream response", hooks.StatusUpstreamError)
		return
	}
	canon, err := openaiegress.ParseResponse(respBody)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to parse upstream response", hooks.StatusUpstreamError)
		return
	}
	out, _ := googleingress.SerializeResponse(canon)
	x.applyCanonUsage(canon.Usage)
	x.ev.Status = hooks.StatusOK
	x.ev.HTTPStatus = http.StatusOK
	x.w.Header().Set("Content-Type", "application/json")
	x.w.WriteHeader(http.StatusOK)
	x.w.Write(out)
}

func (s *Server) googleToAnthropic(x *exchange, route Route, body []byte, pathModel string, stream bool) {
	req, err := googleingress.ParseRequest(body, pathModel)
	if err != nil {
		x.fail(http.StatusBadRequest, "invalid_request_error", err.Error(), hooks.StatusBadRequest)
		return
	}
	req.Stream = stream
	upstreamBody, err := anthropicegress.BuildRequest(req, route.UpstreamModel)
	if err != nil {
		x.fail(http.StatusBadRequest, "invalid_request_error", "failed to build upstream request", hooks.StatusBadRequest)
		return
	}
	resp, ok := x.sendUpstream(route, "/messages", upstreamBody)
	if !ok {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		s.translateAnthropicErrorToGoogle(x, resp)
		return
	}
	if stream {
		s.streamAnthropicToGoogle(x, resp)
		return
	}
	respBody, err := readAll(resp)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to read upstream response", hooks.StatusUpstreamError)
		return
	}
	canon, err := anthropicegress.ParseResponse(respBody)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to parse upstream response", hooks.StatusUpstreamError)
		return
	}
	out, _ := googleingress.SerializeResponse(canon)
	x.applyCanonUsage(canon.Usage)
	x.ev.Status = hooks.StatusOK
	x.ev.HTTPStatus = http.StatusOK
	x.w.Header().Set("Content-Type", "application/json")
	x.w.WriteHeader(http.StatusOK)
	x.w.Write(out)
}

func (s *Server) streamOpenAIToGoogle(x *exchange, resp *http.Response) {
	flusher, ok := x.w.(http.Flusher)
	if !ok {
		x.fail(http.StatusInternalServerError, "api_error", "streaming unsupported by server", hooks.StatusUpstreamError)
		return
	}
	x.w.Header().Set("Content-Type", "text/event-stream")
	x.w.Header().Set("Cache-Control", "no-cache")
	x.w.WriteHeader(http.StatusOK)
	x.ev.HTTPStatus = http.StatusOK

	parser := openaiegress.NewStreamParser()
	ser := googleingress.NewStreamSerializer()
	x.ev.Estimated = true
	firstByte := false

	write := func(evs []canonical.StreamEvent) error {
		for _, cev := range evs {
			if cev.Type == canonical.EventFinish {
				x.applyCanonUsage(cev.Usage)
			}
			if out := ser.Event(cev); out != nil {
				if _, werr := x.w.Write(out); werr != nil {
					return werr
				}
				flusher.Flush()
			}
		}
		return nil
	}

	err := sse.Scan(resp.Body, func(line []byte) error {
		if !firstByte {
			firstByte = true
			x.ev.TTFTMS = time.Since(x.start).Milliseconds()
		}
		data := sse.Data(line)
		if data == nil {
			return nil
		}
		return write(parser.Parse(data))
	})
	if err == nil {
		_ = write(parser.Finish())
	}
	x.finishStream(err)
}

func (s *Server) streamAnthropicToGoogle(x *exchange, resp *http.Response) {
	flusher, ok := x.w.(http.Flusher)
	if !ok {
		x.fail(http.StatusInternalServerError, "api_error", "streaming unsupported by server", hooks.StatusUpstreamError)
		return
	}
	x.w.Header().Set("Content-Type", "text/event-stream")
	x.w.Header().Set("Cache-Control", "no-cache")
	x.w.WriteHeader(http.StatusOK)
	x.ev.HTTPStatus = http.StatusOK

	parser := anthropicegress.NewStreamParser()
	ser := googleingress.NewStreamSerializer()
	x.ev.Estimated = true
	firstByte := false
	err := sse.Scan(resp.Body, func(line []byte) error {
		if !firstByte {
			firstByte = true
			x.ev.TTFTMS = time.Since(x.start).Milliseconds()
		}
		data := sse.Data(line)
		if data == nil {
			return nil
		}
		for _, cev := range parser.Parse(data) {
			if cev.Type == canonical.EventFinish {
				x.applyCanonUsage(cev.Usage)
			}
			if out := ser.Event(cev); out != nil {
				if _, werr := x.w.Write(out); werr != nil {
					return werr
				}
				flusher.Flush()
			}
		}
		return nil
	})
	x.finishStream(err)
}

// openAIToGoogle translates OpenAI → native Gemini.
func (s *Server) openAIToGoogle(x *exchange, route Route, body []byte) {
	req, err := oaingress.ParseRequest(body)
	if err != nil {
		x.fail(http.StatusBadRequest, "invalid_request_error", err.Error(), hooks.StatusBadRequest)
		return
	}
	upstreamBody, err := googleegress.BuildRequest(req, route.UpstreamModel)
	if err != nil {
		x.fail(http.StatusBadRequest, "invalid_request_error", "failed to build upstream request", hooks.StatusBadRequest)
		return
	}
	resp, ok := x.sendUpstream(route, googleegress.Path(route.UpstreamModel, req.Stream), upstreamBody)
	if !ok {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		s.translateGoogleErrorToOpenAI(x, resp)
		return
	}
	created := time.Now().Unix()
	if req.Stream {
		s.streamGoogleToOpenAI(x, resp, created)
		return
	}
	respBody, err := readAll(resp)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to read upstream response", hooks.StatusUpstreamError)
		return
	}
	canon, err := googleegress.ParseResponse(respBody)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to parse upstream response", hooks.StatusUpstreamError)
		return
	}
	out, _ := oaingress.SerializeResponse(canon, created)
	x.applyCanonUsage(canon.Usage)
	x.ev.Status = hooks.StatusOK
	x.ev.HTTPStatus = http.StatusOK
	x.w.Header().Set("Content-Type", "application/json")
	x.w.WriteHeader(http.StatusOK)
	x.w.Write(out)
}

// anthropicToGoogle translates Anthropic → native Gemini.
func (s *Server) anthropicToGoogle(x *exchange, route Route, body []byte) {
	req, err := antingress.ParseRequest(body)
	if err != nil {
		x.fail(http.StatusBadRequest, "invalid_request_error", err.Error(), hooks.StatusBadRequest)
		return
	}
	upstreamBody, err := googleegress.BuildRequest(req, route.UpstreamModel)
	if err != nil {
		x.fail(http.StatusBadRequest, "invalid_request_error", "failed to build upstream request", hooks.StatusBadRequest)
		return
	}
	resp, ok := x.sendUpstream(route, googleegress.Path(route.UpstreamModel, req.Stream), upstreamBody)
	if !ok {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		s.translateGoogleErrorToAnthropic(x, resp)
		return
	}
	if req.Stream {
		s.streamGoogleToAnthropic(x, resp)
		return
	}
	respBody, err := readAll(resp)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to read upstream response", hooks.StatusUpstreamError)
		return
	}
	canon, err := googleegress.ParseResponse(respBody)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to parse upstream response", hooks.StatusUpstreamError)
		return
	}
	out, _ := antingress.SerializeResponse(canon)
	x.applyCanonUsage(canon.Usage)
	x.ev.Status = hooks.StatusOK
	x.ev.HTTPStatus = http.StatusOK
	x.w.Header().Set("Content-Type", "application/json")
	x.w.WriteHeader(http.StatusOK)
	x.w.Write(out)
}

func (s *Server) streamGoogleToOpenAI(x *exchange, resp *http.Response, created int64) {
	flusher, ok := x.w.(http.Flusher)
	if !ok {
		x.fail(http.StatusInternalServerError, "api_error", "streaming unsupported by server", hooks.StatusUpstreamError)
		return
	}
	x.w.Header().Set("Content-Type", "text/event-stream")
	x.w.Header().Set("Cache-Control", "no-cache")
	x.w.WriteHeader(http.StatusOK)
	x.ev.HTTPStatus = http.StatusOK

	parser := googleegress.NewStreamParser()
	ser := oaingress.NewStreamSerializer(created)
	x.ev.Estimated = true
	firstByte := false
	err := sse.Scan(resp.Body, func(line []byte) error {
		if !firstByte {
			firstByte = true
			x.ev.TTFTMS = time.Since(x.start).Milliseconds()
		}
		data := sse.Data(line)
		if data == nil {
			return nil
		}
		for _, cev := range parser.Parse(data) {
			if cev.Type == canonical.EventFinish {
				x.applyCanonUsage(cev.Usage)
			}
			if out := ser.Event(cev); out != nil {
				if _, werr := x.w.Write(out); werr != nil {
					return werr
				}
				flusher.Flush()
			}
		}
		return nil
	})
	if err == nil {
		for _, cev := range parser.Finish() {
			if cev.Type == canonical.EventFinish {
				x.applyCanonUsage(cev.Usage)
			}
			if out := ser.Event(cev); out != nil {
				x.w.Write(out)
				flusher.Flush()
			}
		}
		x.w.Write(ser.Done())
		flusher.Flush()
	}
	x.finishStream(err)
}

func (s *Server) streamGoogleToAnthropic(x *exchange, resp *http.Response) {
	flusher, ok := x.w.(http.Flusher)
	if !ok {
		x.fail(http.StatusInternalServerError, "api_error", "streaming unsupported by server", hooks.StatusUpstreamError)
		return
	}
	x.w.Header().Set("Content-Type", "text/event-stream")
	x.w.Header().Set("Cache-Control", "no-cache")
	x.w.WriteHeader(http.StatusOK)
	x.ev.HTTPStatus = http.StatusOK

	parser := googleegress.NewStreamParser()
	ser := antingress.NewStreamSerializer()
	x.ev.Estimated = true
	firstByte := false

	write := func(evs []canonical.StreamEvent) error {
		for _, cev := range evs {
			if cev.Type == canonical.EventFinish {
				x.applyCanonUsage(cev.Usage)
			}
			if out := ser.Event(cev); out != nil {
				if _, werr := x.w.Write(out); werr != nil {
					return werr
				}
				flusher.Flush()
			}
		}
		return nil
	}

	err := sse.Scan(resp.Body, func(line []byte) error {
		if !firstByte {
			firstByte = true
			x.ev.TTFTMS = time.Since(x.start).Milliseconds()
		}
		data := sse.Data(line)
		if data == nil {
			return nil
		}
		return write(parser.Parse(data))
	})
	if err == nil {
		_ = write(parser.Finish())
	}
	x.finishStream(err)
}

func extractGoogleUsage(data []byte, ev *hooks.UsageEvent) bool {
	var env struct {
		UsageMetadata *struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			CachedContent        int `json:"cachedContentTokenCount"`
			Thoughts             int `json:"thoughtsTokenCount"`
			PromptSnake          int `json:"prompt_token_count"`
			CandidatesSnake      int `json:"candidates_token_count"`
			CachedSnake          int `json:"cached_content_token_count"`
			ThoughtsSnake        int `json:"thoughts_token_count"`
		} `json:"usageMetadata"`
		UsageSnake *struct {
			PromptTokenCount     int `json:"prompt_token_count"`
			CandidatesTokenCount int `json:"candidates_token_count"`
			CachedContent        int `json:"cached_content_token_count"`
			Thoughts             int `json:"thoughts_token_count"`
		} `json:"usage_metadata"`
	}
	if json.Unmarshal(data, &env) != nil {
		return false
	}
	in, out, cached, reasoning := 0, 0, 0, 0
	if env.UsageMetadata != nil {
		in = env.UsageMetadata.PromptTokenCount
		out = env.UsageMetadata.CandidatesTokenCount
		cached = env.UsageMetadata.CachedContent
		reasoning = env.UsageMetadata.Thoughts
		if in == 0 {
			in = env.UsageMetadata.PromptSnake
		}
		if out == 0 {
			out = env.UsageMetadata.CandidatesSnake
		}
		if cached == 0 {
			cached = env.UsageMetadata.CachedSnake
		}
		if reasoning == 0 {
			reasoning = env.UsageMetadata.ThoughtsSnake
		}
	}
	if env.UsageSnake != nil {
		if in == 0 {
			in = env.UsageSnake.PromptTokenCount
		}
		if out == 0 {
			out = env.UsageSnake.CandidatesTokenCount
		}
		if cached == 0 {
			cached = env.UsageSnake.CachedContent
		}
		if reasoning == 0 {
			reasoning = env.UsageSnake.Thoughts
		}
	}
	if in == 0 && out == 0 {
		return false
	}
	ev.TokensIn = in
	ev.TokensOut = out
	if cached > 0 {
		ev.CachedTokens = cached
	}
	if reasoning > 0 {
		ev.ReasoningTokens = reasoning
	}
	return true
}

func writeGoogleError(w http.ResponseWriter, status int, code, msg string) int {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    status,
			"message": msg,
			"status":  code,
		},
	})
	return status
}

func (s *Server) translateGoogleErrorToOpenAI(x *exchange, resp *http.Response) {
	body, _ := readAll(resp)
	msg, code := parseGoogleError(body)
	x.ev.Status = hooks.StatusUpstreamError
	x.ev.HTTPStatus = writeOpenAIError(x.w, resp.StatusCode, code, msg)
}

func (s *Server) translateGoogleErrorToAnthropic(x *exchange, resp *http.Response) {
	body, _ := readAll(resp)
	msg, code := parseGoogleError(body)
	x.ev.Status = hooks.StatusUpstreamError
	x.ev.HTTPStatus = writeAnthropicError(x.w, resp.StatusCode, code, msg)
}

func (s *Server) translateOpenAIErrorToGoogle(x *exchange, resp *http.Response) {
	body, _ := readAll(resp)
	var in struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	msg, code := "upstream error", "api_error"
	if json.Unmarshal(body, &in) == nil && in.Error.Message != "" {
		msg = in.Error.Message
		if in.Error.Type != "" {
			code = in.Error.Type
		}
	}
	x.ev.Status = hooks.StatusUpstreamError
	x.ev.HTTPStatus = writeGoogleError(x.w, resp.StatusCode, code, msg)
}

func (s *Server) translateAnthropicErrorToGoogle(x *exchange, resp *http.Response) {
	body, _ := readAll(resp)
	var in struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	msg, code := "upstream error", "api_error"
	if json.Unmarshal(body, &in) == nil && in.Error.Message != "" {
		msg = in.Error.Message
		if in.Error.Type != "" {
			code = in.Error.Type
		}
	}
	x.ev.Status = hooks.StatusUpstreamError
	x.ev.HTTPStatus = writeGoogleError(x.w, resp.StatusCode, code, msg)
}

func parseGoogleError(body []byte) (msg, code string) {
	msg, code = "upstream error", "api_error"
	var in struct {
		Error struct {
			Message string `json:"message"`
			Status  string `json:"status"`
			Code    int    `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &in) == nil && in.Error.Message != "" {
		msg = in.Error.Message
		if in.Error.Status != "" {
			code = in.Error.Status
		}
	}
	return msg, code
}
