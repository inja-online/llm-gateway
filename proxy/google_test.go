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
	m, method, ok = parseGoogleAction("gemini-2.0-flash:countTokens")
	if !ok || m != "gemini-2.0-flash" || method != "countTokens" {
		t.Fatalf("%q %q %v", m, method, ok)
	}
	if _, _, ok := parseGoogleAction("nope"); ok {
		t.Fatal("expected fail")
	}
}

func TestGoogleModelsListAndGet(t *testing.T) {
	var gotListPath, gotGetPath, gotAuth string
	var gets int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("x-goog-api-key")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/models":
			gets++
			gotListPath = r.URL.Path
			fmt.Fprint(w, `{"models":[{"name":"models/gemini-2.0-flash"}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/models/gemini-2.0-flash":
			gets++
			gotGetPath = r.URL.Path
			fmt.Fprint(w, `{"name":"models/gemini-2.0-flash","displayName":"Gemini"}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
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

	req, _ := http.NewRequest(http.MethodGet, gw.URL+"/v1beta/models", nil)
	req.Header.Set("x-goog-api-key", "gk")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "gemini-2.0-flash") {
		t.Fatalf("list %d %s", resp.StatusCode, body)
	}
	if gotListPath != "/models" || gotAuth != "gk" {
		t.Fatalf("list path/auth %q %q", gotListPath, gotAuth)
	}

	req2, _ := http.NewRequest(http.MethodGet, gw.URL+"/v1beta/models/gemini-2.0-flash?provider=google", nil)
	req2.Header.Set("x-goog-api-key", "gk")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 200 || !strings.Contains(string(b2), "displayName") {
		t.Fatalf("get %d %s", resp2.StatusCode, b2)
	}
	if gotGetPath != "/models/gemini-2.0-flash" {
		t.Fatalf("get path %q", gotGetPath)
	}
	if gets != 2 {
		t.Fatalf("gets=%d", gets)
	}
	// Discovery must not emit usage events.
	col.mu.Lock()
	n := len(col.events)
	col.mu.Unlock()
	if n != 0 {
		t.Fatalf("usage events on models discovery: %d", n)
	}
}

func TestGoogleModelsListRejectsNonGoogleProvider(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not call upstream")
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
	gw := httptest.NewServer(NewServer(cfg, &collector{}).Handler())
	t.Cleanup(gw.Close)
	resp, err := http.Get(gw.URL + "/v1beta/models")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestGoogleCountTokensPassthrough(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("x-goog-api-key")
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"totalTokens":42}`)
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
		gw.URL+"/v1beta/models/gemini-2.0-flash:countTokens",
		strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	req.Header.Set("x-goog-api-key", "gk")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if gotPath != "/models/gemini-2.0-flash:countTokens" {
		t.Fatalf("path %q", gotPath)
	}
	if gotAuth != "gk" {
		t.Fatalf("auth %q", gotAuth)
	}
	var out struct {
		TotalTokens int `json:"totalTokens"`
	}
	json.Unmarshal(body, &out)
	if out.TotalTokens != 42 {
		t.Fatalf("body %s", body)
	}
	// generateContent still works on the same mux pattern.
	upstream2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, ":generateContent") {
			t.Errorf("path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1}}`)
	}))
	t.Cleanup(upstream2.Close)
	cfg2, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  google_dialect: google
`, upstream2.URL)))
	col2 := &collector{}
	gw2 := httptest.NewServer(NewServer(cfg2, col2).Handler())
	t.Cleanup(gw2.Close)
	reqG, _ := http.NewRequest(http.MethodPost,
		gw2.URL+"/v1beta/models/gemini-2.0-flash:generateContent",
		strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	reqG.Header.Set("x-goog-api-key", "gk")
	reqG.Header.Set("Content-Type", "application/json")
	respG, err := http.DefaultClient.Do(reqG)
	if err != nil {
		t.Fatal(err)
	}
	respG.Body.Close()
	if respG.StatusCode != 200 {
		t.Fatalf("generateContent %d", respG.StatusCode)
	}
	col2.one(t)

	// countTokens must not emit usage.
	col.mu.Lock()
	n := len(col.events)
	col.mu.Unlock()
	if n != 0 {
		t.Fatalf("usage events on countTokens: %d", n)
	}
}
