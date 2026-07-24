package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/inja-online/llm-gateway/canonical"
	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
	"github.com/inja-online/llm-gateway/internal/sse"
)

// maxBodyBytes is the package default (32 MiB). Prefer exchange.bodyLimit() /
// Server.bodyLimit() so config.max_body_bytes is honored.
const maxBodyBytes = config.DefaultMaxBodyBytes

// errWriter renders an error body in a specific dialect's envelope and returns
// the HTTP status it wrote.
type errWriter func(w http.ResponseWriter, status int, code, msg string) int

// exchange carries per-request state and guarantees exactly one usage event.
type exchange struct {
	s        *Server
	w        http.ResponseWriter
	r        *http.Request
	start    time.Time
	ev       hooks.UsageEvent
	writeErr errWriter
	emitted  bool
}

// bodyLimit returns the configured request/response body cap for this exchange.
func (x *exchange) bodyLimit() int64 {
	if x != nil && x.s != nil {
		return x.s.bodyLimit()
	}
	return maxBodyBytes
}

// bodyLimit returns the configured request/response body cap for this server.
func (s *Server) bodyLimit() int64 {
	if s != nil && s.cfg != nil {
		return s.cfg.BodyLimit()
	}
	return maxBodyBytes
}

func (s *Server) newExchange(w http.ResponseWriter, r *http.Request, dialect string, writeErr errWriter) *exchange {
	return &exchange{
		s:     s,
		w:     w,
		r:     r,
		start: time.Now(),
		ev: hooks.UsageEvent{
			RequestID: newRequestID(),
			Time:      time.Now(),
			DialectIn: dialect,
		},
		writeErr: writeErr,
	}
}

// emit sends the usage event exactly once.
func (x *exchange) emit() {
	if x.emitted {
		return
	}
	x.emitted = true
	x.ev.LatencyMS = time.Since(x.start).Milliseconds()
	// Always print a human-readable line so operators see usage in gateway.log
	// (jsonl hooks may also write structured lines to stdout/file).
	log.Print(formatUsageLog(x.ev))
	if x.s != nil && x.s.hook != nil {
		x.s.hook.OnUsage(context.WithoutCancel(x.r.Context()), x.ev)
	}
}

// formatUsageLog builds one operator-facing summary line (no secrets / bodies).
func formatUsageLog(ev hooks.UsageEvent) string {
	status := ev.Status
	if status == "" {
		status = "unknown"
	}
	mod := ev.Modality
	if mod == "" {
		mod = "text"
	}
	stream := "false"
	if ev.Stream {
		stream = "true"
	}
	est := ""
	if ev.Estimated {
		est = " estimated"
	}
	return fmt.Sprintf(
		"usage status=%s modality=%s dialect=%s provider=%s model=%s upstream=%s tokens_in=%d tokens_out=%d cached=%d http=%d lat_ms=%d stream=%s req=%s%s",
		status,
		mod,
		ev.DialectIn,
		ev.Provider,
		ev.Model,
		ev.UpstreamModel,
		ev.TokensIn,
		ev.TokensOut,
		ev.CachedTokens,
		ev.HTTPStatus,
		ev.LatencyMS,
		stream,
		ev.RequestID,
		est,
	)
}

// applyCanonUsage copies token totals and optional detail fields from
// canonical usage into the exchange event.
func (x *exchange) applyCanonUsage(u canonical.Usage) {
	x.ev.TokensIn = u.InputTokens
	x.ev.TokensOut = u.OutputTokens
	x.ev.Estimated = !u.HasUsage
	if u.CacheReadTokens > 0 {
		x.ev.CachedTokens = u.CacheReadTokens
	}
	if u.CacheWriteTokens > 0 {
		x.ev.CacheWriteTokens = u.CacheWriteTokens
	}
	if u.ReasoningTokens > 0 {
		x.ev.ReasoningTokens = u.ReasoningTokens
	}
}

// fail writes a dialect error, records status, and marks the event.
func (x *exchange) fail(httpStatus int, code, msg, evStatus string) {
	x.ev.Status = evStatus
	x.ev.HTTPStatus = x.writeErr(x.w, httpStatus, code, msg)
}

// readBody reads and size-limits the request body. Bodies larger than
// max_body_bytes yield HTTP 413 with a dialect-shaped error envelope.
func (x *exchange) readBody() ([]byte, bool) {
	limit := x.bodyLimit()
	// Read one byte past the limit so oversize is distinguishable from exact-limit.
	body, err := io.ReadAll(io.LimitReader(x.r.Body, limit+1))
	if err != nil {
		x.fail(http.StatusBadRequest, "invalid_request_error", "failed to read request body", hooks.StatusBadRequest)
		return nil, false
	}
	if int64(len(body)) > limit {
		x.fail(http.StatusRequestEntityTooLarge, "invalid_request_error",
			fmt.Sprintf("request body exceeds max_body_bytes (%d)", limit),
			hooks.StatusBadRequest)
		return nil, false
	}
	return body, true
}

