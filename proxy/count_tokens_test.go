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
