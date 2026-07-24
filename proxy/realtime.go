package proxy

import (
	"bufio"
	"context"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

// tlsClientConfig returns the TLS config for wss/https upstream dials.
// Overridden in tests to trust httptest.NewTLSServer certs.
var tlsClientConfig = func() *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS12}
}

// wsDialTimeout is the TCP+TLS dial budget for upstream WebSocket.
const wsDialTimeout = 15 * time.Second

// wsTCPKeepAlive is enabled on upstream TCP conns so idle LBs do not drop
// long Realtime/Live sessions. Application WebSocket ping/pong frames from
// either peer are passed through raw (no gateway-initiated control frames).
const wsTCPKeepAlive = 30 * time.Second

// sessionLimiter enforces process-local realtime concurrency and duration.
type sessionLimiter struct {
	mu           sync.Mutex
	active       map[string]time.Time // id → start
	maxSessions  int
	maxMinutes   int
	now          func() time.Time // injectable for tests
}

func newSessionLimiter(maxSessions, maxMinutes int) *sessionLimiter {
	if maxSessions <= 0 {
		maxSessions = 1024
	}
	if maxMinutes <= 0 {
		maxMinutes = 60
	}
	return &sessionLimiter{
		active:      make(map[string]time.Time),
		maxSessions: maxSessions,
		maxMinutes:  maxMinutes,
		now:         time.Now,
	}
}

func (l *sessionLimiter) tryAcquire(id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.active) >= l.maxSessions {
		return fmt.Errorf("realtime max_sessions (%d) reached", l.maxSessions)
	}
	l.active[id] = l.now()
	return nil
}

func (l *sessionLimiter) release(id string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	start, ok := l.active[id]
	if !ok {
		return 0
	}
	delete(l.active, id)
	return l.now().Sub(start)
}

func (l *sessionLimiter) maxDuration() time.Duration {
	return time.Duration(l.maxMinutes) * time.Minute
}

func (l *sessionLimiter) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.active)
}

// handleRealtime serves GET /v1/realtime WebSocket upgrades (OpenAI Realtime).
// Capability realtime required; OpenAI-family providers only.
//
// Skeleton: HTTP Upgrade handshake + bidirectional raw-frame passthrough after
// dialing the upstream WS. Cross-protocol bridge to Google Live is not
// implemented — attempts fail closed with CodeUnsupportedRealtimeBridge.
func (s *Server) handleRealtime(w http.ResponseWriter, r *http.Request) {
	x := s.newExchange(w, r, DialectOpenAI, writeOpenAIError)
	defer x.emit()
	x.ev.Modality = config.ModalityRealtime
	x.ev.Transport = hooks.TransportWebSocket
	x.ev.Stream = true
	x.ev.Estimated = true

	if !isWebSocketUpgrade(r) {
		x.fail(http.StatusUpgradeRequired, "invalid_request_error",
			"Realtime requires WebSocket Upgrade (Connection: Upgrade, Upgrade: websocket)",
			hooks.StatusBadRequest)
		return
	}

	model := r.URL.Query().Get("model")
	if model == "" {
		x.fail(http.StatusBadRequest, "invalid_request_error", "missing required query parameter: model", hooks.StatusBadRequest)
		return
	}
	x.ev.Model = model

	route, err := Resolve(s.cfg, DialectOpenAI, model)
	if err != nil {
		x.fail(http.StatusNotFound, "invalid_request_error", err.Error(), hooks.StatusBadRequest)
		return
	}
	// Cross-protocol Realtime → Live bridge is not implemented (fail closed).
	if route.Provider.Kind == config.KindGoogle {
		x.fail(http.StatusNotImplemented, CodeUnsupportedRealtimeBridge,
			"OpenAI Realtime ↔ Google Live bridge is not implemented; "+
				"use GET /v1beta/models/{model}:bidiGenerateContent for Google Live, "+
				"or route model to an openai/openai_compat provider for Realtime passthrough",
			hooks.StatusBadRequest)
		return
	}
	if !ensureOpenAIFamily(x, route, "Realtime") {
		return
	}
	if !route.Provider.Supports(config.ModalityRealtime) {
		x.fail(http.StatusNotImplemented, "invalid_request_error",
			"provider "+route.ProviderName+" does not support realtime (set capabilities.realtime: true)",
			hooks.StatusBadRequest)
		return
	}
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = route.UpstreamModel

	if err := s.sessions.tryAcquire(x.ev.RequestID); err != nil {
		x.fail(http.StatusTooManyRequests, "rate_limit_error", err.Error(), hooks.StatusBadRequest)
		return
	}
	// release + usage duration always on exit
	var sessionDur time.Duration
	defer func() {
		sessionDur = s.sessions.release(x.ev.RequestID)
		if x.ev.Media == nil {
			mins := int(sessionDur.Minutes())
			if mins < 1 && sessionDur > 0 {
				mins = 1
			}
			x.ev.Media = &hooks.MediaUsage{
				Units:      mins,
				UnitKind:   hooks.MediaUnitSessionMinute,
				DurationMS: sessionDur.Milliseconds(),
			}
		}
	}()

	if err := s.proxyWebSocket(x, route, r, wsUpstreamURL(route.Provider.BaseURL, "/realtime", r.URL.Query())); err != nil {
		if x.ev.Status == "" {
			x.ev.Status = hooks.StatusUpstreamError
			x.ev.HTTPStatus = http.StatusBadGateway
		}
		// If we already upgraded, we cannot write an HTTP error body.
		return
	}
	if x.ev.Status == "" {
		x.ev.Status = hooks.StatusOK
		x.ev.HTTPStatus = http.StatusSwitchingProtocols
	}
}

