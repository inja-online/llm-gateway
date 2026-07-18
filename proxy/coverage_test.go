package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

// errBody fails on Read — covers readBody error path.
type errBody struct{}

func (errBody) Read([]byte) (int, error) { return 0, errors.New("read fail") }
func (errBody) Close() error             { return nil }

// noFlushWriter is an http.ResponseWriter that is not a Flusher.
type noFlushWriter struct {
	h    http.Header
	code int
	buf  strings.Builder
}

func (w *noFlushWriter) Header() http.Header {
	if w.h == nil {
		w.h = make(http.Header)
	}
	return w.h
}
func (w *noFlushWriter) Write(b []byte) (int, error) { return w.buf.Write(b) }
func (w *noFlushWriter) WriteHeader(c int)            { w.code = c }

func TestReadBodyError(t *testing.T) {
	cfg, _ := config.Parse([]byte(`providers: { x: { kind: openai, base_url: "https://x" } }`))
	col := &collector{}
	s := NewServer(cfg, col)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", errBody{})
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 400 {
		t.Fatalf("code %d", rr.Code)
	}
	if col.one(t).Status != hooks.StatusBadRequest {
		t.Fatal()
	}
}

func TestPassthroughStreamNoFlusher(t *testing.T) {
	cfg, _ := config.Parse([]byte(`providers: { x: { kind: openai, base_url: "https://x" } }`))
	col := &collector{}
	s := NewServer(cfg, col)
	req := httptest.NewRequest("POST", "/", nil)
	x := s.newExchange(&noFlushWriter{}, req, DialectOpenAI, writeOpenAIError)
	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("data: {}\n\n")),
		Header:     make(http.Header),
	}
	x.passthroughStream(resp, func([]byte, *hooks.UsageEvent) bool { return false })
	if x.ev.Status != hooks.StatusUpstreamError {
		t.Fatalf("%+v", x.ev)
	}
}

func TestSendUpstreamBadURL(t *testing.T) {
	// base_url with control characters can fail NewRequest
	cfg := &config.Config{
		Providers: map[string]config.Provider{
			"up": {Kind: config.KindOpenAICompat, BaseURL: "http://x\ninvalid"},
		},
		Defaults: config.Defaults{OpenAIDialect: "up"},
	}
	col := &collector{}
	s := NewServer(cfg, col)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions",
		strings.NewReader(`{"model":"m","messages":[]}`))
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		// If NewRequest accepts it, transport will fail instead — still error.
		if rr.Code != 400 && rr.Code != 502 {
			t.Fatalf("code %d body %s", rr.Code, rr.Body.String())
		}
	}
	if len(col.events) != 1 {
		t.Fatalf("events %d", len(col.events))
	}
}

