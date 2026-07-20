package proxy

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/inja-online/llm-gateway/hooks"
)

// processMetrics holds low-cardinality counters for GET /metrics (#95/#154).
// No Prometheus client library — Prometheus text exposition format only.
type processMetrics struct {
	requestsTotal  atomic.Uint64
	requestsOK     atomic.Uint64
	requestsError  atomic.Uint64
	tokensInTotal  atomic.Uint64
	tokensOutTotal atomic.Uint64
}

func (m *processMetrics) observe(ev hooks.UsageEvent) {
	if m == nil {
		return
	}
	m.requestsTotal.Add(1)
	if ev.Status == hooks.StatusOK {
		m.requestsOK.Add(1)
	} else {
		m.requestsError.Add(1)
	}
	if ev.TokensIn > 0 {
		m.tokensInTotal.Add(uint64(ev.TokensIn))
	}
	if ev.TokensOut > 0 {
		m.tokensOutTotal.Add(uint64(ev.TokensOut))
	}
}

// metricsHook records usage then forwards to the next hook.
type metricsHook struct {
	m    *processMetrics
	next hooks.Hook
}

func (h metricsHook) OnUsage(ctx context.Context, ev hooks.UsageEvent) {
	h.m.observe(ev)
	if h.next != nil {
		h.next.OnUsage(ctx, ev)
	}
}

// handleMetrics serves GET /metrics (Prometheus text format). Always on;
// low cardinality only. Exempt from edge auth like /healthz.
func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	m := s.metrics
	if m == nil {
		m = &processMetrics{}
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = fmt.Fprintf(w, "# HELP llm_gateway_requests_total Total usage events (proxied requests).\n")
	_, _ = fmt.Fprintf(w, "# TYPE llm_gateway_requests_total counter\n")
	_, _ = fmt.Fprintf(w, "llm_gateway_requests_total %d\n", m.requestsTotal.Load())
	_, _ = fmt.Fprintf(w, "# HELP llm_gateway_requests_ok_total Usage events with status ok.\n")
	_, _ = fmt.Fprintf(w, "# TYPE llm_gateway_requests_ok_total counter\n")
	_, _ = fmt.Fprintf(w, "llm_gateway_requests_ok_total %d\n", m.requestsOK.Load())
	_, _ = fmt.Fprintf(w, "# HELP llm_gateway_requests_error_total Usage events not ok.\n")
	_, _ = fmt.Fprintf(w, "# TYPE llm_gateway_requests_error_total counter\n")
	_, _ = fmt.Fprintf(w, "llm_gateway_requests_error_total %d\n", m.requestsError.Load())
	_, _ = fmt.Fprintf(w, "# HELP llm_gateway_tokens_in_total Sum of tokens_in from usage events.\n")
	_, _ = fmt.Fprintf(w, "# TYPE llm_gateway_tokens_in_total counter\n")
	_, _ = fmt.Fprintf(w, "llm_gateway_tokens_in_total %d\n", m.tokensInTotal.Load())
	_, _ = fmt.Fprintf(w, "# HELP llm_gateway_tokens_out_total Sum of tokens_out from usage events.\n")
	_, _ = fmt.Fprintf(w, "# TYPE llm_gateway_tokens_out_total counter\n")
	_, _ = fmt.Fprintf(w, "llm_gateway_tokens_out_total %d\n", m.tokensOutTotal.Load())
}