// sendUpstream builds and sends the upstream POST, applying auth and the
// forwarded (or env-override) key. On transport failure it records the event
// and writes a dialect error, returning ok=false.
func (x *exchange) sendUpstream(route Route, path string, body []byte) (*http.Response, bool) {
	return x.sendUpstreamRaw(route, http.MethodPost, path, body, "application/json")
}

// prepareResponseHeaders copies allowlisted upstream headers and sets the
// gateway correlation id before WriteHeader.
func (x *exchange) prepareResponseHeaders(resp *http.Response) {
	copyAllowlistedResponseHeaders(x.w.Header(), resp.Header)
	setGatewayRequestID(x.w, x.ev.RequestID)
}

// forwardErrorResponse relays a >=400 upstream response verbatim.
func (x *exchange) forwardErrorResponse(resp *http.Response) {
	x.ev.Status = hooks.StatusUpstreamError
	x.ev.HTTPStatus = resp.StatusCode
	x.prepareResponseHeaders(resp)
	if x.w.Header().Get("Content-Type") == "" {
		x.w.Header().Set("Content-Type", "application/json")
	}
	x.w.WriteHeader(resp.StatusCode)
	io.Copy(x.w, io.LimitReader(resp.Body, x.bodyLimit()))
}

// usageExtractor pulls token counts out of a data payload during a passthrough
// stream. It returns true once usage is found.
type usageExtractor func(data []byte, ev *hooks.UsageEvent) bool

// passthroughStream relays SSE bytes verbatim while scanning data lines for
// usage. done payloads (e.g. "[DONE]") are handled by extract returning false.
func (x *exchange) passthroughStream(resp *http.Response, extract usageExtractor) {
	x.passthroughStreamMap(resp, extract, nil)
}

// passthroughStreamMap is like passthroughStream but rewrites Claude OAuth tool
// names on each SSE line using reverseMap (may be nil).
func (x *exchange) passthroughStreamMap(resp *http.Response, extract usageExtractor, reverse map[string]string) {
	flusher, ok := x.w.(http.Flusher)
	if !ok {
		x.fail(http.StatusInternalServerError, "api_error", "streaming unsupported by server", hooks.StatusUpstreamError)
		return
	}
	x.prepareResponseHeaders(resp)
	if x.w.Header().Get("Content-Type") == "" {
		x.w.Header().Set("Content-Type", "text/event-stream")
	}
	x.w.Header().Set("Cache-Control", "no-cache")
	x.w.WriteHeader(resp.StatusCode)
	x.ev.HTTPStatus = resp.StatusCode

	sawUsage := false
	firstByte := false
	err := sse.Scan(resp.Body, func(line []byte) error {
		if !firstByte {
			firstByte = true
			x.ev.TTFTMS = time.Since(x.start).Milliseconds()
		}
		if data := sse.Data(line); data != nil {
			if extract(data, &x.ev) {
				sawUsage = true
			}
		}
		out := line
		if len(reverse) > 0 {
			out = restoreClaudeOAuthStreamLine(line, reverse)
		}
		if _, werr := x.w.Write(out); werr != nil {
			return werr
		}
		flusher.Flush()
		return nil
	})
	x.ev.Estimated = !sawUsage
	x.finishStream(err)
}

// finishStream sets the terminal status for a stream based on the scan error.
func (x *exchange) finishStream(err error) {
	switch {
	case err == nil:
		x.ev.Status = hooks.StatusOK
	case errors.Is(x.r.Context().Err(), context.Canceled):
		x.ev.Status = hooks.StatusClientAbort
	default:
		x.ev.Status = hooks.StatusUpstreamError
	}
}

// providerKind reports the egress kind for a resolved route.
func providerKind(p config.Provider) string { return p.Kind }

// readAll reads a size-limited response body (package default limit).
// Prefer exchange.readAllResp when the configured max_body_bytes should apply.
func readAll(resp *http.Response) ([]byte, error) {
	return io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
}

// readAllResp reads a size-limited response body using the exchange body limit.
func (x *exchange) readAllResp(resp *http.Response) ([]byte, error) {
	return io.ReadAll(io.LimitReader(resp.Body, x.bodyLimit()))
}

// readAllLimited reads a size-limited request body (package default limit).
// Does not emit dialect errors on oversize — callers that need that use readBody.
func readAllLimited(r *http.Request) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
}

// readAllLimitedN reads a size-limited request body with an explicit cap.
// Oversize is truncated (not rejected); use exchange.readBody for 413.
func readAllLimitedN(r *http.Request, limit int64) ([]byte, error) {
	if limit <= 0 {
		limit = maxBodyBytes
	}
	return io.ReadAll(io.LimitReader(r.Body, limit))
}

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

func timeSinceMS(start time.Time) int64 { return time.Since(start).Milliseconds() }
