package proxy

import (
	"context"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/inja-online/llm-gateway/hooks"
)

// gatewayMetrics holds Prometheus instruments registered on a private registry
// so library consumers of NewServer do not clash with the default global registry.
type gatewayMetrics struct {
	reg            *prometheus.Registry
	requestsTotal  *prometheus.CounterVec
	tokensInTotal  prometheus.Counter
	tokensOutTotal prometheus.Counter
	latencySeconds *prometheus.HistogramVec
	handler        http.Handler
}

func newGatewayMetrics() *gatewayMetrics {
	reg := prometheus.NewRegistry()
	// Process/Go collectors are useful for ops scrapes of the binary.
	reg.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	reg.MustRegister(prometheus.NewGoCollector())

	factory := promauto.With(reg)
	m := &gatewayMetrics{
		reg: reg,
		requestsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "llm_gateway_requests_total",
			Help: "Total usage events (proxied requests) by status.",
		}, []string{"status"}),
		tokensInTotal: factory.NewCounter(prometheus.CounterOpts{
			Name: "llm_gateway_tokens_in_total",
			Help: "Sum of tokens_in from usage events.",
		}),
		tokensOutTotal: factory.NewCounter(prometheus.CounterOpts{
			Name: "llm_gateway_tokens_out_total",
			Help: "Sum of tokens_out from usage events.",
		}),
		latencySeconds: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "llm_gateway_request_duration_seconds",
			Help:    "Request latency from usage events (seconds).",
			Buckets: prometheus.DefBuckets,
		}, []string{"status"}),
	}
	// Pre-create low-cardinality status series so scrapes show zeros before traffic.
	for _, st := range []string{
		hooks.StatusOK, hooks.StatusUpstreamError, hooks.StatusClientAbort, hooks.StatusBadRequest,
	} {
		m.requestsTotal.WithLabelValues(st)
		m.latencySeconds.WithLabelValues(st)
	}
	m.handler = promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
	return m
}

func (m *gatewayMetrics) observe(ev hooks.UsageEvent) {
	if m == nil {
		return
	}
	status := ev.Status
	if status == "" {
		status = "unknown"
	}
	m.requestsTotal.WithLabelValues(status).Inc()
	if ev.TokensIn > 0 {
		m.tokensInTotal.Add(float64(ev.TokensIn))
	}
	if ev.TokensOut > 0 {
		m.tokensOutTotal.Add(float64(ev.TokensOut))
	}
	// Always observe latency (0 if unknown) so the histogram series stay live.
	m.latencySeconds.WithLabelValues(status).Observe(float64(ev.LatencyMS) / 1000.0)
}

// metricsHook records usage into Prometheus then forwards to the next hook.
type metricsHook struct {
	m    *gatewayMetrics
	next hooks.Hook
}

func (h metricsHook) OnUsage(ctx context.Context, ev hooks.UsageEvent) {
	if h.m != nil {
		h.m.observe(ev)
	}
	if h.next != nil {
		h.next.OnUsage(ctx, ev)
	}
}

// handleMetrics serves GET /metrics via promhttp (Prometheus / OpenMetrics).
// Always on; exempt from edge auth like /healthz.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if s.metrics == nil || s.metrics.handler == nil {
		http.Error(w, "metrics unavailable", http.StatusServiceUnavailable)
		return
	}
	s.metrics.handler.ServeHTTP(w, r)
}
