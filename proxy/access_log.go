package proxy

import (
	"bufio"
	"errors"
	"log"
	"net"
	"net/http"
	"time"
)

var errNoHijack = errors.New("http: ResponseWriter does not implement Hijacker")

// withAccessLog wraps h with a concise HTTP access line per request.
// Skips /healthz and /metrics (high-frequency probes). Never logs headers or bodies.
func withAccessLog(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/healthz" || path == "/metrics" {
			h.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		h.ServeHTTP(rec, r)
		// Prefer raw path; include query only when short (model list filters, etc.).
		target := path
		if q := r.URL.RawQuery; q != "" && len(q) < 120 {
			target = path + "?" + q
		}
		log.Printf("http status=%d method=%s path=%s lat_ms=%d bytes=%d",
			rec.status, r.Method, target, time.Since(start).Milliseconds(), rec.bytes)
	})
}

// statusRecorder captures status code and approximate response size.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// Unwrap exposes the underlying ResponseWriter for interfaces like http.Flusher.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// Flush implements http.Flusher when the underlying writer supports it (SSE).
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack implements http.Hijacker for WebSocket upgrades when available.
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := r.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errNoHijack
}
