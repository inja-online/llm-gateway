package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
	"github.com/inja-online/llm-gateway/config"
)

func TestApplyAutoBreakpointsOffByDefault(t *testing.T) {
	cfg := &config.Config{}
	sys := strings.Repeat("a", 3000)
	req := &canonical.Request{
		System: []canonical.Block{{Type: canonical.BlockText, Text: sys}},
		Tools: []canonical.Tool{{
			Name: "t", Description: strings.Repeat("d", 3000), Schema: json.RawMessage(`{}`),
		}},
	}
	if got := applyAutoBreakpoints(cfg, req); len(got) != 0 {
		t.Fatalf("want none, got %v", got)
	}
	if req.System[0].CacheControl != nil || req.Tools[0].CacheControl != nil {
		t.Fatal("must not mutate when disabled")
	}
}

func TestApplyAutoBreakpointsSystemAndTools(t *testing.T) {
	cfg := &config.Config{}
	cfg.Caching.AutoBreakpoints.Enabled = true
	cfg.Caching.AutoBreakpoints.MinChars = 100
	sys := strings.Repeat("s", 120)
	req := &canonical.Request{
		System: []canonical.Block{
			{Type: canonical.BlockText, Text: "short"},
			{Type: canonical.BlockText, Text: sys},
		},
		Tools: []canonical.Tool{
			{Name: "a", Description: "x", Schema: json.RawMessage(`{}`)},
			{Name: "b", Description: strings.Repeat("y", 100), Schema: json.RawMessage(`{"type":"object"}`)},
		},
	}
	got := applyAutoBreakpoints(cfg, req)
	if len(got) != 2 || got[0] != "system" || got[1] != "tools" {
		t.Fatalf("applied=%v", got)
	}
	if req.System[0].CacheControl != nil {
		t.Fatal("breakpoint must be on last system text block only")
	}
	if req.System[1].CacheControl == nil || req.System[1].CacheControl.Type != "ephemeral" {
		t.Fatalf("system cc=%v", req.System[1].CacheControl)
	}
	if req.Tools[0].CacheControl != nil {
		t.Fatal("breakpoint must be on last tool only")
	}
	if req.Tools[1].CacheControl == nil || req.Tools[1].CacheControl.Type != "ephemeral" {
		t.Fatalf("tool cc=%v", req.Tools[1].CacheControl)
	}
}

func TestApplyAutoBreakpointsClientWins(t *testing.T) {
	cfg := &config.Config{}
	cfg.Caching.AutoBreakpoints.Enabled = true
	cfg.Caching.AutoBreakpoints.MinChars = 10
	req := &canonical.Request{
		System: []canonical.Block{{
			Type: canonical.BlockText, Text: strings.Repeat("s", 50),
			CacheControl: &canonical.CacheControl{Type: "ephemeral", TTL: "1h"},
		}},
		Tools: []canonical.Tool{{
			Name: "t", Description: strings.Repeat("d", 50),
			CacheControl: &canonical.CacheControl{Type: "ephemeral"},
		}},
	}
	if got := applyAutoBreakpoints(cfg, req); len(got) != 0 {
		t.Fatalf("client wins: want none applied, got %v", got)
	}
	if req.System[0].CacheControl.TTL != "1h" {
		t.Fatalf("must not overwrite client TTL: %+v", req.System[0].CacheControl)
	}
}

func TestApplyAutoBreakpointsMinChars(t *testing.T) {
	cfg := &config.Config{}
	cfg.Caching.AutoBreakpoints.Enabled = true
	cfg.Caching.AutoBreakpoints.MinChars = 500
	req := &canonical.Request{
		System: []canonical.Block{{Type: canonical.BlockText, Text: "too short"}},
		Tools:  []canonical.Tool{{Name: "t", Description: "d", Schema: json.RawMessage(`{}`)}},
	}
	if got := applyAutoBreakpoints(cfg, req); len(got) != 0 {
		t.Fatalf("below min_chars: got %v", got)
	}
}

func TestApplyAutoBreakpointsTargetsSystemOnly(t *testing.T) {
	cfg := &config.Config{}
	cfg.Caching.AutoBreakpoints.Enabled = true
	cfg.Caching.AutoBreakpoints.MinChars = 10
	cfg.Caching.AutoBreakpoints.Targets = []string{"system"}
	req := &canonical.Request{
		System: []canonical.Block{{Type: canonical.BlockText, Text: strings.Repeat("s", 20)}},
		Tools:  []canonical.Tool{{Name: "t", Description: strings.Repeat("d", 20), Schema: json.RawMessage(`{}`)}},
	}
	got := applyAutoBreakpoints(cfg, req)
	if len(got) != 1 || got[0] != "system" {
		t.Fatalf("got %v", got)
	}
	if req.Tools[0].CacheControl != nil {
		t.Fatal("tools not in targets")
	}
}

func TestOpenAIToAnthropicAutoBreakpointsHeader(t *testing.T) {
	var gotBody map[string]any
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_1","type":"message","role":"assistant","model":"claude",
			"content":[{"type":"text","text":"ok"}],
			"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":1}
		}`))
	}))
	t.Cleanup(up.Close)

	sys := strings.Repeat("S", 100)
	cfg, err := config.Parse([]byte(`
caching:
  auto_breakpoints:
    enabled: true
    min_chars: 50
    targets: [system]
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

	body := `{"model":"anthropic/claude","messages":[
		{"role":"system","content":"` + sys + `"},
		{"role":"user","content":"hi"}
	]}`
	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d %s", resp.StatusCode, out)
	}
	if hdr := resp.Header.Get("X-Gateway-Cache-Auto"); hdr != "system" {
		t.Fatalf("header=%q", hdr)
	}
	// Anthropic system array should carry cache_control on the text block.
	raw, _ := json.Marshal(gotBody)
	if !strings.Contains(string(raw), `"cache_control"`) {
		t.Fatalf("upstream body missing cache_control: %s", raw)
	}
}

func TestOpenAIToAnthropicAutoBreakpointsOffNoHeader(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_1","type":"message","role":"assistant","model":"claude",
			"content":[{"type":"text","text":"ok"}],
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

	body := `{"model":"anthropic/claude","messages":[
		{"role":"system","content":"` + strings.Repeat("S", 3000) + `"},
		{"role":"user","content":"hi"}
	]}`
	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if h := resp.Header.Get("X-Gateway-Cache-Auto"); h != "" {
		t.Fatalf("default off must not set header: %q", h)
	}
}
