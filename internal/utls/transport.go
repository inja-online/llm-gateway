// Package utls provides a Chrome-like TLS HTTP client for hosts that reject
// stock Go TLS fingerprints (api.anthropic.com, chatgpt.com).
package utls

import (
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	tls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

// ProtectedHosts reject stock Go TLS fingerprints (Cloudflare / bot checks).
var ProtectedHosts = map[string]struct{}{
	"api.anthropic.com": {},
	"chatgpt.com":       {},
}

type roundTripper struct {
	mu          sync.Mutex
	connections map[string]*http2.ClientConn
	pending     map[string]*sync.Cond
}

func newRoundTripper() *roundTripper {
	return &roundTripper{
		connections: make(map[string]*http2.ClientConn),
		pending:     make(map[string]*sync.Cond),
	}
}

func (t *roundTripper) getOrCreateConnection(host, addr string) (*http2.ClientConn, error) {
	t.mu.Lock()
	if h2Conn, ok := t.connections[host]; ok && h2Conn.CanTakeNewRequest() {
		t.mu.Unlock()
		return h2Conn, nil
	}
	if cond, ok := t.pending[host]; ok {
		cond.Wait()
		if h2Conn, ok := t.connections[host]; ok && h2Conn.CanTakeNewRequest() {
			t.mu.Unlock()
			return h2Conn, nil
		}
	}
	cond := sync.NewCond(&t.mu)
	t.pending[host] = cond
	t.mu.Unlock()

	h2Conn, err := t.createConnection(host, addr)

	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.pending, host)
	cond.Broadcast()
	if err != nil {
		return nil, err
	}
	t.connections[host] = h2Conn
	return h2Conn, nil
}

func (t *roundTripper) createConnection(host, addr string) (*http2.ClientConn, error) {
	conn, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		return nil, err
	}
	tlsConn := tls.UClient(conn, &tls.Config{ServerName: host}, tls.HelloChrome_Auto)
	if err := tlsConn.Handshake(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	tr := &http2.Transport{}
	h2Conn, err := tr.NewClientConn(tlsConn)
	if err != nil {
		_ = tlsConn.Close()
		return nil, err
	}
	return h2Conn, nil
}

func (t *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	hostname := req.URL.Hostname()
	port := req.URL.Port()
	if port == "" {
		port = "443"
	}
	addr := net.JoinHostPort(hostname, port)
	h2Conn, err := t.getOrCreateConnection(hostname, addr)
	if err != nil {
		return nil, err
	}
	resp, err := h2Conn.RoundTrip(req)
	if err != nil {
		t.mu.Lock()
		if cached, ok := t.connections[hostname]; ok && cached == h2Conn {
			delete(t.connections, hostname)
		}
		t.mu.Unlock()
		return nil, err
	}
	return resp, nil
}

// Fallback uses Chrome TLS for ProtectedHosts, else the standard transport.
type Fallback struct {
	Utls     http.RoundTripper
	Fallback http.RoundTripper
}

func (f *Fallback) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme == "https" {
		if _, ok := ProtectedHosts[strings.ToLower(req.URL.Hostname())]; ok {
			return f.Utls.RoundTrip(req)
		}
	}
	return f.Fallback.RoundTrip(req)
}

// NewSubscriptionClient returns a client with Chrome-like TLS for protected hosts.
func NewSubscriptionClient() *http.Client {
	standard := &http.Transport{
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	}
	return &http.Client{
		Transport: &Fallback{
			Utls:     newRoundTripper(),
			Fallback: standard,
		},
	}
}

// HostNeeds reports whether baseURL's host should use utls.
func HostNeeds(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	_, ok := ProtectedHosts[strings.ToLower(u.Hostname())]
	return ok
}
