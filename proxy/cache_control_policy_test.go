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

// #41 Option B: cache_control is passthrough-only for Anthropic.

func TestCacheControlPassthroughAnthropic(t *testing.T) {
	var got map[string]any
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_1","type":"message","role":"assistant","model":"claude",
			"content":[{"type":"text","text":"ok"}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":2,"output_tokens":1,"cache_read_input_tokens":1}
		}`))
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

	body := `{
		"model":"anthropic/claude",
		"max_tokens":64,
		"system":[{"type":"text","text":"rules","cache_control":{"type":"ephemeral"}}],
		"messages":[{"role":"user","content":"hi"}],
		"tools":[{"name":"lookup","description":"d","input_schema":{"type":"object"},"cache_control":{"type":"ephemeral"}}]
	}`
	resp, err := http.Post(gw.URL+"/v1/messages", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d %s", resp.StatusCode, out)
	}
	raw, _ := json.Marshal(got)
	if !strings.Contains(string(raw), "cache_control") {
		t.Fatalf("passthrough must preserve cache_control: %s", raw)
	}
	if got["model"] != "claude" {
		t.Fatalf("model rewrite: %v", got["model"])
	}
}
