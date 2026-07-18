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

func TestAnthropicToGoogle(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, ":generateContent") {
			t.Errorf("path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{"text":"yo"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2}}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  anthropic_dialect: google
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest("POST", gw.URL+"/v1/messages",
		strings.NewReader(`{"model":"gemini-2.0-flash","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("x-api-key", "gk")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	if out["type"] != "message" {
		t.Fatalf("%v", out)
	}
	ev := col.one(t)
	if ev.TokensIn != 5 || ev.TokensOut != 2 {
		t.Fatalf("%+v", ev)
	}
}

func TestGoogleToAnthropic(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Errorf("path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"id":"msg_1","type":"message","role":"assistant","model":"claude",
			"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn",
			"usage":{"input_tokens":3,"output_tokens":1}}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  anthropic: { kind: anthropic, base_url: %q }
defaults:
  google_dialect: anthropic
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest("POST", gw.URL+"/v1beta/models/claude-sonnet:generateContent",
		strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	req.Header.Set("x-api-key", "sk")
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
		t.Fatalf("%s", b)
	}
	col.one(t)
}

func TestOpenAIToGoogleStream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "alt=sse") {
			t.Errorf("query %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"A\"}]}}]}\n\n")
		fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"B\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":2}}\n\n")
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

	req, _ := http.NewRequest("POST", gw.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"gemini-2.0-flash","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer g")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "chat.completion.chunk") {
		t.Fatalf("%s", body)
	}
	ev := col.one(t)
	if ev.Stream != true {
		t.Fatalf("%+v", ev)
	}
}

func TestGoogleToOpenAIStream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
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

	req, _ := http.NewRequest("POST", gw.URL+"/v1beta/models/gpt-4o:streamGenerateContent",
		strings.NewReader(`{"contents":[{"parts":[{"text":"hi"}]}]}`))
	req.Header.Set("Authorization", "Bearer sk")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "data:") {
		t.Fatalf("%s", body)
	}
	col.one(t)
}

func TestGoogleToAnthropicStream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// Anthropic-style named events as data payloads our parser understands
		fmt.Fprint(w, "data: {\"type\":\"message_start\",\"message\":{\"id\":\"m1\",\"model\":\"c\",\"usage\":{\"input_tokens\":2}}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hi\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  anthropic: { kind: anthropic, base_url: %q }
defaults:
  google_dialect: anthropic
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest("POST", gw.URL+"/v1beta/models/claude:streamGenerateContent",
		strings.NewReader(`{"contents":[{"parts":[{"text":"hi"}]}]}`))
	req.Header.Set("x-api-key", "sk")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)
	col.one(t)
}

func TestAnthropicToGoogleStream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Z\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":1}}\n\n")
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  anthropic_dialect: google
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest("POST", gw.URL+"/v1/messages",
		strings.NewReader(`{"model":"g","max_tokens":10,"stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("x-api-key", "gk")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "content_block") && !strings.Contains(string(body), "message_start") {
		t.Fatalf("%s", body)
	}
	col.one(t)
}

func TestGooglePassthroughStreamAndErrors(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "streamGenerateContent") {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"s\"}]}}],\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":1}}\n\n")
			return
		}
		w.WriteHeader(500)
		fmt.Fprint(w, `{"error":{"message":"boom","status":"INTERNAL"}}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  google_dialect: google
  openai_dialect: google
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	// stream passthrough
	req, _ := http.NewRequest("POST", gw.URL+"/v1beta/models/gemini:streamGenerateContent",
		strings.NewReader(`{"contents":[{"parts":[{"text":"hi"}]}]}`))
	req.Header.Set("x-goog-api-key", "k")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()
	if col.one(t).Stream != true {
		t.Fatal("expected stream event")
	}

	// google error → openai dialect envelope
	col2 := &collector{}
	gw2 := httptest.NewServer(NewServer(cfg, col2).Handler())
	t.Cleanup(gw2.Close)
	req2, _ := http.NewRequest("POST", gw2.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"err-model","messages":[{"role":"user","content":"x"}]}`))
	req2.Header.Set("Authorization", "Bearer k")
	resp2, _ := http.DefaultClient.Do(req2)
	io.ReadAll(resp2.Body)
	resp2.Body.Close()
	ev := col2.one(t)
	if ev.Status != hooks.StatusUpstreamError {
		t.Fatalf("%+v", ev)
	}
}

func TestGoogleErrorTranslations(t *testing.T) {
	// OpenAI upstream error when google client talks to openai provider
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		fmt.Fprint(w, `{"error":{"message":"nope","type":"auth_error"}}`)
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

	req, _ := http.NewRequest("POST", gw.URL+"/v1beta/models/gpt:generateContent",
		strings.NewReader(`{"contents":[{"parts":[{"text":"hi"}]}]}`))
	req.Header.Set("Authorization", "Bearer sk")
	resp, _ := http.DefaultClient.Do(req)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 401 || !strings.Contains(string(body), "nope") {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	col.one(t)

	// anthropic upstream error → google envelope
	upstream2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		fmt.Fprint(w, `{"type":"error","error":{"type":"rate_limit_error","message":"slow"}}`)
	}))
	t.Cleanup(upstream2.Close)
	cfg2, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  anthropic: { kind: anthropic, base_url: %q }
