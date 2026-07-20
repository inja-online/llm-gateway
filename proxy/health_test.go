package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/inja-online/llm-gateway/config"
)

func TestProviderHealthDisabledByDefault(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "http://127.0.0.1:9" }
`))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	resp, err := http.Get(gw.URL + "/v1/health/providers")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestProviderHealthProbes(t *testing.T) {
	upOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(upOK.Close)
	upDown := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(upDown.Close)

	cfg, err := config.Parse([]byte(`
health_checks:
  enabled: true
  timeout: 1s
providers:
  good: { kind: openai, base_url: "` + upOK.URL + `" }
  bad: { kind: openai_compat, base_url: "` + upDown.URL + `" }
`))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	resp, err := http.Get(gw.URL + "/v1/health/providers")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503 degraded, got %d %s", resp.StatusCode, raw)
	}
	var body struct {
		Status    string `json:"status"`
		Providers []struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "degraded" {
		t.Fatalf("%s", raw)
	}
	byName := map[string]bool{}
	for _, p := range body.Providers {
		byName[p.Name] = p.OK
	}
	if !byName["good"] || byName["bad"] {
		t.Fatalf("providers=%v", byName)
	}
}

func TestProviderHealthAllOK(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized) // reachable
	}))
	t.Cleanup(up.Close)

	cfg, err := config.Parse([]byte(`
health_checks:
  enabled: true
providers:
  openai: { kind: openai, base_url: "` + up.URL + `" }
`))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	resp, err := http.Get(gw.URL + "/v1/health/providers")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
}
