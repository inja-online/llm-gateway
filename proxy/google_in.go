package proxy

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/inja-online/llm-gateway/canonical"
	"github.com/inja-online/llm-gateway/config"
	anthropicegress "github.com/inja-online/llm-gateway/egress/anthropic"
	googleegress "github.com/inja-online/llm-gateway/egress/google"
	openaiegress "github.com/inja-online/llm-gateway/egress/openai"
	googleingress "github.com/inja-online/llm-gateway/ingress/google"
	oaingress "github.com/inja-online/llm-gateway/ingress/openai"
	antingress "github.com/inja-online/llm-gateway/ingress/anthropic"
	"github.com/inja-online/llm-gateway/hooks"
	"github.com/inja-online/llm-gateway/internal/sse"
)

// handleGoogle serves native Gemini paths:
//
//	POST /v1beta/models/{action}
//
// where action is "{model}:generateContent" or "{model}:streamGenerateContent".
func (s *Server) handleGoogle(w http.ResponseWriter, r *http.Request) {
	x := s.newExchange(w, r, DialectGoogle, writeGoogleError)
	defer x.emit()

	action := r.PathValue("action")
	model, method, ok := parseGoogleAction(action)
	if !ok {
		x.fail(http.StatusNotFound, "invalid_request_error",
			"unknown google path; want models/{model}:generateContent or :streamGenerateContent",
			hooks.StatusBadRequest)
		return
	}
	stream := method == "streamGenerateContent"

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
		publicModel = head.Model
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
	x.ev.UpstreamModel = route.UpstreamModel
	if x.ev.UpstreamModel == "" {
		x.ev.UpstreamModel = model
	}

	switch providerKind(route.Provider) {
	case config.KindGoogle:
		s.googlePassthrough(x, route, body, stream)
	case config.KindOpenAI, config.KindOpenAICompat:
		s.googleToOpenAI(x, route, body, model, stream)
	case config.KindAnthropic:
		s.googleToAnthropic(x, route, body, model, stream)
	default:
		x.fail(http.StatusNotImplemented, "invalid_request_error",
			"translation to provider kind "+route.Provider.Kind+" is not implemented", hooks.StatusBadRequest)
	}
}

func parseGoogleAction(action string) (model, method string, ok bool) {
	const gen = ":generateContent"
	const stream = ":streamGenerateContent"
	switch {
	case strings.HasSuffix(action, stream):
		return strings.TrimSuffix(action, stream), "streamGenerateContent", true
	case strings.HasSuffix(action, gen):
		return strings.TrimSuffix(action, gen), "generateContent", true
	default:
		return "", "", false
	}
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
	x.w.Header().Set("Content-Type", "application/json")
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
