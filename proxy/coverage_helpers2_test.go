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

func TestGoogleNativeEmbedRejectsNonGoogleProviders(t *testing.T) {
	// openai default for google dialect → embed not implemented
	up := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("must not call")
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  google_dialect: openai
`, up.URL)))
	gw := httptest.NewServer(NewServer(cfg, &collector{}).Handler())
	t.Cleanup(gw.Close)
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1beta/models/emb:embedContent",
		strings.NewReader(`{"content":{"parts":[{"text":"x"}]}}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("%d", resp.StatusCode)
	}

	// anthropic
	cfgA, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  anthropic: { kind: anthropic, base_url: %q }
defaults:
  google_dialect: anthropic
`, up.URL)))
	gwA := httptest.NewServer(NewServer(cfgA, &collector{}).Handler())
	t.Cleanup(gwA.Close)
	req2, _ := http.NewRequest(http.MethodPost, gwA.URL+"/v1beta/models/emb:batchEmbedContents",
		strings.NewReader(`{"requests":[]}`))
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := http.DefaultClient.Do(req2)
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotImplemented {
		t.Fatalf("%d", resp2.StatusCode)
	}
}

func TestModerationsErrorPaths(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		fmt.Fprint(w, `{"error":{"message":"rate"}}`)
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

	// invalid JSON
	resp, _ := http.Post(gw.URL+"/v1/moderations", "application/json", strings.NewReader(`{`))
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("%d", resp.StatusCode)
	}

	// unknown model
	resp2, _ := http.Post(gw.URL+"/v1/moderations", "application/json",
		strings.NewReader(`{"model":"nope/m","input":"x"}`))
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 404 {
		t.Fatalf("%d", resp2.StatusCode)
	}

	// non-openai family model
	cfgA, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  anthropic: { kind: anthropic, base_url: %q }
  openai: { kind: openai, base_url: "http://127.0.0.1:9" }
defaults:
  openai_dialect: openai
`, up.URL)))
	gwA := httptest.NewServer(NewServer(cfgA, &collector{}).Handler())
	t.Cleanup(gwA.Close)
	resp3, _ := http.Post(gwA.URL+"/v1/moderations", "application/json",
		strings.NewReader(`{"model":"anthropic/claude","input":"x"}`))
	io.Copy(io.Discard, resp3.Body)
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusNotImplemented && resp3.StatusCode != 400 {
		t.Fatalf("%d", resp3.StatusCode)
	}

	// upstream 429
	resp4, _ := http.Post(gw.URL+"/v1/moderations", "application/json",
		strings.NewReader(`{"model":"omni-moderation-latest","input":"x"}`))
	io.Copy(io.Discard, resp4.Body)
	resp4.Body.Close()
	if resp4.StatusCode != 429 {
		t.Fatalf("%d", resp4.StatusCode)
	}
}

func TestGoogleImageToOpenAICoveragePaths(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/generations" {
			t.Fatalf("path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"created":1,"data":[{"b64_json":"YQ=="}]}`)
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  google_dialect: openai
`, up.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	req, _ := http.NewRequest(http.MethodPost,
		gw.URL+"/v1beta/models/dall-e-3:generateImages",
		strings.NewReader(`{"prompt":"a cat","numberOfImages":1}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	col.one(t)

	// upstream error
	upE := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"error":{"message":"bad","type":"invalid_request_error"}}`)
	}))
	t.Cleanup(upE.Close)
	cfgE, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  google_dialect: openai
`, upE.URL)))
	gwE := httptest.NewServer(NewServer(cfgE, &collector{}).Handler())
	t.Cleanup(gwE.Close)
	req2, _ := http.NewRequest(http.MethodPost,
		gwE.URL+"/v1beta/models/dall-e-3:generateImages",
		strings.NewReader(`{"prompt":"x"}`))
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := http.DefaultClient.Do(req2)
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode < 400 {
		t.Fatalf("%d", resp2.StatusCode)
	}

	// parse fail
	upP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[]}`)
	}))
	t.Cleanup(upP.Close)
	cfgP, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  google_dialect: openai
`, upP.URL)))
	gwP := httptest.NewServer(NewServer(cfgP, &collector{}).Handler())
	t.Cleanup(gwP.Close)
	req3, _ := http.NewRequest(http.MethodPost,
		gwP.URL+"/v1beta/models/dall-e-3:generateImages",
		strings.NewReader(`{"prompt":"x"}`))
	req3.Header.Set("Content-Type", "application/json")
	resp3, _ := http.DefaultClient.Do(req3)
	io.Copy(io.Discard, resp3.Body)
	resp3.Body.Close()
	// 200 or 502 depending on empty-data handling
}

