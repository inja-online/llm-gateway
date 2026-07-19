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

func TestCompletionsV1Passthrough(t *testing.T) {
	var gotPath, gotModel, gotAuth, gotPrompt, gotSuffix string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		gotModel, _ = body["model"].(string)
		gotPrompt, _ = body["prompt"].(string)
		gotSuffix, _ = body["suffix"].(string)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"cmpl-fim","object":"text_completion","choices":[{"text":"  if a<=1: return a\n"}],"usage":{"prompt_tokens":20,"completion_tokens":8}}`)
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  deepseek: { kind: openai_compat, base_url: %q }
defaults:
  openai_dialect: deepseek
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/completions",
		strings.NewReader(`{"model":"deepseek/deepseek-chat","prompt":"def fib(a):","suffix":"    return fib(a-1)+fib(a-2)","max_tokens":64}`))
	req.Header.Set("Authorization", "Bearer sk-ds")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d body %s", resp.StatusCode, out)
	}
	if gotPath != "/completions" {
		t.Errorf("upstream path %q", gotPath)
	}
	if gotModel != "deepseek-chat" {
		t.Errorf("model rewrite: %q", gotModel)
	}
	if gotAuth != "Bearer sk-ds" {
		t.Errorf("auth %q", gotAuth)
	}
	if gotPrompt != "def fib(a):" || !strings.Contains(gotSuffix, "return fib") {
		t.Errorf("prompt/suffix not forwarded: %q %q", gotPrompt, gotSuffix)
	}
	if !strings.Contains(string(out), "cmpl-fim") {
		t.Fatalf("body not forwarded: %s", out)
	}

	ev := col.one(t)
	if ev.TokensIn != 20 || ev.TokensOut != 8 || ev.Estimated || ev.Status != hooks.StatusOK {
		t.Errorf("bad event: %+v", ev)
	}
	if ev.Model != "deepseek/deepseek-chat" || ev.UpstreamModel != "deepseek-chat" || ev.Provider != "deepseek" {
		t.Errorf("routing fields: %+v", ev)
	}
}

func TestCompletionsBetaRewritesBaseToBeta(t *testing.T) {
	// Simulate DeepSeek: chat base is host root (or /v1); FIM must hit /beta/completions.
	var gotURL string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.Path
		fmt.Fprint(w, `{"id":"cmpl-beta","choices":[{"text":"x"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	t.Cleanup(upstream.Close)

	// Provider base is the httptest root (like https://api.deepseek.com).
	// /beta/completions should call {root}/beta/completions.
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  deepseek: { kind: openai_compat, base_url: %q }
defaults:
  openai_dialect: deepseek
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	// Patch base to look like host without path; httptest URL is http://127.0.0.1:port
	// rewriteCompletionsBetaBase will append /beta → we need a mux that serves both.
	// Use a custom base pointing at upstream with no path suffix.
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	// Re-parse with base that includes a trailing-less URL; rewrite adds /beta.
	// httptest only has one listener — path will be /beta/completions on same host.
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/beta/completions",
		strings.NewReader(`{"model":"deepseek/deepseek-chat","prompt":"a","suffix":"b"}`))
	req.Header.Set("Authorization", "Bearer k")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d %s", resp.StatusCode, body)
	}
	if gotURL != "/beta/completions" {
		t.Errorf("want /beta/completions, got %q", gotURL)
	}
	col.one(t)
}

func TestCompletionsBetaStripsV1Suffix(t *testing.T) {
	var gotPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/beta/completions", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"choices":[{"text":"ok"}],"usage":{"prompt_tokens":2,"completion_tokens":3}}`)
	})
	mux.HandleFunc("/completions", func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not hit /v1-style /completions when client used /beta")
		http.NotFound(w, r)
	})
	upstream := httptest.NewServer(mux)
	t.Cleanup(upstream.Close)

	// Chat-style base ending in /v1 — FIM should strip /v1 and use /beta.
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  deepseek: { kind: openai_compat, base_url: %q }
defaults:
  openai_dialect: deepseek
