package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

func TestObserveDroppedFieldsHeaderOnTranslate(t *testing.T) {
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
observe_dropped_fields: true
providers:
  anthropic: { kind: anthropic, base_url: "` + up.URL + `" }
defaults:
  openai_dialect: anthropic
`))
	if err != nil {
		t.Fatal(err)
	}
	var last hooks.UsageEvent
	h := hooks.Func(func(_ context.Context, ev hooks.UsageEvent) { last = ev })
	gw := httptest.NewServer(NewServer(cfg, h).Handler())
	t.Cleanup(gw.Close)

	body := `{"model":"anthropic/claude","service_tier":"auto","logprobs":true,"messages":[{"role":"user","content":"ping"}]}`
	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d %s", resp.StatusCode, raw)
	}
	hdr := resp.Header.Get("X-Gateway-Dropped-Fields")
	if !strings.Contains(hdr, "openai.service_tier") || !strings.Contains(hdr, "openai.logprobs") {
		t.Fatalf("header=%q", hdr)
	}
	if len(last.DroppedFields) < 2 {
		t.Fatalf("usage dropped_fields=%v", last.DroppedFields)
	}
}

func TestObserveDroppedFieldsOffByDefault(t *testing.T) {
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

	body := `{"model":"anthropic/claude","service_tier":"auto","messages":[{"role":"user","content":"ping"}]}`
	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
	if resp.Header.Get("X-Gateway-Dropped-Fields") != "" {
		t.Fatalf("header must be absent by default: %q", resp.Header.Get("X-Gateway-Dropped-Fields"))
	}
}

func TestOpenAITranslateDropsUnit(t *testing.T) {
	d := openaiTranslateDrops([]byte(`{"logprobs":true,"service_tier":"auto","n":1,"messages":[]}`))
	joined := strings.Join(d, ",")
	if !strings.Contains(joined, "openai.logprobs") || !strings.Contains(joined, "openai.service_tier") {
		t.Fatalf("%v", d)
	}
	if strings.Contains(joined, "openai.n") {
		t.Fatalf("n=1 should not be listed: %v", d)
	}
}