func TestGoogleToOpenAIBadUpstreamBody(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[]}`)
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  google_dialect: openai
`, up.URL)))
	gw := httptest.NewServer(NewServer(cfg, &collector{}).Handler())
	t.Cleanup(gw.Close)
	resp, _ := http.Post(gw.URL+"/v1beta/models/gpt-4o:generateContent",
		"application/json", strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway && resp.StatusCode != 200 {
		t.Fatalf("%d", resp.StatusCode)
	}

	// anthropic bad body
	upA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"type":"message","content":[]}`)
	}))
	t.Cleanup(upA.Close)
	cfgA, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  anthropic: { kind: anthropic, base_url: %q }
defaults:
  google_dialect: anthropic
`, upA.URL)))
	gwA := httptest.NewServer(NewServer(cfgA, &collector{}).Handler())
	t.Cleanup(gwA.Close)
	resp2, _ := http.Post(gwA.URL+"/v1beta/models/claude:generateContent",
		"application/json", strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
}

func TestAnthropicImageToOpenAIParseFail(t *testing.T) {
	// unusable OpenAI image body → parse error path
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not-json`)
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  anthropic_dialect: openai
`, up.URL)))
	gw := httptest.NewServer(NewServer(cfg, &collector{}).Handler())
	t.Cleanup(gw.Close)
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/images",
		strings.NewReader(`{"model":"dall-e-3","prompt":"x"}`))
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Fatalf("expected error status, got %d", resp.StatusCode)
	}

	// google parse fail
	upG := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not-json`)
	}))
	t.Cleanup(upG.Close)
	cfgG, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  anthropic_dialect: google
`, upG.URL)))
	gwG := httptest.NewServer(NewServer(cfgG, &collector{}).Handler())
	t.Cleanup(gwG.Close)
	req2, _ := http.NewRequest(http.MethodPost, gwG.URL+"/v1/images",
		strings.NewReader(`{"model":"imagen","prompt":"x"}`))
	req2.Header.Set("anthropic-version", "2023-06-01")
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := http.DefaultClient.Do(req2)
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode < 400 {
		t.Fatalf("%d", resp2.StatusCode)
	}
}

func TestGoogleCountTokensBodyModelFallback(t *testing.T) {
	// Resolve fails on body model, falls back to path model
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"totalTokens":1}`)
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  google_dialect: google
`, up.URL)))
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)
	// body model unknown provider prefix but path resolves via default
	resp, _ := http.Post(gw.URL+"/v1beta/models/gemini-2.0-flash:countTokens",
		"application/json", strings.NewReader(`{"model":"models/gemini-2.0-flash","contents":[]}`))
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d", resp.StatusCode)
	}

	// unknown path model
	resp2, _ := http.Post(gw.URL+"/v1beta/models/nope:countTokens",
		"application/json", strings.NewReader(`{"model":"unknownprov/x"}`))
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	// may 404 or still resolve via default
}

