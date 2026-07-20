package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
)

// #49: non-function tools — error on translate; passthrough forwards wire tools.

func TestNonFunctionToolsRejectedOnTranslate(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream must not be called when tools fail validation")
		w.WriteHeader(500)
	}))
	t.Cleanup(up.Close)

	cfg, err := config.Parse([]byte(`
providers:
  anthropic: { kind: anthropic, base_url: "` + up.URL + `" }
defaults:
  openai_dialect: anthropic
`))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	body := `{
		"model":"anthropic/claude",
		"messages":[{"role":"user","content":"hi"}],
		"tools":[{"type":"custom","custom":{"name":"x","format":{}}}]
	}`
	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d body %s", resp.StatusCode, raw)
	}
	if !strings.Contains(string(raw), "unsupported tool type") && !strings.Contains(string(raw), "function") {
		t.Fatalf("want tool type error, got %s", raw)
	}
}

func TestNonFunctionToolsPassthroughForwarded(t *testing.T) {
	var gotTools any
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotTools = req["tools"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"c1","object":"chat.completion","model":"gpt",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
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

	body := `{
		"model":"openai/gpt",
		"messages":[{"role":"user","content":"hi"}],
		"tools":[{"type":"custom","custom":{"name":"regex_tool"}}]
	}`
	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d body %s", resp.StatusCode, raw)
	}
	arr, ok := gotTools.([]any)
	if !ok || len(arr) != 1 {
		t.Fatalf("tools not forwarded: %v", gotTools)
	}
	m, _ := arr[0].(map[string]any)
	if m["type"] != "custom" {
		t.Fatalf("expected custom tool passthrough, got %v", m)
	}
}

func TestFunctionToolsStillTranslate(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_1","type":"message","role":"assistant","model":"claude",
			"content":[{"type":"text","text":"hi"}],
			"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}
		}`))
	}))
	t.Cleanup(up.Close)

	cfg, err := config.Parse([]byte(`
providers:
  anthropic: { kind: anthropic, base_url: "` + up.URL + `" }
defaults:
  openai_dialect: anthropic
`))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	body := `{
		"model":"anthropic/claude",
		"messages":[{"role":"user","content":"hi"}],
		"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]
	}`
	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("function tools should still translate: %d %s", resp.StatusCode, raw)
	}
}