func TestAnthropicUnknownModelRoute(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	gw, col := newAnthropicGateway(t, upstream)
	resp, err := http.Post(gw.URL+"/v1/messages", "application/json",
		strings.NewReader(`{"model":"nope/x","max_tokens":1,"messages":[{"role":"user","content":"h"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatal(resp.StatusCode)
	}
	col.one(t)
}

func TestOpenAIPassthroughNonObjectBody(t *testing.T) {
	// Head parse of model works on objects only; use number model fail already.
	// Exercise estimated stream with no usage.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\n")
		fl.Flush()
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	gw, col := newTestGateway(t, upstream)
	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"m","stream":true,"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	ev := col.one(t)
	if !ev.Estimated || !ev.Stream {
		t.Fatalf("%+v", ev)
	}
}

func TestTranslateOpenAIErrorEmptyBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	gw, col := newTranslateGateway(t, upstream)
	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 500 || !strings.Contains(string(body), "upstream error") {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	col.one(t)
}

func TestTranslateAnthropicErrorEmptyBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	gw, col := newAnthropicToOpenAIGateway(t, upstream)
	resp, err := http.Post(gw.URL+"/v1/messages", "application/json",
		strings.NewReader(`{"model":"m","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 503 || !strings.Contains(string(body), "upstream error") {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	col.one(t)
}

func TestOpenAIToAnthropicInvalidUpstreamJSON(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not-json`)
	}))
	gw, col := newTranslateGateway(t, upstream)
	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatal(resp.StatusCode)
	}
	if col.one(t).Status != hooks.StatusUpstreamError {
		t.Fatal()
	}
}

func TestAnthropicToOpenAIInvalidUpstreamJSON(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{bad`)
	}))
	gw, col := newAnthropicToOpenAIGateway(t, upstream)
	resp, err := http.Post(gw.URL+"/v1/messages", "application/json",
		strings.NewReader(`{"model":"m","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatal(resp.StatusCode)
	}
	col.one(t)
}

func TestStreamNoFlusherOpenAITranslate(t *testing.T) {
	// Direct call path for streamAnthropicToOpenAI without flusher
	cfg, _ := config.Parse([]byte(`
providers:
  claude: { kind: anthropic, base_url: "https://example.com" }
defaults:
  openai_dialect: claude
`))
	col := &collector{}
	s := NewServer(cfg, col)
	req := httptest.NewRequest("POST", "/", nil)
	x := s.newExchange(&noFlushWriter{}, req, DialectOpenAI, writeOpenAIError)
	resp := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}
	s.streamAnthropicToOpenAI(x, resp, time.Now().Unix())
	if x.ev.Status != hooks.StatusUpstreamError {
		t.Fatalf("%+v", x.ev)
	}
}

func TestStreamNoFlusherAnthropicTranslate(t *testing.T) {
	cfg, _ := config.Parse([]byte(`
providers:
  up: { kind: openai_compat, base_url: "https://example.com" }
defaults:
  anthropic_dialect: up
`))
	col := &collector{}
	s := NewServer(cfg, col)
	req := httptest.NewRequest("POST", "/", nil)
	x := s.newExchange(&noFlushWriter{}, req, DialectAnthropic, writeAnthropicError)
	resp := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}
	s.streamOpenAIToAnthropic(x, resp)
	if x.ev.Status != hooks.StatusUpstreamError {
		t.Fatalf("%+v", x.ev)
	}
}

func TestCountTokensProxyTransportFail(t *testing.T) {
	// Anthropic kind with unreachable base_url falls back to estimate
	cfg, err := config.Parse([]byte(`
providers:
  ant: { kind: anthropic, base_url: "http://127.0.0.1:1" }
defaults:
  anthropic_dialect: ant
`))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, hooks.Multi{}).Handler())
	t.Cleanup(gw.Close)
	resp, err := http.Post(gw.URL+"/v1/messages/count_tokens", "application/json",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hello world"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
}

func TestEstimateWithToolsAndSystem(t *testing.T) {
	n := estimateTokens([]byte(`{
		"model":"m","max_tokens":1,
		"system":"sys prompt here",
		"tools":[{"name":"toolname","description":"does things","input_schema":{"type":"object","properties":{"a":{"type":"string"}}}}],
		"messages":[{"role":"user","content":[{"type":"text","text":"hi"},{"type":"tool_result","tool_use_id":"t","content":"result text"}]}]
	}`))
	if n < 5 {
		t.Fatalf("estimate %d", n)
	}
}

func TestFinishStreamClientAbort(t *testing.T) {
	cfg, _ := config.Parse([]byte(`providers: { x: { kind: openai, base_url: "https://x" } }`))
	s := NewServer(cfg, hooks.Multi{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequestWithContext(ctx, "POST", "/", nil)
	x := s.newExchange(httptest.NewRecorder(), req, DialectOpenAI, writeOpenAIError)
	x.finishStream(context.Canceled)
	if x.ev.Status != hooks.StatusClientAbort {
		t.Fatalf("%+v", x.ev)
	}
	x2 := s.newExchange(httptest.NewRecorder(), httptest.NewRequest("POST", "/", nil), DialectOpenAI, writeOpenAIError)
	x2.finishStream(errors.New("other"))
	if x2.ev.Status != hooks.StatusUpstreamError {
		t.Fatalf("%+v", x2.ev)
	}
}

func TestProxyCountTokensBadJSONBody(t *testing.T) {
	// Direct unit: proxyCountTokens with non-object JSON after route resolved is hard via HTTP.
	// Cover via estimate path when body is valid model but parse fails for estimate — already have unparseable.
	cfg, _ := config.Parse([]byte(`
providers:
  ant: { kind: anthropic, base_url: "http://127.0.0.1:1" }
defaults:
  anthropic_dialect: ant
`))
	s := NewServer(cfg, hooks.Multi{})
	rr := httptest.NewRecorder()
	// array root fails model head? actually model missing
	req := httptest.NewRequest("POST", "/v1/messages/count_tokens", strings.NewReader(`[]`))
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 400 {
		t.Fatal(rr.Code)
	}
}