// handleGoogleLive serves Google Live / BidiGenerateContent WebSocket path:
//
//	GET /v1beta/models/{model}:bidiGenerateContent
//
// Auth: x-goog-api-key. Requires capabilities.realtime on a kind:google provider.
func (s *Server) handleGoogleLive(w http.ResponseWriter, r *http.Request) {
	x := s.newExchange(w, r, DialectGoogle, writeGoogleError)
	defer x.emit()
	x.ev.Modality = config.ModalityRealtime
	x.ev.Transport = hooks.TransportWebSocket
	x.ev.Stream = true
	x.ev.Estimated = true

	if !isWebSocketUpgrade(r) {
		x.fail(http.StatusUpgradeRequired, "invalid_request_error",
			"Google Live requires WebSocket Upgrade", hooks.StatusBadRequest)
		return
	}

	action := r.PathValue("action")
	model := strings.TrimSuffix(action, ":bidiGenerateContent")
	if model == "" || model == action {
		x.fail(http.StatusNotFound, "invalid_request_error",
			"want /v1beta/models/{model}:bidiGenerateContent", hooks.StatusBadRequest)
		return
	}
	x.ev.Model = model

	route, err := Resolve(s.cfg, DialectGoogle, model)
	if err != nil {
		x.fail(http.StatusNotFound, "invalid_request_error", err.Error(), hooks.StatusBadRequest)
		return
	}
	if route.Provider.Kind != config.KindGoogle {
		// Cross-protocol Live → Realtime bridge is not implemented (fail closed).
		if isOpenAIFamily(route.Provider) {
			x.fail(http.StatusNotImplemented, CodeUnsupportedRealtimeBridge,
				"OpenAI Realtime ↔ Google Live bridge is not implemented; "+
					"use GET /v1/realtime for OpenAI Realtime passthrough, "+
					"or a kind:google provider for Live",
				hooks.StatusBadRequest)
			return
		}
		x.fail(http.StatusNotImplemented, "invalid_request_error",
			"Google Live requires kind:google provider (got "+route.Provider.Kind+")",
			hooks.StatusBadRequest)
		return
	}
	if !route.Provider.Supports(config.ModalityRealtime) {
		x.fail(http.StatusNotImplemented, "invalid_request_error",
			"provider "+route.ProviderName+" does not support realtime",
			hooks.StatusBadRequest)
		return
	}
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = route.UpstreamModel

	if err := s.sessions.tryAcquire(x.ev.RequestID); err != nil {
		x.fail(http.StatusTooManyRequests, "rate_limit_error", err.Error(), hooks.StatusBadRequest)
		return
	}
	defer func() {
		dur := s.sessions.release(x.ev.RequestID)
		mins := int(dur.Minutes())
		if mins < 1 && dur > 0 {
			mins = 1
		}
		x.ev.Media = &hooks.MediaUsage{
			Units: mins, UnitKind: hooks.MediaUnitSessionMinute, DurationMS: dur.Milliseconds(),
		}
	}()

	// Google Live public WS path (BidiGenerateContent).
	// base_url is typically …/v1beta; Live often uses a /ws/… host path.
	// Skeleton: dial {base}/models/{model}:bidiGenerateContent with Upgrade.
	path := "/models/" + route.UpstreamModel + ":bidiGenerateContent"
	if err := s.proxyWebSocket(x, route, r, wsUpstreamURL(route.Provider.BaseURL, path, r.URL.Query())); err != nil {
		if x.ev.Status == "" {
			x.ev.Status = hooks.StatusUpstreamError
			x.ev.HTTPStatus = http.StatusBadGateway
		}
		return
	}
	if x.ev.Status == "" {
		x.ev.Status = hooks.StatusOK
		x.ev.HTTPStatus = http.StatusSwitchingProtocols
	}
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

func wsUpstreamURL(base, path string, q url.Values) string {
	u, err := url.Parse(base + path)
	if err != nil {
		return base + path
	}
	// Rewrite scheme for dial: we dial via HTTP Upgrade, so keep http/https.
	// Copy query but rewrite model to bare upstream when present.
	vals := url.Values{}
	for k, vs := range q {
		if k == "provider" {
			continue
		}
		vals[k] = vs
	}
	u.RawQuery = vals.Encode()
	return u.String()
}

// proxyWebSocket dials the upstream with HTTP Upgrade (plain or TLS/wss),
// completes the client handshake, then copies frames both ways until either
// side closes or the session max duration elapses.
//
// TLS uses system root CAs (MinVersion TLS1.2). Application WebSocket
// ping/pong from either peer is passed through; TCP keepalive is enabled on
// the upstream socket. Mid-session protocol validation is intentionally light
// (raw frame passthrough).
func (s *Server) proxyWebSocket(x *exchange, route Route, clientReq *http.Request, upstreamURL string) error {
	key := clientKey(clientReq)
	x.ev.KeyHash = hashKey(key)
	if s != nil {
		if k, errMsg := s.resolveUpstreamKey(clientReq, route.ProviderName, route.Provider); errMsg != "" {
			x.fail(http.StatusBadGateway, "api_error", errMsg, hooks.StatusUpstreamError)
			return fmt.Errorf("%s", errMsg)
		} else if k != "" {
			key = k
			x.ev.KeyHash = hashKey(key)
		}
	}

	clientWSKey := clientReq.Header.Get("Sec-WebSocket-Key")
	if clientWSKey == "" {
		x.fail(http.StatusBadRequest, "invalid_request_error", "missing Sec-WebSocket-Key", hooks.StatusBadRequest)
		return fmt.Errorf("missing ws key")
	}

	upURL, err := url.Parse(upstreamURL)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "invalid upstream url", hooks.StatusUpstreamError)
		return err
	}
	if upURL.Query().Get("model") != "" {
		q := upURL.Query()
		q.Set("model", route.UpstreamModel)
		upURL.RawQuery = q.Encode()
	}

	upConn, err := dialUpstreamWebSocket(clientReq.Context(), upURL)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "upstream dial failed: "+err.Error(), hooks.StatusUpstreamError)
		return err
	}
	// closed in closeBoth

	// Build upstream upgrade request (reuse client Sec-WebSocket-Key for simplicity).
	host := upURL.Host
	if host == "" {
		host = upURL.Hostname()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "GET %s HTTP/1.1\r\nHost: %s\r\n", upURL.RequestURI(), host)
	b.WriteString("Upgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\n")
	fmt.Fprintf(&b, "Sec-WebSocket-Key: %s\r\n", clientWSKey)
	if proto := clientReq.Header.Get("Sec-WebSocket-Protocol"); proto != "" {
		fmt.Fprintf(&b, "Sec-WebSocket-Protocol: %s\r\n", proto)
	}
	tmp, _ := http.NewRequest(http.MethodGet, upstreamURL, nil)
	applyAuth(tmp, route.Provider, key)
	forwardOpenAIRequestHeaders(tmp, clientReq, route.Provider)
	applySubscriptionHeaders(tmp, clientReq, route.Provider)
	for k, vs := range tmp.Header {
		for _, v := range vs {
			fmt.Fprintf(&b, "%s: %s\r\n", k, v)
		}
	}
	if beta := clientReq.Header.Get("OpenAI-Beta"); beta != "" {
		fmt.Fprintf(&b, "OpenAI-Beta: %s\r\n", beta)
	}
	b.WriteString("\r\n")
	if _, err := io.WriteString(upConn, b.String()); err != nil {
		upConn.Close()
		x.fail(http.StatusBadGateway, "api_error", "upstream upgrade write failed", hooks.StatusUpstreamError)
		return err
	}

	upReader := bufio.NewReader(upConn)
	upResp, err := http.ReadResponse(upReader, &http.Request{Method: http.MethodGet})
	if err != nil {
		upConn.Close()
		x.fail(http.StatusBadGateway, "api_error", "upstream upgrade read failed: "+err.Error(), hooks.StatusUpstreamError)
		return err
	}
	if upResp.StatusCode != http.StatusSwitchingProtocols {
		body, _ := io.ReadAll(io.LimitReader(upResp.Body, x.bodyLimit()))
		upResp.Body.Close()
		upConn.Close()
		x.ev.Status = hooks.StatusUpstreamError
		x.ev.HTTPStatus = upResp.StatusCode
		x.prepareResponseHeaders(upResp)
		if x.w.Header().Get("Content-Type") == "" {
			x.w.Header().Set("Content-Type", "application/json")
		}
		x.w.WriteHeader(upResp.StatusCode)
		x.w.Write(body)
		return fmt.Errorf("upstream upgrade status %d", upResp.StatusCode)
	}
	// Drain upgrade response body if any (should be empty for 101).
	io.Copy(io.Discard, upResp.Body)
	upResp.Body.Close()

	hj, ok := x.w.(http.Hijacker)
	if !ok {
		upConn.Close()
		x.fail(http.StatusInternalServerError, "api_error", "response does not support hijack", hooks.StatusUpstreamError)
		return fmt.Errorf("no hijacker")
	}
	clientConn, clientRW, err := hj.Hijack()
	if err != nil {
		upConn.Close()
		return err
	}

	// Write 101 on the hijacked client connection ourselves.
	var hb strings.Builder
	hb.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	hb.WriteString("Upgrade: websocket\r\nConnection: Upgrade\r\n")
	fmt.Fprintf(&hb, "Sec-WebSocket-Accept: %s\r\n", wsAccept(clientWSKey))
	if p := upResp.Header.Get("Sec-WebSocket-Protocol"); p != "" {
		fmt.Fprintf(&hb, "Sec-WebSocket-Protocol: %s\r\n", p)
	}
	fmt.Fprintf(&hb, "X-Gateway-Request-Id: %s\r\n\r\n", x.ev.RequestID)
	if _, err := io.WriteString(clientConn, hb.String()); err != nil {
		clientConn.Close()
		upConn.Close()
		return err
	}
	x.ev.HTTPStatus = http.StatusSwitchingProtocols

	ctx, cancel := context.WithTimeout(clientReq.Context(), s.sessions.maxDuration())
	defer cancel()

	var once sync.Once
	closeBoth := func() {
		once.Do(func() {
			clientConn.Close()
			upConn.Close()
		})
	}

	// Upstream reader may still hold post-101 bytes; clientRW may hold post-request bytes.
	upSrc := io.Reader(upReader)
	var clientSrc io.Reader = clientConn
	if clientRW != nil {
		clientSrc = clientRW
	}

	var wg sync.WaitGroup
	var clientAbort atomic.Bool
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer closeBoth()
		_, _ = io.Copy(clientConn, upSrc)
		if clientReq.Context().Err() == context.Canceled {
			clientAbort.Store(true)
		}
	}()
	go func() {
		defer wg.Done()
		defer closeBoth()
		_, _ = io.Copy(upConn, clientSrc)
		if clientReq.Context().Err() == context.Canceled {
			clientAbort.Store(true)
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		closeBoth()
		<-done
	}

	switch {
	case clientAbort.Load() || clientReq.Context().Err() == context.Canceled:
		x.ev.Status = hooks.StatusClientAbort
	default:
		x.ev.Status = hooks.StatusOK
	}
	return nil
}

