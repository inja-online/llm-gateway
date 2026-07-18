package proxy

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

func TestOpenAIOrgProjectNotSentToAnthropic(t *testing.T) {
	var gotOrg string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOrg = r.Header.Get("OpenAI-Organization")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("anthropic-ratelimit-requests-remaining", "50")
		fmt.Fprint(w, `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"model":"claude","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  anthropic: { kind: anthropic, base_url: %q }
defaults:
  anthropic_dialect: anthropic
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/messages",
		strings.NewReader(`{"model":"claude","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("x-api-key", "sk-ant")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OpenAI-Organization", "should-not-forward")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)
	if gotOrg != "" {
		t.Fatalf("org leaked to anthropic: %q", gotOrg)
	}
	if resp.Header.Get("Anthropic-Ratelimit-Requests-Remaining") != "50" {
		t.Fatalf("anthropic rate limit not passed: %v", resp.Header)
	}
	col.one(t)
}

func TestAllowResponseHeader(t *testing.T) {
	cases := map[string]bool{
		"X-Ratelimit-Remaining-Requests":     true,
		"anthropic-ratelimit-tokens-remaining": true,
		"Retry-After":                        true,
		"x-request-id":                       true,
		"Content-Type":                       true,
		"Set-Cookie":                         false,
		"Authorization":                      false,
		"X-Custom-Secret":                    false,
	}
	for h, want := range cases {
		if got := allowResponseHeader(h); got != want {
			t.Errorf("%s: got %v want %v", h, got, want)
		}
	}
}

func TestSessionLimiter(t *testing.T) {
	l := newSessionLimiter(2, 1)
	if err := l.tryAcquire("a"); err != nil {
		t.Fatal(err)
	}
	if err := l.tryAcquire("b"); err != nil {
		t.Fatal(err)
	}
	if err := l.tryAcquire("c"); err == nil {
		t.Fatal("want max_sessions error")
	}
	if l.count() != 2 {
		t.Fatal(l.count())
	}
	l.release("a")
	if err := l.tryAcquire("c"); err != nil {
		t.Fatal(err)
	}
	if l.maxDuration() != 60000000000 { // 1 minute
		t.Fatalf("dur %v", l.maxDuration())
	}
}

func TestRealtimeRejectsWithoutUpgrade(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "http://127.0.0.1:9" }
defaults:
  openai_dialect: openai
`))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	resp, err := http.Get(gw.URL + "/v1/realtime?model=openai/gpt-4o-realtime-preview")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
}

func TestRealtimeCapabilityReject(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  compat: { kind: openai_compat, base_url: "http://127.0.0.1:9" }
defaults:
  openai_dialect: compat
`))
	if err != nil {
		t.Fatal(err)
	}
	// openai_compat defaults realtime=false
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodGet, gw.URL+"/v1/realtime?model=compat/rt", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d %s", resp.StatusCode, body)
	}
	col.one(t)
}

func TestRealtimeMaxSessions(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "http://127.0.0.1:9" }
defaults:
  openai_dialect: openai
realtime:
  max_sessions: 1
  max_session_minutes: 5
`))
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(cfg, &collector{})
	if err := s.sessions.tryAcquire("hold"); err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	s.hook = col
	gw := httptest.NewServer(s.Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodGet, gw.URL+"/v1/realtime?model=openai/rt", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d %s", resp.StatusCode, body)
	}
	col.one(t)
	s.sessions.release("hold")
}

func TestRealtimeWSPassthrough(t *testing.T) {
	// Upstream accepts WS upgrade, writes a marker, then closes.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/realtime" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.URL.Query().Get("model") != "gpt-4o-realtime-preview" {
			t.Errorf("model %s", r.URL.Query().Get("model"))
		}
		if r.Header.Get("Authorization") != "Bearer sk-rt" {
			t.Errorf("auth %s", r.Header.Get("Authorization"))
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("no hijack")
		}
		conn, bufrw, err := hj.Hijack()
		if err != nil {
			t.Fatal(err)
		}
		key := r.Header.Get("Sec-WebSocket-Key")
		fmt.Fprintf(bufrw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", wsAccept(key))
		bufrw.Flush()
		_, _ = conn.Write([]byte("upstream-hello"))
		conn.Close()
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  openai_dialect: openai
realtime:
  max_sessions: 8
  max_session_minutes: 1
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	conn, err := dialWS(gw.URL+"/v1/realtime?model=openai/gpt-4o-realtime-preview", map[string]string{
		"Authorization": "Bearer sk-rt",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil && n == 0 {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(buf[:n]), "upstream-hello") {
		t.Fatalf("got %q", buf[:n])
	}
	conn.Close()
	ev := waitOne(t, col)
	if ev.Modality != "realtime" || ev.Transport != hooks.TransportWebSocket {
		t.Fatalf("%+v", ev)
	}
	if ev.Media == nil || ev.Media.UnitKind != hooks.MediaUnitSessionMinute {
		t.Fatalf("media %+v", ev.Media)
	}
}

// bufferedConn reads first from a bufio.Reader (leftover after HTTP parse), then the net.Conn.
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.r.Read(p) }

func dialWS(rawURL string, headers map[string]string) (net.Conn, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	conn, err := net.Dial("tcp", u.Host)
	if err != nil {
		return nil, err
	}
	path := u.RequestURI()
	req := "GET " + path + " HTTP/1.1\r\nHost: " + u.Host + "\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n"
	for k, v := range headers {
		req += k + ": " + v + "\r\n"
	}
	req += "\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		conn.Close()
		return nil, err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodGet})
	if err != nil {
		conn.Close()
		return nil, err
	}
	if resp.StatusCode != 101 {
		b, _ := io.ReadAll(resp.Body)
		conn.Close()
		return nil, fmt.Errorf("status %d %s", resp.StatusCode, b)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return &bufferedConn{Conn: conn, r: br}, nil
}

func waitOne(t *testing.T, c *collector) hooks.UsageEvent {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		n := len(c.events)
		c.mu.Unlock()
		if n >= 1 {
			return c.one(t)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no usage event")
	return hooks.UsageEvent{}
}

func TestGoogleLiveRejectsWithoutUpgrade(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  google: { kind: google, base_url: "http://127.0.0.1:9" }
defaults:
  google_dialect: google
`))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	resp, err := http.Get(gw.URL + "/v1beta/models/gemini-2.0-flash-live:bidiGenerateContent")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
}

