package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	oaingress "github.com/inja-online/llm-gateway/ingress/openai"

	"github.com/inja-online/llm-gateway/canonical"
	"github.com/inja-online/llm-gateway/config"
	anthropicegress "github.com/inja-online/llm-gateway/egress/anthropic"
	"github.com/inja-online/llm-gateway/hooks"
	"github.com/inja-online/llm-gateway/internal/sse"
)

// handleOpenAI serves POST /v1/chat/completions. It routes to a passthrough
// (OpenAI-wire upstream) or a translation path (Anthropic upstream).
func (s *Server) handleOpenAI(w http.ResponseWriter, r *http.Request) {
	x := s.newExchange(w, r, DialectOpenAI, writeOpenAIError)
	defer x.emit()

	body, ok := x.readBody()
	if !ok {
		return
	}

	// Peek the model and stream flag without a full parse.
	var head struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if json.Unmarshal(body, &head) != nil || head.Model == "" {
		x.fail(http.StatusBadRequest, "invalid_request_error", "missing or invalid required field: model", hooks.StatusBadRequest)
		return
	}
	x.ev.Model = head.Model
	x.ev.Stream = head.Stream

	route, err := Resolve(s.cfg, DialectOpenAI, head.Model)
	if err != nil {
		x.fail(http.StatusNotFound, "invalid_request_error", err.Error(), hooks.StatusBadRequest)
		return
	}
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = route.UpstreamModel

	switch providerKind(route.Provider) {
	case config.KindOpenAI, config.KindOpenAICompat:
		s.openAIPassthrough(x, route, body, head.Stream)
	case config.KindAnthropic:
		s.openAIToAnthropic(x, route, body)
	case config.KindGoogle:
		s.openAIToGoogle(x, route, body)
	default:
		x.fail(http.StatusNotImplemented, "invalid_request_error",
			"translation to provider kind "+route.Provider.Kind+" is not implemented", hooks.StatusBadRequest)
	}
}

// openAIPassthrough forwards an OpenAI request to an OpenAI-wire upstream,
// rewriting only the model id and injecting usage reporting for streams.
func (s *Server) openAIPassthrough(x *exchange, route Route, body []byte, stream bool) {
	var req map[string]any
	if json.Unmarshal(body, &req) != nil {
		x.fail(http.StatusBadRequest, "invalid_request_error", "request body is not valid JSON", hooks.StatusBadRequest)
		return
	}
	req["model"] = route.UpstreamModel
	if stream {
		ensureIncludeUsage(req)
	}
	upstreamBody, _ := json.Marshal(req)

	resp, ok := x.sendUpstream(route, "/chat/completions", upstreamBody)
	if !ok {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		x.forwardErrorResponse(resp)
		return
	}
	if stream {
		x.passthroughStream(resp, extractOpenAIUsage)
		return
	}
	s.forwardOpenAIJSON(x, resp)
}