defaults:
  google_dialect: anthropic
`, upstream2.URL)))
	col2 := &collector{}
	gw2 := httptest.NewServer(NewServer(cfg2, col2).Handler())
	t.Cleanup(gw2.Close)
	req3, _ := http.NewRequest("POST", gw2.URL+"/v1beta/models/c:generateContent",
		strings.NewReader(`{"contents":[{"parts":[{"text":"hi"}]}]}`))
	req3.Header.Set("x-api-key", "sk")
	resp3, _ := http.DefaultClient.Do(req3)
	b3, _ := io.ReadAll(resp3.Body)
	resp3.Body.Close()
	if !strings.Contains(string(b3), "slow") {
		t.Fatalf("%s", b3)
	}
	col2.one(t)

	// google upstream error on anthropic client
	upstream3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		fmt.Fprint(w, `{"error":{"message":"denied","status":"PERMISSION_DENIED"}}`)
	}))
	t.Cleanup(upstream3.Close)
	cfg3, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  anthropic_dialect: google
`, upstream3.URL)))
	col3 := &collector{}
	gw3 := httptest.NewServer(NewServer(cfg3, col3).Handler())
	t.Cleanup(gw3.Close)
	req4, _ := http.NewRequest("POST", gw3.URL+"/v1/messages",
		strings.NewReader(`{"model":"g","max_tokens":8,"messages":[{"role":"user","content":"x"}]}`))
	req4.Header.Set("x-api-key", "gk")
	resp4, _ := http.DefaultClient.Do(req4)
	b4, _ := io.ReadAll(resp4.Body)
	resp4.Body.Close()
	if !strings.Contains(string(b4), "denied") {
		t.Fatalf("%s", b4)
	}
	col3.one(t)
}

func TestGoogleBadPathAndBadBody(t *testing.T) {
	cfg, _ := config.Parse([]byte(`
providers:
  google: { kind: google, base_url: "http://127.0.0.1:9" }
defaults:
  google_dialect: google
`))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	resp, _ := http.Post(gw.URL+"/v1beta/models/not-a-valid-action", "application/json", strings.NewReader(`{}`))
	if resp.StatusCode != 404 {
		t.Fatalf("%d", resp.StatusCode)
	}
	resp.Body.Close()
	col.one(t)

	// missing contents → 400 on translation path
	cfg2, _ := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "http://127.0.0.1:9" }
defaults:
  google_dialect: openai
`))
	col2 := &collector{}
	gw2 := httptest.NewServer(NewServer(cfg2, col2).Handler())
	t.Cleanup(gw2.Close)
	req, _ := http.NewRequest("POST", gw2.URL+"/v1beta/models/m:generateContent", strings.NewReader(`{}`))
	resp2, _ := http.DefaultClient.Do(req)
	resp2.Body.Close()
	if resp2.StatusCode != 400 {
		t.Fatalf("%d", resp2.StatusCode)
	}
	col2.one(t)
}

func TestExtractGoogleUsage(t *testing.T) {
	ev := &hooks.UsageEvent{}
	if !extractGoogleUsage([]byte(`{"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3}}`), ev) {
		t.Fatal()
	}
	if ev.TokensIn != 2 || ev.TokensOut != 3 {
		t.Fatalf("%+v", ev)
	}
	ev2 := &hooks.UsageEvent{}
	if !extractGoogleUsage([]byte(`{"usage_metadata":{"prompt_token_count":4,"candidates_token_count":5}}`), ev2) {
		t.Fatal()
	}
	if !extractGoogleUsage([]byte(`{"usageMetadata":{"prompt_token_count":6,"candidates_token_count":7}}`), &hooks.UsageEvent{}) {
		t.Fatal()
	}
	if extractGoogleUsage([]byte(`{}`), &hooks.UsageEvent{}) {
		t.Fatal()
	}
	if extractGoogleUsage([]byte(`nope`), &hooks.UsageEvent{}) {
		t.Fatal()
	}
}

func TestWriteAndParseGoogleError(t *testing.T) {
	rr := httptest.NewRecorder()
	writeGoogleError(rr, 400, "INVALID", "bad")
	if rr.Code != 400 {
		t.Fatal(rr.Code)
	}
	msg, code := parseGoogleError([]byte(`{"error":{"message":"m","status":"S"}}`))
	if msg != "m" || code != "S" {
		t.Fatalf("%s %s", msg, code)
	}
	msg, code = parseGoogleError([]byte(`{}`))
	if msg != "upstream error" {
		t.Fatal(msg)
	}
}