`, upstream.URL+"/v1")))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/beta/completions",
		strings.NewReader(`{"model":"deepseek/deepseek-coder","prompt":"fn","max_tokens":8}`))
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
	if gotPath != "/beta/completions" {
		t.Errorf("path %q", gotPath)
	}
	ev := col.one(t)
	if ev.TokensIn != 2 || ev.TokensOut != 3 {
		t.Errorf("usage: %+v", ev)
	}
}

func TestCompletionsBetaBaseAlreadyBeta(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"choices":[{"text":"y"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  ds_fim: { kind: openai_compat, base_url: %q }
defaults:
  openai_dialect: ds_fim
`, upstream.URL+"/beta")))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/beta/completions",
		strings.NewReader(`{"model":"ds_fim/deepseek-chat","prompt":"p"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	// base already …/beta + path /completions → URL path /beta/completions
	if gotPath != "/beta/completions" {
		t.Errorf("path %q want /beta/completions", gotPath)
	}
	col.one(t)
}

func TestCompletionsStreamPassthrough(t *testing.T) {
	chunks := []string{
		`data: {"id":"c1","choices":[{"text":"hel"}]}` + "\n\n",
		`data: {"id":"c1","choices":[{"text":"lo"}],"usage":{"prompt_tokens":4,"completion_tokens":2}}` + "\n\n",
		"data: [DONE]\n\n",
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if so, ok := body["stream_options"].(map[string]any); !ok || so["include_usage"] != true {
			t.Error("include_usage not injected")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		for _, c := range chunks {
			io.WriteString(w, c)
			fl.Flush()
		}
	}))
	gw, col := newTestGateway(t, upstream)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/completions",
		strings.NewReader(`{"model":"m","prompt":"hi","stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if string(got) != strings.Join(chunks, "") {
		t.Fatalf("stream altered: %q", got)
	}
	ev := col.one(t)
	if ev.TokensIn != 4 || ev.TokensOut != 2 || !ev.Stream {
		t.Errorf("%+v", ev)
	}
}

func TestCompletionsRejectsAnthropic(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  anthropic: { kind: anthropic, base_url: "https://api.anthropic.com" }
  openai: { kind: openai, base_url: "https://api.openai.com/v1" }
defaults:
  openai_dialect: openai
`))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	resp, err := http.Post(gw.URL+"/v1/completions", "application/json",
		strings.NewReader(`{"model":"anthropic/claude-3-5-sonnet-latest","prompt":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
}

func TestCompletionsMissingModel(t *testing.T) {
	gw, col := newTestGateway(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream should not be called")
	})))
	resp, err := http.Post(gw.URL+"/v1/completions", "application/json",
		strings.NewReader(`{"prompt":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
}

func TestCompletionsUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"bad fim","type":"invalid_request_error"}}`)
	}))
	gw, col := newTestGateway(t, upstream)
	resp, err := http.Post(gw.URL+"/v1/completions", "application/json",
		strings.NewReader(`{"model":"m","prompt":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 400 || !strings.Contains(string(body), "bad fim") {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	ev := col.one(t)
	if ev.Status != hooks.StatusUpstreamError {
		t.Errorf("%+v", ev)
	}
}

func TestRewriteCompletionsBetaBase(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"https://api.deepseek.com", "https://api.deepseek.com/beta"},
		{"https://api.deepseek.com/", "https://api.deepseek.com/beta"},
		{"https://api.deepseek.com/v1", "https://api.deepseek.com/beta"},
		{"https://api.deepseek.com/v1/", "https://api.deepseek.com/beta"},
		{"https://api.deepseek.com/beta", "https://api.deepseek.com/beta"},
		{"https://api.deepseek.com/beta/", "https://api.deepseek.com/beta"},
	}
	for _, tc := range cases {
		r := Route{Provider: config.Provider{BaseURL: tc.in, Kind: config.KindOpenAICompat}}
		got := rewriteCompletionsBetaBase(r).Provider.BaseURL
		if got != tc.want {
			t.Errorf("in %q: got %q want %q", tc.in, got, tc.want)
		}
	}
}
