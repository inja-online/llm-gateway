package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/inja-online/llm-gateway/config"
)

// handleProviderHealth serves GET /v1/health/providers when health_checks.enabled.
// Probes each configured provider with a short timeout HEAD/GET against base_url.
// Never logs API keys. Distinct from GET /healthz (process liveness).
//
// When disabled: 404 with OpenAI-shaped error (feature off).
func (s *Server) handleProviderHealth(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil || !s.cfg.HealthChecks.Enabled {
		writeOpenAIError(w, http.StatusNotFound, "invalid_request_error",
			"provider health checks are disabled (set health_checks.enabled: true)")
		return
	}

	timeout := s.cfg.HealthChecks.HealthTimeout()
	type result struct {
		Name   string `json:"name"`
		Kind   string `json:"kind"`
		OK     bool   `json:"ok"`
		Status int    `json:"status,omitempty"`
		Error  string `json:"error,omitempty"`
		MS     int64  `json:"latency_ms"`
	}
	names := make([]string, 0, len(s.cfg.Providers))
	for n := range s.cfg.Providers {
		names = append(names, n)
	}
	sort.Strings(names)

	out := make([]result, 0, len(names))
	for _, name := range names {
		p := s.cfg.Providers[name]
		start := time.Now()
		st, errMsg := s.probeProvider(r.Context(), name, p, timeout)
		res := result{
			Name: name,
			Kind: p.Kind,
			OK:   errMsg == "" && st > 0 && st < 500,
			MS:   time.Since(start).Milliseconds(),
		}
		if st > 0 {
			res.Status = st
		}
		if errMsg != "" {
			res.Error = errMsg
			res.OK = false
		}
		// 401/403 still mean the host is reachable — count as ok for "up".
		if st == http.StatusUnauthorized || st == http.StatusForbidden {
			res.OK = true
			res.Error = ""
		}
		out = append(out, res)
	}

	allOK := true
	for _, r := range out {
		if !r.OK {
			allOK = false
			break
		}
	}
	body := map[string]any{
		"object":    "gateway.provider_health",
		"status":    map[bool]string{true: "ok", false: "degraded"}[allOK],
		"providers": out,
	}
	code := http.StatusOK
	if !allOK {
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, body)
}

func (s *Server) probeProvider(parent context.Context, name string, p config.Provider, timeout time.Duration) (status int, errMsg string) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	// Prefer a cheap discovery path when known; otherwise hit base root.
	path := ""
	switch p.Kind {
	case config.KindAnthropic:
		path = "/models"
	case config.KindGoogle:
		path = "/models"
	default:
		path = "/models"
	}
	url := p.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, "build request failed"
	}
	// Prefer api_key_env for probes (no client request context). Never log keys.
	key := ""
	if p.APIKeyEnv != "" {
		key = envLookup(p.APIKeyEnv)
	}
	applyAuth(req, p, key)

	client := s.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return 0, "timeout"
		}
		return 0, "unreachable"
	}
	defer resp.Body.Close()
	// Drain lightly without logging body.
	_, _ = json.Marshal(nil)
	return resp.StatusCode, ""
}