func genWSKey() string {
	var b [16]byte
	// non-crypto fine for tests; use request id entropy
	copy(b[:], []byte(newRequestID()))
	return base64.StdEncoding.EncodeToString(b[:])
}

func wsAccept(key string) string {
	const magic = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	h := sha1.Sum([]byte(key + magic))
	return base64.StdEncoding.EncodeToString(h[:])
}

// dialUpstreamWebSocket opens a TCP (and optional TLS) connection for an
// HTTP Upgrade to the upstream WebSocket. Supports http/ws and https/wss.
func dialUpstreamWebSocket(ctx context.Context, upURL *url.URL) (net.Conn, error) {
	if upURL == nil {
		return nil, fmt.Errorf("nil upstream url")
	}
	host := upURL.Hostname()
	if host == "" {
		return nil, fmt.Errorf("upstream host required")
	}
	port := upURL.Port()
	useTLS := false
	switch strings.ToLower(upURL.Scheme) {
	case "https", "wss":
		useTLS = true
		if port == "" {
			port = "443"
		}
	case "http", "ws", "":
		if port == "" {
			port = "80"
		}
	default:
		return nil, fmt.Errorf("unsupported websocket scheme %q", upURL.Scheme)
	}
	addr := net.JoinHostPort(host, port)

	dctx := ctx
	var cancel context.CancelFunc
	if _, ok := ctx.Deadline(); !ok {
		dctx, cancel = context.WithTimeout(ctx, wsDialTimeout)
		defer cancel()
	}
	d := net.Dialer{Timeout: wsDialTimeout, KeepAlive: wsTCPKeepAlive}
	conn, err := d.DialContext(dctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(wsTCPKeepAlive)
	}
	if !useTLS {
		return conn, nil
	}
	cfg := tlsClientConfig()
	if cfg == nil {
		cfg = &tls.Config{MinVersion: tls.VersionTLS12}
	} else {
		cfg = cfg.Clone()
	}
	if cfg.ServerName == "" {
		cfg.ServerName = host
	}
	if cfg.MinVersion == 0 {
		cfg.MinVersion = tls.VersionTLS12
	}
	tlsConn := tls.Client(conn, cfg)
	if err := tlsConn.HandshakeContext(dctx); err != nil {
		conn.Close()
		return nil, fmt.Errorf("tls handshake: %w", err)
	}
	return tlsConn, nil
}
