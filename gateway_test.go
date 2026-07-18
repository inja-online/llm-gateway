package gateway_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	gateway "github.com/inja-online/llm-gateway"
	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

func TestNewWiresJSONLAndExtraHook(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"1","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":2,"completion_tokens":1}}`)
	}))
	t.Cleanup(upstream.Close)

	jsonlPath := filepath.Join(t.TempDir(), "u.jsonl")
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  up: { kind: openai_compat, base_url: %q }
defaults:
  openai_dialect: up
hooks:
  jsonl: { output: %q }
`, upstream.URL, jsonlPath)))
	if err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var extra []hooks.UsageEvent
	h, err := gateway.New(cfg, gateway.WithHook(hooks.Func(func(_ context.Context, ev hooks.UsageEvent) {
		mu.Lock()
		extra = append(extra, ev)
		mu.Unlock()
	})))
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	// healthz
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "ok") {
		t.Fatalf("healthz: %d %s", resp.StatusCode, body)
	}

	// chat
	resp, err = http.Post(srv.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}

	mu.Lock()
	if len(extra) != 1 {
		t.Fatalf("extra hook events = %d", len(extra))
	}
	mu.Unlock()

	raw, err := os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	var ev hooks.UsageEvent
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(raw))), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Status != hooks.StatusOK || ev.TokensIn != 2 {
		t.Fatalf("jsonl event: %+v", ev)
	}
}

func TestNewJSONLBadPath(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  up: { kind: openai, base_url: "https://example.com/v1" }
hooks:
  jsonl: { output: "/no/such/dir/usage.jsonl" }
`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = gateway.New(cfg)
	if err == nil {
		t.Fatal("expected jsonl init error")
	}
	if !strings.Contains(err.Error(), "hooks.jsonl") {
		t.Fatalf("error = %v", err)
	}
}

func TestNewWithoutHooks(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  up: { kind: openai, base_url: "https://example.com/v1" }
`))
	if err != nil {
		t.Fatal(err)
	}
	h, err := gateway.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if h == nil {
		t.Fatal("nil handler")
	}
}
