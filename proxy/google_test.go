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

func TestOpenAIToGoogle(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "gkey" {
			t.Errorf("auth %q", r.Header.Get("x-goog-api-key"))
		}
		if !strings.HasSuffix(r.URL.Path, "/models/gemini-2.0-flash:generateContent") {
			t.Errorf("path %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)
		if _, ok := req["model"]; ok {
			t.Error("model must not be in google body")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"hi"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":1}}`)
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  openai_dialect: google
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"gemini-2.0-flash","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer gkey")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d %s", resp.StatusCode, b)
	}
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	if out["object"] != "chat.completion" {
		t.Fatalf("%v", out)
	}
	ev := col.one(t)
	if ev.Provider != "google" || ev.TokensIn != 4 || ev.TokensOut != 1 {
		t.Fatalf("%+v", ev)
	}
}

func TestGoogleDialectPassthrough(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "gk" {
			t.Errorf("auth %q", r.Header.Get("x-goog-api-key"))
		}
		if !strings.Contains(r.URL.Path, "gemini-2.0-flash:generateContent") {
			t.Errorf("path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1}}`)
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  google_dialect: google
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost,
		gw.URL+"/v1beta/models/gemini-2.0-flash:generateContent",
		strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	req.Header.Set("x-goog-api-key", "gk")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	ev := col.one(t)
	if ev.DialectIn != DialectGoogle || ev.Status != hooks.StatusOK {
		t.Fatalf("%+v", ev)
	}
	if ev.TokensIn != 2 || ev.TokensOut != 1 {
		t.Fatalf("usage %+v", ev)
	}
}

func TestGoogleDialectToOpenAI(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"c1","model":"gpt","choices":[{"message":{"role":"assistant","content":"hey"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1}}`)
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  google_dialect: openai
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost,
		gw.URL+"/v1beta/models/gpt-4o:generateContent",
		strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	req.Header.Set("Authorization", "Bearer sk")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if !strings.Contains(string(b), "candidates") {
		t.Fatalf("expected google-shaped body: %s", b)
	}
	ev := col.one(t)
	if ev.TokensIn != 3 || ev.TokensOut != 1 {
		t.Fatalf("%+v", ev)
	}
}

func TestParseGoogleAction(t *testing.T) {
	m, method, ok := parseGoogleAction("gemini-2.0-flash:generateContent")
	if !ok || m != "gemini-2.0-flash" || method != "generateContent" {
		t.Fatalf("%q %q %v", m, method, ok)
	}
	m, method, ok = parseGoogleAction("models/x:streamGenerateContent")
	// action is only the last segment
	m, method, ok = parseGoogleAction("gemini-2.0-flash:streamGenerateContent")
	if !ok || method != "streamGenerateContent" {
		t.Fatalf("%q %q %v", m, method, ok)
	}
	if _, _, ok := parseGoogleAction("nope"); ok {
		t.Fatal("expected fail")
	}
}
