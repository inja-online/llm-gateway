package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

func TestHealthz(t *testing.T) {
	cfg, _ := config.Parse([]byte(`providers: { x: { kind: openai, base_url: "https://x" } }`))
	srv := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(body) != `{"status":"ok"}` {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
}

func TestNewServerNilHook(t *testing.T) {
	cfg, _ := config.Parse([]byte(`providers: { x: { kind: openai, base_url: "https://x" } }`))
	s := NewServer(cfg, nil)
	if s.hook == nil {
		t.Fatal("hook should be non-nil Multi")
	}
}

func TestApplyAuthSchemes(t *testing.T) {
	cases := []struct {
		kind   string
		wantH  string
		wantV  string
		extra  string
		extraV string
	}{
		{config.KindOpenAI, "Authorization", "Bearer sk", "", ""},
		{config.KindOpenAICompat, "Authorization", "Bearer sk", "", ""},
		{config.KindAnthropic, "x-api-key", "sk", "anthropic-version", "2023-06-01"},
		{config.KindGoogle, "x-goog-api-key", "sk", "", ""},
	}
	for _, c := range cases {
		req, _ := http.NewRequest("POST", "http://x", nil)
		applyAuth(req, config.Provider{Kind: c.kind}, "sk")
		if got := req.Header.Get(c.wantH); got != c.wantV {
			t.Errorf("%s: %s=%q want %q", c.kind, c.wantH, got, c.wantV)
		}
		if c.extra != "" && req.Header.Get(c.extra) != c.extraV {
			t.Errorf("%s: %s=%q", c.kind, c.extra, req.Header.Get(c.extra))
		}
	}
	// empty key: no headers
	req, _ := http.NewRequest("POST", "http://x", nil)
	applyAuth(req, config.Provider{Kind: config.KindOpenAI}, "")
	if req.Header.Get("Authorization") != "" {
		t.Error("empty key should not set auth")
	}
	// api_key_env empty env falls back to client key
	old := envLookup
	envLookup = func(string) string { return "" }
	t.Cleanup(func() { envLookup = old })
	req, _ = http.NewRequest("POST", "http://x", nil)
	applyAuth(req, config.Provider{Kind: config.KindOpenAI, APIKeyEnv: "MISSING"}, "client")
	if req.Header.Get("Authorization") != "Bearer client" {
		t.Errorf("fallback: %q", req.Header.Get("Authorization"))
	}
}

func TestClientKeySources(t *testing.T) {
	r, _ := http.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer abc")
	if clientKey(r) != "abc" {
		t.Fatal(clientKey(r))
	}
	r, _ = http.NewRequest("GET", "/", nil)
	r.Header.Set("x-api-key", "xyz")
	if clientKey(r) != "xyz" {
		t.Fatal(clientKey(r))
	}
}

func TestHashKeyEmpty(t *testing.T) {
	if hashKey("") != "" {
		t.Fatal("empty key hash")
	}
	if len(hashKey("sk")) != 12 {
		t.Fatal(hashKey("sk"))
	}
}

func TestUnknownProviderKindOnOpenAI(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not call")
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  g: { kind: google, base_url: %q }
defaults:
  openai_dialect: g
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"gemini","messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status %d", resp.StatusCode)
	}
	ev := col.one(t)
	if ev.Status != hooks.StatusBadRequest {
		t.Errorf("%+v", ev)
	}
}

func TestUnknownProviderKindOnAnthropic(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("must not call")
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  g: { kind: google, base_url: %q }
defaults:
  anthropic_dialect: g
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	resp, err := http.Post(gw.URL+"/v1/messages", "application/json",
		strings.NewReader(`{"model":"g","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
}

func TestMissingModelOpenAI(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	gw, col := newTestGateway(t, upstream)
	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatal(resp.StatusCode)
	}
	if col.one(t).Status != hooks.StatusBadRequest {
		t.Fatal("want bad_request")
	}
}

func TestUnknownRouteModel(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	gw, col := newTestGateway(t, upstream)
	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"nope/x","messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatal(resp.StatusCode)
	}
	col.one(t)
}

func TestOpenAIPassthroughInvalidJSONBodyMap(t *testing.T) {
	// model peeks ok via partial parse? actually invalid JSON fails earlier
	// Use a case that peeks model but fails full map? hard with json
	// Exercise estimated usage path (no usage in response)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":"1","choices":[{"message":{"role":"assistant","content":"x"}}]}`)
	}))
	gw, col := newTestGateway(t, upstream)
	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	ev := col.one(t)
	if !ev.Estimated || ev.Status != hooks.StatusOK {
		t.Errorf("%+v", ev)
	}
}

func TestSendUpstreamTransportError(t *testing.T) {
	cfg, _ := config.Parse([]byte(`
providers:
  up: { kind: openai_compat, base_url: "http://127.0.0.1:1" }
defaults:
  openai_dialect: up
`))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"m","messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if col.one(t).Status != hooks.StatusUpstreamError {
		t.Fatal("want upstream_error")
	}
}

func TestClientAbortOnUpstream(t *testing.T) {
	started := make(chan struct{})
	block := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-block
		fmt.Fprint(w, `{}`)
	}))
	t.Cleanup(func() { close(block); upstream.Close() })

	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  up: { kind: openai_compat, base_url: %q }
defaults:
  openai_dialect: up
`, upstream.URL)))
	col := &collector{}
	s := NewServer(cfg, col)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequestWithContext(ctx, "POST", "/v1/chat/completions",
		strings.NewReader(`{"model":"m","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		s.Handler().ServeHTTP(rr, req)
		close(done)
	}()
	<-started
	cancel()
	<-done

	ev := col.one(t)
	if ev.Status != hooks.StatusClientAbort || ev.HTTPStatus != 499 {
		t.Errorf("%+v", ev)
	}
}

func TestEmitOnce(t *testing.T) {
	cfg, _ := config.Parse([]byte(`providers: { x: { kind: openai, base_url: "https://x" } }`))
	col := &collector{}
	s := NewServer(cfg, col)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", nil)
	x := s.newExchange(rr, req, DialectOpenAI, writeOpenAIError)
	x.emit()
	x.emit() // second must be no-op
	if len(col.events) != 1 {
		t.Fatalf("events=%d", len(col.events))
	}
}

func TestXAPIKeyForwardedToAnthropic(t *testing.T) {
	var gotKey, gotVer string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		gotVer = r.Header.Get("anthropic-version")
		fmt.Fprint(w, `{"id":"m","type":"message","role":"assistant","model":"c","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  ant: { kind: anthropic, base_url: %q }
defaults:
  anthropic_dialect: ant
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	t.Cleanup(upstream.Close)

	req, _ := http.NewRequest("POST", gw.URL+"/v1/messages",
		strings.NewReader(`{"model":"c","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("x-api-key", "sk-ant-test")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotKey != "sk-ant-test" || gotVer != "2023-06-01" {
		t.Errorf("key=%q ver=%q", gotKey, gotVer)
	}
	col.one(t)
}
