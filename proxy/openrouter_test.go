package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

// OpenRouter sends extra headers (HTTP-Referer, X-Title) and body fields
// (provider routing, plugins). Passthrough must not strip them.
func TestOpenRouterPassthroughHeadersAndExtraFields(t *testing.T) {
	var (
		gotReferer string
		gotTitle   string
		gotBody    map[string]any
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReferer = r.Header.Get("HTTP-Referer")
		if gotReferer == "" {
			gotReferer = r.Header.Get("Referer")
		}
		gotTitle = r.Header.Get("X-Title")
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"or-1","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":2,"completion_tokens":3}}`)
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  openrouter:
    kind: openai_compat
    base_url: %q
    # media models: set capabilities.image_gen: true when needed
defaults:
  openai_dialect: openrouter
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	body := `{
		"model": "openrouter/anthropic/claude-3.5-sonnet",
		"messages": [{"role":"user","content":"hi"}],
		"provider": {"order": ["Anthropic", "OpenAI"], "allow_fallbacks": true},
		"plugins": [{"id": "web", "max_results": 3}],
		"route": "fallback"
	}`
	req, _ := http.NewRequest("POST", gw.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-or-test")
	req.Header.Set("HTTP-Referer", "https://example.com/app")
	req.Header.Set("X-Title", "Inja Gateway Test")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, raw)
	}

	if gotReferer != "https://example.com/app" {
		t.Errorf("HTTP-Referer not forwarded: %q", gotReferer)
	}
	if gotTitle != "Inja Gateway Test" {
		t.Errorf("X-Title not forwarded: %q", gotTitle)
	}
	// model rewritten to upstream id (strip provider prefix)
	if gotBody["model"] != "anthropic/claude-3.5-sonnet" {
		t.Errorf("model = %v", gotBody["model"])
	}
	// OpenRouter-specific extras survive
	if _, ok := gotBody["provider"].(map[string]any); !ok {
		t.Errorf("provider field dropped: %+v", gotBody)
	}
	if _, ok := gotBody["plugins"].([]any); !ok {
		t.Errorf("plugins field dropped: %+v", gotBody)
	}
	if gotBody["route"] != "fallback" {
		t.Errorf("route = %v", gotBody["route"])
	}

	ev := col.one(t)
	if ev.TokensIn != 2 || ev.TokensOut != 3 || ev.Status != hooks.StatusOK {
		t.Fatalf("%+v", ev)
	}
}

func TestOpenAIOrgProjectHeadersForwarded(t *testing.T) {
	var org, project string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		org = r.Header.Get("OpenAI-Organization")
		project = r.Header.Get("OpenAI-Project")
		fmt.Fprint(w, `{"id":"c","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	gw, col := newTestGateway(t, upstream)

	req, _ := http.NewRequest("POST", gw.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk")
	req.Header.Set("OpenAI-Organization", "org-123")
	req.Header.Set("OpenAI-Project", "proj-456")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	if org != "org-123" || project != "proj-456" {
		t.Fatalf("org=%q project=%q", org, project)
	}
	col.one(t)
}
