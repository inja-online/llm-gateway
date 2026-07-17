package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mamad/llm-gateway/config"
	"github.com/mamad/llm-gateway/hooks"
	"github.com/mamad/llm-gateway/internal/sse"
)

const maxBodyBytes = 32 << 20 // 32 MiB

// openAIUsage matches the usage object in OpenAI-style responses and stream chunks.
type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// handleOpenAIPassthrough proxies /v1/chat/completions to an OpenAI-wire
// upstream (kinds: openai, openai_compat) with no canonical translation.
func (s *Server) handleOpenAIPassthrough(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ev := hooks.UsageEvent{
		RequestID: newRequestID(),
		Time:      start,
		DialectIn: DialectOpenAI,
	}
	// Exactly-one-event invariant: every return path below goes through emit.
	emitted := false
	emit := func() {
		if emitted {
			return
		}
		emitted = true
		ev.LatencyMS = time.Since(start).Milliseconds()
		s.hook.OnUsage(context.WithoutCancel(r.Context()), ev)
	}
	defer emit()

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		ev.Status = hooks.StatusBadRequest
		ev.HTTPStatus = writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "failed to read request body")
		return
	}

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		ev.Status = hooks.StatusBadRequest
		ev.HTTPStatus = writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "request body is not valid JSON")
		return
	}
	model, _ := req["model"].(string)
	if model == "" {
		ev.Status = hooks.StatusBadRequest
		ev.HTTPStatus = writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing required field: model")
		return
	}
	stream, _ := req["stream"].(bool)
	ev.Model = model
	ev.Stream = stream

	route, err := Resolve(s.cfg, DialectOpenAI, model)
	if err != nil {
		ev.Status = hooks.StatusBadRequest
		ev.HTTPStatus = writeOpenAIError(w, http.StatusNotFound, "invalid_request_error", err.Error())
		return
	}
	ev.Provider = route.ProviderName
	ev.UpstreamModel = route.UpstreamModel

	switch route.Provider.Kind {
	case config.KindOpenAI, config.KindOpenAICompat:
	default:
		ev.Status = hooks.StatusBadRequest
		ev.HTTPStatus = writeOpenAIError(w, http.StatusNotImplemented, "invalid_request_error",
			fmt.Sprintf("cross-dialect translation to provider kind %q is not implemented yet", route.Provider.Kind))
		return
	}

	// Rewrite model to the upstream id; ask for usage in streams so metering works.
	req["model"] = route.UpstreamModel
	if stream {
		ensureIncludeUsage(req)
	}
	upstreamBody, err := json.Marshal(req)
	if err != nil {
		ev.Status = hooks.StatusBadRequest
		ev.HTTPStatus = writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "failed to re-encode request body")
		return
	}

	key := clientKey(r)
	ev.KeyHash = hashKey(key)

	upReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		route.Provider.BaseURL+"/chat/completions", bytes.NewReader(upstreamBody))
	if err != nil {
		ev.Status = hooks.StatusUpstreamError
		ev.HTTPStatus = writeOpenAIError(w, http.StatusBadGateway, "api_error", "failed to build upstream request")
		return
	}
	upReq.Header.Set("Content-Type", "application/json")
	applyAuth(upReq, route.Provider, key)

	resp, err := s.client.Do(upReq)
	if err != nil {
		if errors.Is(r.Context().Err(), context.Canceled) {
			ev.Status = hooks.StatusClientAbort
			ev.HTTPStatus = 499
			return
		}
		ev.Status = hooks.StatusUpstreamError
		ev.HTTPStatus = writeOpenAIError(w, http.StatusBadGateway, "api_error", "upstream request failed: "+err.Error())
		return
	}
	defer resp.Body.Close()

	ev.HTTPStatus = resp.StatusCode
	if resp.StatusCode >= 400 {
		ev.Status = hooks.StatusUpstreamError
		copyResponse(w, resp)
		return
	}

	if !stream {
		s.forwardJSON(w, resp, &ev)
		return
	}
	s.forwardStream(w, r, resp, &ev, start)
}

// forwardJSON relays a non-streaming upstream response and extracts usage.
func (s *Server) forwardJSON(w http.ResponseWriter, resp *http.Response, ev *hooks.UsageEvent) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		ev.Status = hooks.StatusUpstreamError
		ev.HTTPStatus = writeOpenAIError(w, http.StatusBadGateway, "api_error", "failed to read upstream response")
		return
	}
	var parsed struct {
		Usage *openAIUsage `json:"usage"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Usage != nil {
		ev.TokensIn = parsed.Usage.PromptTokens
		ev.TokensOut = parsed.Usage.CompletionTokens
	} else {
		ev.Estimated = true
	}
	ev.Status = hooks.StatusOK
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

// forwardStream relays SSE bytes verbatim while scanning data lines for usage.
func (s *Server) forwardStream(w http.ResponseWriter, r *http.Request, resp *http.Response, ev *hooks.UsageEvent, start time.Time) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		ev.Status = hooks.StatusUpstreamError
		ev.HTTPStatus = writeOpenAIError(w, http.StatusInternalServerError, "api_error", "streaming unsupported by server")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(resp.StatusCode)

	sawUsage := false
	firstByte := false
	err := sse.Scan(resp.Body, func(line []byte) error {
		if !firstByte {
			firstByte = true
			ev.TTFTMS = time.Since(start).Milliseconds()
		}
		if data := sse.Data(line); data != nil && !bytes.Equal(data, []byte("[DONE]")) {
			var chunk struct {
				Usage *openAIUsage `json:"usage"`
			}
			if json.Unmarshal(data, &chunk) == nil && chunk.Usage != nil &&
				(chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0) {
				ev.TokensIn = chunk.Usage.PromptTokens
				ev.TokensOut = chunk.Usage.CompletionTokens
				sawUsage = true
			}
		}
		if _, werr := w.Write(line); werr != nil {
			return werr
		}
		flusher.Flush()
		return nil
	})

	ev.Estimated = !sawUsage
	switch {
	case err == nil:
		ev.Status = hooks.StatusOK
	case errors.Is(r.Context().Err(), context.Canceled):
		ev.Status = hooks.StatusClientAbort
	default:
		ev.Status = hooks.StatusUpstreamError
	}
}

// ensureIncludeUsage sets stream_options.include_usage=true without clobbering
// other user-provided stream options.
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

func copyResponse(w http.ResponseWriter, resp *http.Response) {
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, io.LimitReader(resp.Body, maxBodyBytes))
}

func writeOpenAIError(w http.ResponseWriter, status int, errType, msg string) int {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"message": msg, "type": errType, "code": nil},
	})
	return status
}
