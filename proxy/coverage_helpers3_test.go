package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
)

func TestOpenAIToGoogleErrorPaths(t *testing.T) {
	// invalid request body
	up := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("no call")
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  openai_dialect: google
`, up.URL)))
	gw := httptest.NewServer(NewServer(cfg, &collector{}).Handler())
	t.Cleanup(gw.Close)
	// n>1 rejected on translation path (does not call upstream)
	resp, _ := http.Post(gw.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"gemini","n":2,"messages":[{"role":"user","content":"hi"}]}`))
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("%d", resp.StatusCode)
	}

	// upstream 400
	upE := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"error":{"code":400,"message":"bad","status":"INVALID_ARGUMENT"}}`)
	}))
	t.Cleanup(upE.Close)
	cfgE, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  openai_dialect: google
`, upE.URL)))
	gwE := httptest.NewServer(NewServer(cfgE, &collector{}).Handler())
	t.Cleanup(gwE.Close)
	resp2, _ := http.Post(gwE.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"gemini","messages":[{"role":"user","content":"hi"}]}`))
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode < 400 {
		t.Fatalf("%d", resp2.StatusCode)
	}

	// unparseable upstream body
	upB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not-json`)
	}))
	t.Cleanup(upB.Close)
	cfgB, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  openai_dialect: google
`, upB.URL)))
	gwB := httptest.NewServer(NewServer(cfgB, &collector{}).Handler())
	t.Cleanup(gwB.Close)
	resp3, _ := http.Post(gwB.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"gemini","messages":[{"role":"user","content":"hi"}]}`))
	io.Copy(io.Discard, resp3.Body)
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusBadGateway {
		t.Fatalf("%d", resp3.StatusCode)
	}
}

func TestAnthropicToGoogleErrorPaths(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("no")
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  anthropic_dialect: google
`, up.URL)))
	gw := httptest.NewServer(NewServer(cfg, &collector{}).Handler())
	t.Cleanup(gw.Close)
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/messages",
		strings.NewReader(`{"model":"gemini","messages":[]}`))
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("%d", resp.StatusCode)
	}

	// upstream error
	upE := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		fmt.Fprint(w, `{"error":{"code":403,"message":"denied","status":"PERMISSION_DENIED"}}`)
	}))
	t.Cleanup(upE.Close)
	cfgE, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  anthropic_dialect: google
`, upE.URL)))
	gwE := httptest.NewServer(NewServer(cfgE, &collector{}).Handler())
	t.Cleanup(gwE.Close)
	req2, _ := http.NewRequest(http.MethodPost, gwE.URL+"/v1/messages",
		strings.NewReader(`{"model":"gemini","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`))
	req2.Header.Set("anthropic-version", "2023-06-01")
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := http.DefaultClient.Do(req2)
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode < 400 {
		t.Fatalf("%d", resp2.StatusCode)
	}

	// bad upstream body
	upB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `garbage`)
	}))
	t.Cleanup(upB.Close)
	cfgB, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  anthropic_dialect: google
`, upB.URL)))
	gwB := httptest.NewServer(NewServer(cfgB, &collector{}).Handler())
	t.Cleanup(gwB.Close)
	req3, _ := http.NewRequest(http.MethodPost, gwB.URL+"/v1/messages",
		strings.NewReader(`{"model":"gemini","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`))
	req3.Header.Set("anthropic-version", "2023-06-01")
	req3.Header.Set("Content-Type", "application/json")
	resp3, _ := http.DefaultClient.Do(req3)
	io.Copy(io.Discard, resp3.Body)
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusBadGateway {
		t.Fatalf("%d", resp3.StatusCode)
	}
}

func TestPeekMultipartModelLF(t *testing.T) {
	// LF-only separators (not CRLF)
	body := []byte("--b\nContent-Disposition: form-data; name=\"model\"\n\ngpt-x\n--b--\n")
	if got := peekMultipartModel(body); got != "gpt-x" {
		t.Fatalf("%q", got)
	}
	// missing value separator
	if peekMultipartModel([]byte(`name="model" no sep`)) != "" {
		t.Fatal()
	}
	// end of buffer after headers
	if got := peekMultipartModel([]byte(`name="model"` + "\n\n" + `only`)); got != "only" {
		t.Fatalf("%q", got)
	}
}