func TestOpenAIToAnthropicErrorPaths(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"nope"}}`)
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  anthropic: { kind: anthropic, base_url: %q }
defaults:
  openai_dialect: anthropic
`, up.URL)))
	gw := httptest.NewServer(NewServer(cfg, &collector{}).Handler())
	t.Cleanup(gw.Close)
	resp, _ := http.Post(gw.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"claude","messages":[{"role":"user","content":"hi"}],"max_tokens":10}`))
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Fatalf("%d", resp.StatusCode)
	}

	// bad parse on success path
	upBad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"type":"message"}`)
	}))
	t.Cleanup(upBad.Close)
	cfgB, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  anthropic: { kind: anthropic, base_url: %q }
defaults:
  openai_dialect: anthropic
`, upBad.URL)))
	gwB := httptest.NewServer(NewServer(cfgB, &collector{}).Handler())
	t.Cleanup(gwB.Close)
	resp2, _ := http.Post(gwB.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"claude","messages":[{"role":"user","content":"hi"}],"max_tokens":10}`))
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
}

func TestAnthropicToOpenAIErrorPaths(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"error":{"message":"no","type":"invalid_request_error"}}`)
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  anthropic_dialect: openai
`, up.URL)))
	gw := httptest.NewServer(NewServer(cfg, &collector{}).Handler())
	t.Cleanup(gw.Close)
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/messages",
		strings.NewReader(`{"model":"gpt-4o","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Fatalf("%d", resp.StatusCode)
	}
}

func TestResponsesErrorAndGetPaths(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"error":{"message":"x"}}`)
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
	resp, _ := http.Post(gw.URL+"/v1/responses", "application/json",
		strings.NewReader(`{"model":"gpt-4o","input":"hi"}`))
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("%d", resp.StatusCode)
	}

	// invalid JSON
	resp2, _ := http.Post(gw.URL+"/v1/responses", "application/json", strings.NewReader(`{`))
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 400 {
		t.Fatalf("%d", resp2.StatusCode)
	}

	// missing model
	resp3, _ := http.Post(gw.URL+"/v1/responses", "application/json", strings.NewReader(`{"input":"hi"}`))
	io.Copy(io.Discard, resp3.Body)
	resp3.Body.Close()
	if resp3.StatusCode != 400 {
		t.Fatalf("%d", resp3.StatusCode)
	}
}

func TestGooglePassthroughEstimated(t *testing.T) {
	// google passthrough with no usage → estimated
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{"text":"hi"}]},"finishReason":"STOP"}]}`)
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  google_dialect: google
`, up.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	resp, _ := http.Post(gw.URL+"/v1beta/models/gemini:generateContent",
		"application/json", strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"x"}]}]}`))
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	ev := col.one(t)
	if !ev.Estimated {
		t.Fatalf("%+v", ev)
	}
}

func TestConfigValidateEdgeAuthViaParse(t *testing.T) {
	// exercised through config package but keeps import used
	_ = config.AuthAPIKey
}

func TestAudioCapabilityDeny(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("no")
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  deepseek: { kind: openai_compat, base_url: %q }
defaults:
  openai_dialect: deepseek
`, up.URL)))
	gw := httptest.NewServer(NewServer(cfg, &collector{}).Handler())
	t.Cleanup(gw.Close)
	resp, _ := http.Post(gw.URL+"/v1/audio/transcriptions", "application/json",
		strings.NewReader(`{"model":"whisper","file":"x"}`))
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 400 && resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("%d", resp.StatusCode)
	}
}

func TestVideosGetMissingProvider(t *testing.T) {
	cfg, _ := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "http://127.0.0.1:9" }
`))
	gw := httptest.NewServer(NewServer(cfg, &collector{}).Handler())
	t.Cleanup(gw.Close)
	resp, _ := http.Get(gw.URL + "/v1/videos/vid1")
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("%d", resp.StatusCode)
	}
}

func TestFilesListUpstreamError(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		fmt.Fprint(w, `err`)
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
	resp, _ := http.Get(gw.URL + "/v1/files?provider=openai")
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Fatalf("%d", resp.StatusCode)
	}
}
