package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
)

func TestAnthropicLiveModelsList(t *testing.T) {
	var gotPath, gotVersion, gotKey string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotVersion = r.Header.Get("anthropic-version")
		gotKey = r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-sonnet-4","type":"model","display_name":"Sonnet"}]}`))
	}))
	t.Cleanup(up.Close)

	cfg, err := config.Parse([]byte(`
providers:
  anthropic: { kind: anthropic, base_url: "` + up.URL + `" }
defaults:
  anthropic_dialect: anthropic
`))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodGet, gw.URL+"/v1/models", nil)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("x-api-key", "sk-ant")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if gotPath != "/models" {
		t.Fatalf("path %q", gotPath)
	}
	if gotVersion != "2023-06-01" || gotKey != "sk-ant" {
		t.Fatalf("headers version=%q key=%q", gotVersion, gotKey)
	}
	if !strings.Contains(string(body), "claude-sonnet-4") {
		t.Fatalf("%s", body)
	}
}

func TestAnthropicLiveModelsGet(t *testing.T) {
	var gotPath string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"claude-sonnet-4","type":"model"}`))
	}))
	t.Cleanup(up.Close)

	cfg, err := config.Parse([]byte(`
providers:
  anthropic: { kind: anthropic, base_url: "` + up.URL + `" }
defaults:
  anthropic_dialect: anthropic
`))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodGet, gw.URL+"/v1/models/anthropic/claude-sonnet-4?live=1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d", resp.StatusCode)
	}
	if gotPath != "/models/claude-sonnet-4" {
		t.Fatalf("path %q", gotPath)
	}
}

func TestModelsCatalogStillDefault(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "http://127.0.0.1:9" }
aliases:
  fast: openai/gpt-4o-mini
defaults:
  openai_dialect: openai
`))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	resp, err := http.Get(gw.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "fast") {
		t.Fatalf("config catalog expected: %s", body)
	}
}

func TestLiveModelsMergeOpenAIAndConfig(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-5.6-sol"},{"id":"gpt-5.6-terra"}]}`))
	}))
	t.Cleanup(up.Close)

	cfg, err := config.Parse([]byte(`
providers:
  openai:
    kind: openai
    base_url: "` + up.URL + `"
    api_key_env: TEST_OPENAI_KEY
aliases:
  gpt: openai/gpt-5.6-terra
defaults:
  openai_dialect: openai
`))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_OPENAI_KEY", "sk-test")
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	resp, err := http.Get(gw.URL + "/v1/models?live=1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, `"gpt"`) {
		t.Fatalf("alias missing: %s", s)
	}
	if !strings.Contains(s, "openai/gpt-5.6-sol") {
		t.Fatalf("live id missing: %s", s)
	}
	if !strings.Contains(s, "openai/gpt-5.6-terra") {
		t.Fatalf("live terra missing: %s", s)
	}
}

func TestParseUpstreamModelIDs(t *testing.T) {
	ids := parseUpstreamModelIDs([]byte(`{"data":[{"id":"a"},{"id":"b"}]}`))
	if len(ids) != 2 || ids[0] != "a" {
		t.Fatalf("%v", ids)
	}
}
