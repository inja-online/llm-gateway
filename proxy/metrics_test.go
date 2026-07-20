package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
)

func TestMetricsEndpointIncrements(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"c1","object":"chat.completion","model":"m",
			"choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}
		}`))
	}))
	t.Cleanup(up.Close)

	cfg, err := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "` + up.URL + `" }
defaults:
  openai_dialect: openai
`))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	// Before: zeros ok
	resp0, err := http.Get(gw.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	b0, _ := io.ReadAll(resp0.Body)
	resp0.Body.Close()
	if resp0.StatusCode != 200 || !strings.Contains(string(b0), "llm_gateway_requests_total 0") {
		t.Fatalf("initial metrics: %d %s", resp0.StatusCode, b0)
	}

	_, _ = http.Post(gw.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"openai/m","messages":[{"role":"user","content":"x"}]}`))

	resp, err := http.Get(gw.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "llm_gateway_requests_total 1") {
		t.Fatalf("requests: %s", s)
	}
	if !strings.Contains(s, "llm_gateway_requests_ok_total 1") {
		t.Fatalf("ok: %s", s)
	}
	if !strings.Contains(s, "llm_gateway_tokens_in_total 3") || !strings.Contains(s, "llm_gateway_tokens_out_total 2") {
		t.Fatalf("tokens: %s", s)
	}
}

func TestMetricsOpenWithEdgeAuth(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "http://127.0.0.1:9" }
edge_auth:
  enabled: true
  keys: ["secret"]
`))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	resp, err := http.Get(gw.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("metrics should bypass edge auth: %d", resp.StatusCode)
	}
}