func (s *Server) forwardOpenAIJSON(x *exchange, resp *http.Response) {
	body, err := readAll(resp)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to read upstream response", hooks.StatusUpstreamError)
		return
	}
	var parsed struct {
		Usage *struct {
			PromptTokens            int `json:"prompt_tokens"`
			CompletionTokens        int `json:"completion_tokens"`
			PromptTokensDetails     *struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			CompletionTokensDetails *struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Usage != nil {
		x.ev.TokensIn = parsed.Usage.PromptTokens
		x.ev.TokensOut = parsed.Usage.CompletionTokens
		if parsed.Usage.PromptTokensDetails != nil {
			x.ev.CachedTokens = parsed.Usage.PromptTokensDetails.CachedTokens
		}
		if parsed.Usage.CompletionTokensDetails != nil {
			x.ev.ReasoningTokens = parsed.Usage.CompletionTokensDetails.ReasoningTokens
		}
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

// openAIToAnthropic translates an OpenAI request to Anthropic, sends it, and
// translates the response/stream back to OpenAI form.
func (s *Server) openAIToAnthropic(x *exchange, route Route, body []byte) {
	req, err := oaingress.ParseRequest(body)
	if err != nil {
		x.fail(http.StatusBadRequest, "invalid_request_error", err.Error(), hooks.StatusBadRequest)
		return
	}
	x.noteDroppedFields(openaiTranslateDrops(body))
	x.setCacheAutoHeader(applyAutoBreakpoints(s.cfg, req))
	upstreamBody, err := anthropicegress.BuildRequest(req, route.UpstreamModel)
	if err != nil {
		x.fail(http.StatusBadRequest, "invalid_request_error", err.Error(), hooks.StatusBadRequest)
		return
	}
	resp, ok := x.sendUpstream(route, "/messages", upstreamBody)
	if !ok {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		// Translate Anthropic error envelope into an OpenAI one.
		s.translateAnthropicError(x, resp)
		return
	}

	created := time.Now().Unix()
	if req.Stream {
		s.streamAnthropicToOpenAI(x, resp, created)
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
	out, _ := oaingress.SerializeResponse(canon, created)
	x.applyCanonUsage(canon.Usage)
	x.ev.Status = hooks.StatusOK
	x.ev.HTTPStatus = http.StatusOK
	x.w.Header().Set("Content-Type", "application/json")
	x.w.WriteHeader(http.StatusOK)
	x.w.Write(out)
}

// streamAnthropicToOpenAI reads an Anthropic SSE stream and re-emits it as
// OpenAI chunks.
func (s *Server) streamAnthropicToOpenAI(x *exchange, resp *http.Response, created int64) {
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
	ser := oaingress.NewStreamSerializer(created)
	// Assume estimated until the upstream actually reports usage, so a stream
	// cut before the final event is never mistaken for a measured zero.
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
		x.w.Write(ser.Done())
		flusher.Flush()
	}
	x.finishStream(err)
}

// translateAnthropicError converts an Anthropic error body to an OpenAI one.
func (s *Server) translateAnthropicError(x *exchange, resp *http.Response) {
	body, _ := readAll(resp)
	var in struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	msg := "upstream error"
	code := "api_error"
	if json.Unmarshal(body, &in) == nil && in.Error.Message != "" {
		msg = in.Error.Message
		if in.Error.Type != "" {
			code = in.Error.Type
		}
	}
	x.ev.Status = hooks.StatusUpstreamError
	x.ev.HTTPStatus = writeOpenAIError(x.w, resp.StatusCode, code, msg)
}

func extractOpenAIUsage(data []byte, ev *hooks.UsageEvent) bool {
	if bytes.Equal(data, []byte("[DONE]")) {
		return false
	}
	var chunk struct {
		Usage *struct {
			PromptTokens            int `json:"prompt_tokens"`
			CompletionTokens        int `json:"completion_tokens"`
			PromptTokensDetails     *struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			CompletionTokensDetails *struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
	}
	if json.Unmarshal(data, &chunk) == nil && chunk.Usage != nil &&
		(chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0) {
		ev.TokensIn = chunk.Usage.PromptTokens
		ev.TokensOut = chunk.Usage.CompletionTokens
		if chunk.Usage.PromptTokensDetails != nil && chunk.Usage.PromptTokensDetails.CachedTokens > 0 {
			ev.CachedTokens = chunk.Usage.PromptTokensDetails.CachedTokens
		}
		if chunk.Usage.CompletionTokensDetails != nil && chunk.Usage.CompletionTokensDetails.ReasoningTokens > 0 {
			ev.ReasoningTokens = chunk.Usage.CompletionTokensDetails.ReasoningTokens
		}
		return true
	}
	return false
}

func ensureIncludeUsage(req map[string]any) {
	opts, _ := req["stream_options"].(map[string]any)
	if opts == nil {
		opts = map[string]any{}
	}
	if _, set := opts["include_usage"]; !set {
		opts["include_usage"] = true
	}
	req["stream_options"] = opts
}

func writeOpenAIError(w http.ResponseWriter, status int, code, msg string) int {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"message": msg, "type": code, "code": nil},
	})
	return status
}
