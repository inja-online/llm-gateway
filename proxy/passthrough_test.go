package proxy

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

// collector records usage events and enforces the exactly-once invariant.
type collector struct {
	mu     sync.Mutex
	events []hooks.UsageEvent
}

func (c *collector) OnUsage(_ context.Context, ev hooks.UsageEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, ev)
}

func (c *collector) one(t *testing.T) hooks.UsageEvent {
	t.Helper()
	// Stream handlers may finish after the client has already read the body;
	// wait briefly for the deferred emit under -race.
	deadline := time.Now().Add(2 * time.Second)
	for {
		c.mu.Lock()
		n := len(c.events)
		if n == 1 {
			ev := c.events[0]
			c.mu.Unlock()
			return ev
		}
		if n > 1 {
			evs := append([]hooks.UsageEvent(nil), c.events...)
			c.mu.Unlock()
			t.Fatalf("want exactly 1 usage event, got %d: %+v", n, evs)
		}
		c.mu.Unlock()
		if time.Now().After(deadline) {
			t.Fatalf("want exactly 1 usage event, got 0: []")
		}
		time.Sleep(time.Millisecond)
	}
}

func newTestGateway(t *testing.T, upstream *httptest.Server) (*httptest.Server, *collector) {
	t.Helper()
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  up: { kind: openai_compat, base_url: %q }
defaults:
  openai_dialect: up
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	t.Cleanup(upstream.Close)
	return gw, col
}

func TestNonStreamPassthrough(t *testing.T) {
	var gotAuth, gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		gotModel, _ = body["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"cmpl-1","choices":[{"message":{"role":"assistant","content":"hi"}}],"usage":{"prompt_tokens":12,"completion_tokens":5}}`)
	}))
	gw, col := newTestGateway(t, upstream)

	req, _ := http.NewRequest("POST", gw.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"up/test-model","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer sk-client-key")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"cmpl-1"`) {
		t.Fatalf("body not forwarded: %s", body)
	}
	if gotAuth != "Bearer sk-client-key" {
		t.Errorf("client key not forwarded, upstream saw %q", gotAuth)
	}
	if gotModel != "test-model" {
		t.Errorf("model not rewritten, upstream saw %q", gotModel)
	}

	ev := col.one(t)
	if ev.TokensIn != 12 || ev.TokensOut != 5 || ev.Status != hooks.StatusOK || ev.Estimated {
		t.Errorf("bad event: %+v", ev)
	}
	if ev.Model != "up/test-model" || ev.UpstreamModel != "test-model" || ev.Provider != "up" {
		t.Errorf("bad routing fields: %+v", ev)
	}
}

func TestStreamPassthrough(t *testing.T) {
	chunks := []string{
		`data: {"id":"c1","choices":[{"delta":{"content":"hel"}}]}` + "\n\n",
		`data: {"id":"c1","choices":[{"delta":{"content":"lo"}}]}` + "\n\n",
		`data: {"id":"c1","choices":[],"usage":{"prompt_tokens":7,"completion_tokens":3}}` + "\n\n",
		"data: [DONE]\n\n",
	}
	var sawIncludeUsage bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if so, ok := body["stream_options"].(map[string]any); ok {
			sawIncludeUsage, _ = so["include_usage"].(bool)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		for _, c := range chunks {
			io.WriteString(w, c)
			fl.Flush()
		}
	}))
	gw, col := newTestGateway(t, upstream)

	req, _ := http.NewRequest("POST", gw.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"m","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	got, _ := io.ReadAll(resp.Body)
	want := strings.Join(chunks, "")
	if string(got) != want {
		t.Fatalf("stream bytes altered:\ngot:  %q\nwant: %q", got, want)
	}
	if !sawIncludeUsage {
		t.Error("include_usage not injected into streaming request")
	}

	ev := col.one(t)
	if ev.TokensIn != 7 || ev.TokensOut != 3 || !ev.Stream || ev.Status != hooks.StatusOK {
		t.Errorf("bad event: %+v", ev)
	}
	if ev.TTFTMS < 0 {
		t.Errorf("ttft not recorded: %+v", ev)
	}
}

func TestStreamUpstreamCutMidway(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		io.WriteString(w, `data: {"choices":[{"delta":{"content":"partial"}}]}`+"\n\n")
		fl.Flush()
		// Abort the connection without a final chunk or [DONE].
		hj, _ := w.(http.Hijacker)
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	gw, col := newTestGateway(t, upstream)

	req, _ := http.NewRequest("POST", gw.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"m","stream":true,"messages":[]}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	ev := col.one(t)
	// A killed connection surfaces as a read error mid-stream: upstream_error,
	// no usage seen -> estimated. Exactly one event either way.
	if ev.Status != hooks.StatusUpstreamError || !ev.Estimated || ev.TokensIn != 0 {
		t.Errorf("bad event after upstream cut: %+v", ev)
	}
}

func TestUpstreamHTTPError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(429)
		fmt.Fprint(w, `{"error":{"message":"rate limited","type":"rate_limit_error"}}`)
	}))
	gw, col := newTestGateway(t, upstream)

	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"m","messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 429 {
		t.Fatalf("upstream status not forwarded: %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "rate limited") {
		t.Fatalf("upstream error body not forwarded: %s", body)
	}
	ev := col.one(t)
	if ev.Status != hooks.StatusUpstreamError || ev.HTTPStatus != 429 {
		t.Errorf("bad event: %+v", ev)
	}
}

func TestBadRequestJSON(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream must not be called for invalid JSON")
	}))
	gw, col := newTestGateway(t, upstream)

	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{not json`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	ev := col.one(t)
	if ev.Status != hooks.StatusBadRequest {
		t.Errorf("bad event: %+v", ev)
	}
}

func TestEnvKeyOverride(t *testing.T) {
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		fmt.Fprint(w, `{"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  up: { kind: openai_compat, base_url: %q, api_key_env: TEST_UPSTREAM_KEY }
defaults:
  openai_dialect: up
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	old := envLookup
	envLookup = func(k string) string {
		if k == "TEST_UPSTREAM_KEY" {
			return "sk-server-side"
		}
		return ""
	}
	t.Cleanup(func() { envLookup = old })

	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	t.Cleanup(upstream.Close)

	req, _ := http.NewRequest("POST", gw.URL+"/v1/chat/completions", strings.NewReader(`{"model":"m","messages":[]}`))
	req.Header.Set("Authorization", "Bearer sk-client")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotAuth != "Bearer sk-server-side" {
		t.Errorf("env key override not applied, upstream saw %q", gotAuth)
	}
	col.one(t)
}

// TestStreamBytesReachClientIncrementally guards against buffering the whole
// stream before forwarding (would kill TTFT).
func TestStreamBytesReachClientIncrementally(t *testing.T) {
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n")
		fl.Flush()
		<-release // hold the stream open
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	gw, _ := newTestGateway(t, upstream)
	defer close(release)

	req, _ := http.NewRequest("POST", gw.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"m","stream":true,"messages":[]}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// First chunk must arrive while upstream is still blocked.
	br := bufio.NewReader(resp.Body)
	line, err := br.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, "first") {
		t.Fatalf("first chunk not forwarded before stream end: %q", line)
	}
}
