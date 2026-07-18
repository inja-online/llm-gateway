package proxy

import (
	"encoding/json"
	"net/http"

	"github.com/inja-online/llm-gateway/canonical"
	"github.com/inja-online/llm-gateway/config"
	openaiegress "github.com/inja-online/llm-gateway/egress/openai"
	"github.com/inja-online/llm-gateway/hooks"
	antingress "github.com/inja-online/llm-gateway/ingress/anthropic"
	"github.com/inja-online/llm-gateway/internal/sse"
)

// handleAnthropic serves POST /v1/messages — the endpoint Claude Code and the
// Anthropic SDK talk to.
func (s *Server) handleAnthropic(w http.ResponseWriter, r *http.Request) {
	x := s.newExchange(w, r, DialectAnthropic, writeAnthropicError)
	defer x.emit()

	body, ok := x.readBody()
	if !ok {
		return
	}
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

	route, err := Resolve(s.cfg, DialectAnthropic, head.Model)
	if err != nil {
		x.fail(http.StatusNotFound, "invalid_request_error", err.Error(), hooks.StatusBadRequest)
		return
	}
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = route.UpstreamModel

	switch providerKind(route.Provider) {
	case config.KindAnthropic:
		s.anthropicPassthrough(x, route, body, head.Stream)
	case config.KindOpenAI, config.KindOpenAICompat:
		s.anthropicToOpenAI(x, route, body)
	default:
		x.fail(http.StatusNotImplemented, "invalid_request_error",
			"translation to provider kind "+route.Provider.Kind+" is not implemented", hooks.StatusBadRequest)
	}
}

// anthropicPassthrough forwards an Anthropic request to an Anthropic upstream,
// rewriting only the model id. This is the full-fidelity path Claude Code uses.
func (s *Server) anthropicPassthrough(x *exchange, route Route, body []byte, stream bool) {
	var req map[string]any
	if json.Unmarshal(body, &req) != nil {
		x.fail(http.StatusBadRequest, "invalid_request_error", "request body is not valid JSON", hooks.StatusBadRequest)
		return
	}
	req["model"] = route.UpstreamModel
	upstreamBody, _ := json.Marshal(req)

	resp, ok := x.sendUpstream(route, "/messages", upstreamBody)
	if !ok {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		x.forwardErrorResponse(resp)
		return
	}
	if stream {
		x.passthroughStream(resp, extractAnthropicUsage)
		return
	}
	s.forwardAnthropicJSON(x, resp)
}

func (s *Server) forwardAnthropicJSON(x *exchange, resp *http.Response) {
	body, err := readAll(resp)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to read upstream response", hooks.StatusUpstreamError)
		return
	}
	var parsed struct {
		Usage *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Usage != nil {
		x.ev.TokensIn = parsed.Usage.InputTokens
		x.ev.TokensOut = parsed.Usage.OutputTokens
	} else {
		x.ev.Estimated = true
	}
	x.ev.Status = hooks.StatusOK
	x.ev.HTTPStatus = resp.StatusCode
	x.w.Header().Set("Content-Type", "application/json")
	x.w.WriteHeader(resp.StatusCode)
	x.w.Write(body)
}

// anthropicToOpenAI translates an Anthropic request to OpenAI, sends it, and
// translates the response/stream back to Anthropic form.
func (s *Server) anthropicToOpenAI(x *exchange, route Route, body []byte) {
	req, err := antingress.ParseRequest(body)
	if err != nil {
		x.fail(http.StatusBadRequest, "invalid_request_error", err.Error(), hooks.StatusBadRequest)
		return
	}
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
		s.translateOpenAIError(x, resp)
		return
	}
	if req.Stream {
		s.streamOpenAIToAnthropic(x, resp)
		return
	}
	respBody, err := readAll(resp)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to read upstream response", hooks.StatusUpstreamError)
		return
	}
	canonResp, err := openaiegress.ParseResponse(respBody)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to parse upstream response", hooks.StatusUpstreamError)
		return
	}
	out, _ := antingress.SerializeResponse(canonResp)
	x.ev.TokensIn = canonResp.Usage.InputTokens
	x.ev.TokensOut = canonResp.Usage.OutputTokens
	x.ev.Estimated = !canonResp.Usage.HasUsage
	x.ev.Status = hooks.StatusOK
	x.ev.HTTPStatus = http.StatusOK
	x.w.Header().Set("Content-Type", "application/json")
	x.w.WriteHeader(http.StatusOK)
	x.w.Write(out)
}

// streamOpenAIToAnthropic reads an OpenAI SSE stream and re-emits it as
// Anthropic named events.
func (s *Server) streamOpenAIToAnthropic(x *exchange, resp *http.Response) {
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
	ser := antingress.NewStreamSerializer()
	x.ev.Estimated = true
	firstByte := false

	write := func(evs []canonical.StreamEvent) error {
		for _, cev := range evs {
			if cev.Type == canonical.EventFinish {
				x.ev.TokensIn = cev.Usage.InputTokens
				x.ev.TokensOut = cev.Usage.OutputTokens
				x.ev.Estimated = !cev.Usage.HasUsage
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
			x.ev.TTFTMS = timeSinceMS(x.start)
		}
		data := sse.Data(line)
		if data == nil {
			return nil
		}
		return write(parser.Parse(data))
	})
	if err == nil {
		// Flush terminal events if the upstream ended without a [DONE].
		write(parser.Finish())
	}
	x.finishStream(err)
}

// translateOpenAIError converts an OpenAI error body into the Anthropic shape.
func (s *Server) translateOpenAIError(x *exchange, resp *http.Response) {
	body, _ := readAll(resp)
	var in struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
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
	x.ev.HTTPStatus = writeAnthropicError(x.w, resp.StatusCode, code, msg)
}

func extractAnthropicUsage(data []byte, ev *hooks.UsageEvent) bool {
	var env struct {
		Type    string `json:"type"`
		Message *struct {
			Usage *struct {
				InputTokens int `json:"input_tokens"`
			} `json:"usage"`
		} `json:"message"`
		Usage *struct {
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(data, &env) != nil {
		return false
	}
	found := false
	if env.Message != nil && env.Message.Usage != nil && env.Message.Usage.InputTokens > 0 {
		ev.TokensIn = env.Message.Usage.InputTokens
		found = true
	}
	if env.Usage != nil && env.Usage.OutputTokens > 0 {
		ev.TokensOut = env.Usage.OutputTokens
		found = true
	}
	return found
}

// writeAnthropicError renders the Anthropic error envelope.
func writeAnthropicError(w http.ResponseWriter, status int, code, msg string) int {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"type":  "error",
		"error": map[string]any{"type": code, "message": msg},
	})
	return status
}