func TestGoogleLiveNonBidi404(t *testing.T) {
	cfg, _ := config.Parse([]byte(`providers: { google: { kind: google, base_url: "http://x" } }`))
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)
	resp, err := http.Get(gw.URL + "/v1beta/models/gemini:generateContent")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestGoogleLiveRejectsOpenAIProvider(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "http://127.0.0.1:9" }
defaults:
  google_dialect: openai
`))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	req, _ := http.NewRequest(http.MethodGet, gw.URL+"/v1beta/models/gpt:bidiGenerateContent", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d %s", resp.StatusCode, body)
	}
	col.one(t)
}

func TestGoogleLiveMaxSessions(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  google: { kind: google, base_url: "http://127.0.0.1:9" }
defaults:
  google_dialect: google
realtime:
  max_sessions: 1
`))
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(cfg, &collector{})
	_ = s.sessions.tryAcquire("hold")
	col := &collector{}
	s.hook = col
	gw := httptest.NewServer(s.Handler())
	t.Cleanup(gw.Close)
	req, _ := http.NewRequest(http.MethodGet, gw.URL+"/v1beta/models/gemini-live:bidiGenerateContent", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d %s", resp.StatusCode, body)
	}
	col.one(t)
	s.sessions.release("hold")
}

func TestRealtimeMissingModel(t *testing.T) {
	cfg, _ := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "http://x" }
defaults: { openai_dialect: openai }
`))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	req, _ := http.NewRequest(http.MethodGet, gw.URL+"/v1/realtime", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
}

func TestGenWSKey(t *testing.T) {
	k := genWSKey()
	if k == "" {
		t.Fatal("empty")
	}
	if _, err := base64.StdEncoding.DecodeString(k); err != nil {
		t.Fatal(err)
	}
}

func TestGoogleLiveWSPassthrough(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "bidiGenerateContent") {
			t.Errorf("path %s", r.URL.Path)
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("no hijack")
		}
		conn, bufrw, err := hj.Hijack()
		if err != nil {
			t.Fatal(err)
		}
		key := r.Header.Get("Sec-WebSocket-Key")
		fmt.Fprintf(bufrw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", wsAccept(key))
		bufrw.Flush()
		_, _ = conn.Write([]byte("live-hello"))
		conn.Close()
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

	conn, err := dialWS(gw.URL+"/v1beta/models/gemini-live:bidiGenerateContent", map[string]string{
		"x-goog-api-key": "gk",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil && n == 0 {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(buf[:n]), "live-hello") {
		t.Fatalf("got %q", buf[:n])
	}
	conn.Close()
	ev := waitOne(t, col)
	if ev.Modality != "realtime" || ev.DialectIn != "google" {
		t.Fatalf("%+v", ev)
	}
}

func TestRealtimeRejectsAnthropicKind(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  anthropic: { kind: anthropic, base_url: "http://x" }
  openai: { kind: openai, base_url: "http://y" }
defaults:
  openai_dialect: openai
`))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	req, _ := http.NewRequest(http.MethodGet, gw.URL+"/v1/realtime?model=anthropic/claude", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
}

func TestRealtimeTLSNotImplemented(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
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
	req, _ := http.NewRequest(http.MethodGet, gw.URL+"/v1/realtime?model=openai/gpt-rt", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d %s", resp.StatusCode, body)
	}
	col.one(t)
}

func TestRealtimeUpstreamNon101(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		fmt.Fprint(w, `{"error":{"message":"bad model"}}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  openai_dialect: openai
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	req, _ := http.NewRequest(http.MethodGet, gw.URL+"/v1/realtime?model=openai/rt", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 400 {
		t.Fatalf("status %d %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "bad model") {
		t.Fatalf("%s", body)
	}
	col.one(t)
}

func TestWsUpstreamURLStripsProvider(t *testing.T) {
	u := wsUpstreamURL("http://example/v1", "/realtime", map[string][]string{
		"model":    {"m"},
		"provider": {"openai"},
	})
	if strings.Contains(u, "provider=") {
		t.Fatalf("%s", u)
	}
	if !strings.Contains(u, "model=m") {
		t.Fatalf("%s", u)
	}
}
