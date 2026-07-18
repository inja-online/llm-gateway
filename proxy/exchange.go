package proxy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/mamad/llm-gateway/config"
	"github.com/mamad/llm-gateway/hooks"
	"github.com/mamad/llm-gateway/internal/sse"
)

const maxBodyBytes = 32 << 20 // 32 MiB

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
	x.s.hook.OnUsage(context.WithoutCancel(x.r.Context()), x.ev)
}

// fail writes a dialect error, records status, and marks the event.
func (x *exchange) fail(httpStatus int, code, msg, evStatus string) {
	x.ev.Status = evStatus
	x.ev.HTTPStatus = x.writeErr(x.w, httpStatus, code, msg)
}

// readBody reads and size-limits the request body.
func (x *exchange) readBody() ([]byte, bool) {
	body, err := io.ReadAll(io.LimitReader(x.r.Body, maxBodyBytes))
	if err != nil {
		x.fail(http.StatusBadRequest, "invalid_request_error", "failed to read request body", hooks.StatusBadRequest)
		return nil, false
	}
	return body, true
}

// sendUpstream builds and sends the upstream POST, applying auth and the
// forwarded (or env-override) key. On transport failure it records the event
// and writes a dialect error, returning ok=false.
func (x *exchange) sendUpstream(route Route, path string, body []byte) (*http.Response, bool) {
	key := clientKey(x.r)
	x.ev.KeyHash = hashKey(key)

	upReq, err := http.NewRequestWithContext(x.r.Context(), http.MethodPost, route.Provider.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to build upstream request", hooks.StatusUpstreamError)
		return nil, false
	}
	upReq.Header.Set("Content-Type", "application/json")
	applyAuth(upReq, route.Provider, key)

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

// forwardErrorResponse relays a >=400 upstream response verbatim.
func (x *exchange) forwardErrorResponse(resp *http.Response) {
	x.ev.Status = hooks.StatusUpstreamError
	x.ev.HTTPStatus = resp.StatusCode
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		x.w.Header().Set("Content-Type", ct)
	}
	x.w.WriteHeader(resp.StatusCode)
	io.Copy(x.w, io.LimitReader(resp.Body, maxBodyBytes))
}

// usageExtractor pulls token counts out of a data payload during a passthrough
// stream. It returns true once usage is found.
type usageExtractor func(data []byte, ev *hooks.UsageEvent) bool

// passthroughStream relays SSE bytes verbatim while scanning data lines for
// usage. done payloads (e.g. "[DONE]") are handled by extract returning false.
func (x *exchange) passthroughStream(resp *http.Response, extract usageExtractor) {
	flusher, ok := x.w.(http.Flusher)
	if !ok {
		x.fail(http.StatusInternalServerError, "api_error", "streaming unsupported by server", hooks.StatusUpstreamError)
		return
	}
	x.w.Header().Set("Content-Type", "text/event-stream")
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
		if _, werr := x.w.Write(line); werr != nil {
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

// readAll reads a size-limited response body.
func readAll(resp *http.Response) ([]byte, error) {
	return io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
}

// readAllLimited reads a size-limited request body.
func readAllLimited(r *http.Request) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
}

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

func timeSinceMS(start time.Time) int64 { return time.Since(start).Milliseconds() }
