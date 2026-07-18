package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCountTokensProxiesToAnthropicUpstream(t *testing.T) {
	var gotPath, gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		gotModel, _ = body["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"input_tokens":2095}`)
	}))
	gw, _ := newAnthropicGateway(t, upstream)

	resp, err := http.Post(gw.URL+"/v1/messages/count_tokens", "application/json",
		strings.NewReader(`{"model":"up/claude-sonnet-5","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if gotPath != "/messages/count_tokens" {
		t.Errorf("upstream path = %q", gotPath)
	}
	if gotModel != "claude-sonnet-5" {
		t.Errorf("model prefix not stripped: %q", gotModel)
	}
	// The exact upstream count must be passed through, not estimated.
	var out struct {
		InputTokens int `json:"input_tokens"`
	}
	json.Unmarshal(body, &out)
	if out.InputTokens != 2095 {
		t.Errorf("upstream count not forwarded: %s", body)
	}
}

func TestCountTokensEstimatesForNonAnthropicProvider(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("no OpenAI-compatible provider exposes count_tokens; must not call upstream")
	}))
	gw, _ := newAnthropicToOpenAIGateway(t, upstream)

	resp, err := http.Post(gw.URL+"/v1/messages/count_tokens", "application/json",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hello there friend"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out struct {
		InputTokens int `json:"input_tokens"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	if out.InputTokens < 1 {
		t.Errorf("estimate must be at least 1, got %d", out.InputTokens)
	}
}

func TestCountTokensFallsBackWhenUpstreamFails(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	gw, _ := newAnthropicGateway(t, upstream)

	resp, err := http.Post(gw.URL+"/v1/messages/count_tokens", "application/json",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	// A failed upstream must degrade to an estimate rather than erroring.
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out struct {
		InputTokens int `json:"input_tokens"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	if out.InputTokens < 1 {
		t.Errorf("no fallback estimate: %+v", out)
	}
}

func TestCountTokensRejectsMissingModel(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	gw, _ := newAnthropicGateway(t, upstream)

	resp, err := http.Post(gw.URL+"/v1/messages/count_tokens", "application/json", strings.NewReader(`{"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestEstimateTokensScalesWithContent(t *testing.T) {
	short := estimateTokens([]byte(`{"model":"m","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`))
	long := estimateTokens([]byte(`{"model":"m","max_tokens":1,"messages":[{"role":"user","content":"` +
		strings.Repeat("word ", 200) + `"}]}`))
	if long <= short {
		t.Errorf("estimate must grow with content: short=%d long=%d", short, long)
	}
}

func TestCountTokensProxiesToGoogleUpstream(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("x-goog-api-key")
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"totalTokens":2095}`)
	}))
	gw, col := newGatewayWithKind(t, upstream, "google")

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/messages/count_tokens",
		strings.NewReader(`{"model":"up/gemini-2.0-flash","messages":[{"role":"user","content":"hi there"}]}`))
	req.Header.Set("x-api-key", "gk")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if gotPath != "/models/gemini-2.0-flash:countTokens" {
		t.Errorf("upstream path = %q", gotPath)
	}
	if gotAuth != "gk" {
		t.Errorf("auth = %q", gotAuth)
	}
	// Translated body is Google-shaped (contents), not Anthropic messages.
	if _, ok := gotBody["contents"]; !ok {
		t.Errorf("expected google contents, got %v", gotBody)
	}
	if _, ok := gotBody["model"]; ok {
		t.Error("model must not be in google countTokens body")
	}
	var out struct {
		InputTokens int `json:"input_tokens"`
	}
	json.Unmarshal(body, &out)
	if out.InputTokens != 2095 {
		t.Errorf("mapped count not returned: %s", body)
	}
	// No usage event.
	col.mu.Lock()
	n := len(col.events)
	col.mu.Unlock()
	if n != 0 {
		t.Fatalf("usage events: %d", n)
	}
}

func TestCountTokensGoogleFallsBackWhenUpstreamFails(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	gw, _ := newGatewayWithKind(t, upstream, "google")

	resp, err := http.Post(gw.URL+"/v1/messages/count_tokens", "application/json",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hello friend"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out struct {
		InputTokens int `json:"input_tokens"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	if out.InputTokens < 1 {
		t.Errorf("no fallback estimate: %+v", out)
	}
}

func TestParseGoogleTotalTokens(t *testing.T) {
	n, ok := parseGoogleTotalTokens([]byte(`{"totalTokens":7}`))
	if !ok || n != 7 {
		t.Fatalf("%d %v", n, ok)
	}
	n, ok = parseGoogleTotalTokens([]byte(`{"total_tokens":3}`))
	if !ok || n != 3 {
		t.Fatalf("%d %v", n, ok)
	}
	if _, ok := parseGoogleTotalTokens([]byte(`{}`)); ok {
		t.Fatal("empty object should not parse")
	}
}