func TestSessionLimiterReleaseUnknown(t *testing.T) {
	l := newSessionLimiter(2, 5)
	if d := l.release("missing"); d != 0 {
		t.Fatalf("%v", d)
	}
}

func TestVideoTranslateToGoogleParseFail(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not-json`)
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  openai_dialect: google
`, up.URL)))
	gw := httptest.NewServer(NewServer(cfg, &collector{}).Handler())
	t.Cleanup(gw.Close)
	resp, _ := http.Post(gw.URL+"/v1/videos", "application/json",
		strings.NewReader(`{"model":"veo","prompt":"waves"}`))
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Fatalf("%d", resp.StatusCode)
	}

	// anthropic video → google parse fail
	cfgA, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  anthropic_dialect: google
`, up.URL)))
	gwA := httptest.NewServer(NewServer(cfgA, &collector{}).Handler())
	t.Cleanup(gwA.Close)
	req, _ := http.NewRequest(http.MethodPost, gwA.URL+"/v1/videos",
		strings.NewReader(`{"model":"veo","prompt":"x"}`))
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	resp2, _ := http.DefaultClient.Do(req)
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode < 400 {
		t.Fatalf("%d", resp2.StatusCode)
	}
}

func TestOpenAIVideoGetGoogleParseFail(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not-json`)
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  openai_dialect: google
`, up.URL)))
	gw := httptest.NewServer(NewServer(cfg, &collector{}).Handler())
	t.Cleanup(gw.Close)
	resp, _ := http.Get(gw.URL + "/v1/videos/op1?provider=google")
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Fatalf("%d", resp.StatusCode)
	}
}

func TestGoogleVideoCreateToOpenAIParseFail(t *testing.T) {
	// Google dialect → openai family video not common; exercise openai→google get fail
	// already covered. Hit google video poll upstream error instead.
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		fmt.Fprint(w, `{"error":{"message":"missing"}}`)
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  google_dialect: google
`, up.URL)))
	gw := httptest.NewServer(NewServer(cfg, &collector{}).Handler())
	t.Cleanup(gw.Close)
	resp, _ := http.Get(gw.URL + "/v1beta/videos/op-missing?provider=google")
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 404 && resp.StatusCode < 400 {
		t.Fatalf("%d", resp.StatusCode)
	}
}

func TestAnthropicPassthroughUpstreamError(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"x"}}`)
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  anthropic: { kind: anthropic, base_url: %q }
defaults:
  anthropic_dialect: anthropic
`, up.URL)))
	gw := httptest.NewServer(NewServer(cfg, &collector{}).Handler())
	t.Cleanup(gw.Close)
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/messages",
		strings.NewReader(`{"model":"claude","max_tokens":5,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("%d", resp.StatusCode)
	}
}

func TestOpenAIPassthroughUpstreamError(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		fmt.Fprint(w, `{"error":{"message":"auth","type":"invalid_request_error"}}`)
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  openai_dialect: openai
`, up.URL)))
	gw := httptest.NewServer(NewServer(cfg, &collector{}).Handler())
	t.Cleanup(gw.Close)
	resp, _ := http.Post(gw.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("%d", resp.StatusCode)
	}
}

func TestGooglePassthroughUpstreamError(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		fmt.Fprint(w, `{"error":{"code":429,"message":"rate","status":"RESOURCE_EXHAUSTED"}}`)
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  google_dialect: google
`, up.URL)))
	gw := httptest.NewServer(NewServer(cfg, &collector{}).Handler())
	t.Cleanup(gw.Close)
	resp, _ := http.Post(gw.URL+"/v1beta/models/gemini:generateContent",
		"application/json", strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"x"}]}]}`))
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 429 {
		t.Fatalf("%d", resp.StatusCode)
	}
}

func TestIngressMediaHelpersRemaining(t *testing.T) {
	// mediaUnitsFromOpenAIImageBody invalid
	mu := mediaUnitsFromOpenAIImageBody([]byte(`notjson`))
	if mu.Units != 1 {
		t.Fatalf("%+v", mu)
	}
	mu = mediaUnitsFromOpenAIImageBody([]byte(`{"n":3,"size":"512x512"}`))
	if mu.Units != 3 || mu.Size != "512x512" {
		t.Fatalf("%+v", mu)
	}
	if videoCreateMediaUsage(nil).Units != 1 {
		t.Fatal()
	}
}
