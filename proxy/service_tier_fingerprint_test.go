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

// #51: service_tier request + system_fingerprint response on OpenAI-family
// passthrough; never invent on cross-dialect translate.

func TestServiceTierPassthroughOpenAI(t *testing.T) {
	var gotBody map[string]any
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-1","object":"chat.completion","model":"gpt-4o-mini",
			"system_fingerprint":"fp_from_upstream",
			"service_tier":"default",
			"choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],
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

	reqBody := `{"model":"openai/gpt-4o-mini","service_tier":"auto","messages":[{"role":"user","content":"ping"}]}`
	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d body %s", resp.StatusCode, out)
	}
	if gotBody["service_tier"] != "auto" {
		t.Fatalf("upstream request service_tier=%v body=%v", gotBody["service_tier"], gotBody)
	}
	if gotBody["model"] != "gpt-4o-mini" {
		t.Fatalf("model rewrite failed: %v", gotBody["model"])
	}
	var client map[string]any
	if err := json.Unmarshal(out, &client); err != nil {
		t.Fatal(err)
	}
	if client["system_fingerprint"] != "fp_from_upstream" {
		t.Fatalf("system_fingerprint not forwarded: %s", out)
	}
	if client["service_tier"] != "default" {
		t.Fatalf("response service_tier not forwarded: %s", out)
	}
}

func TestServiceTierNotInventedOnAnthropicTranslate(t *testing.T) {
	var gotBody map[string]any
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_1","type":"message","role":"assistant","model":"claude-test",
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
  anthropic_dialect: anthropic
`))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	reqBody := `{"model":"anthropic/claude-test","service_tier":"auto","messages":[{"role":"user","content":"ping"}]}`
	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d body %s", resp.StatusCode, out)
	}
	if _, ok := gotBody["service_tier"]; ok {
		t.Fatalf("service_tier must not leak to Anthropic wire: %v", gotBody)
	}
	var client map[string]any
	if err := json.Unmarshal(out, &client); err != nil {
		t.Fatal(err)
	}
	// OpenAI response serializer must not invent fingerprint when Anthropic had none.
	if fp, ok := client["system_fingerprint"]; ok && fp != nil && fp != "" {
		t.Fatalf("must not invent system_fingerprint: %v", fp)
	}
}
