package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

func TestStaticTokenSource(t *testing.T) {
	ts := StaticTokenSource{AccessToken: "tok-1"}
	got, err := ts.Token(context.Background())
	if err != nil || got != "tok-1" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := (StaticTokenSource{}).Token(context.Background()); err == nil {
		t.Fatal("empty should error")
	}
}

func TestCachingTokenSource(t *testing.T) {
	var n atomic.Int32
	inner := FuncTokenSource(func(context.Context) (string, error) {
		n.Add(1)
		return fmt.Sprintf("t-%d", n.Load()), nil
	})
	c := &CachingTokenSource{Inner: inner, TTL: time.Hour}
	a, err := c.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("cache miss: %q vs %q", a, b)
	}
	if n.Load() != 1 {
		t.Fatalf("calls=%d", n.Load())
	}
	// expire
	c.mu.Lock()
	c.expiry = time.Now().Add(-time.Second)
	c.mu.Unlock()
	d, err := c.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d == a {
		t.Fatal("expected refresh")
	}
	if n.Load() != 2 {
		t.Fatalf("calls=%d", n.Load())
	}
}

func TestApplyAuthADCUsesBearer(t *testing.T) {
	req, _ := http.NewRequest("POST", "http://x", nil)
	applyAuth(req, config.Provider{Kind: config.KindGoogle, Auth: config.AuthADC}, "ya29.token")
	if got := req.Header.Get("Authorization"); got != "Bearer ya29.token" {
		t.Fatalf("%q", got)
	}
	if req.Header.Get("x-goog-api-key") != "" {
		t.Fatal("must not set api key header for ADC")
	}
}

func TestApplyAuthServiceAccountUsesBearer(t *testing.T) {
	req, _ := http.NewRequest("POST", "http://x", nil)
	applyAuth(req, config.Provider{Kind: config.KindGoogle, Auth: config.AuthServiceAccount}, "tok")
	if req.Header.Get("Authorization") != "Bearer tok" {
		t.Fatal(req.Header.Get("Authorization"))
	}
}

func TestApplyAuthBearerMode(t *testing.T) {
	req, _ := http.NewRequest("POST", "http://x", nil)
	applyAuth(req, config.Provider{Kind: config.KindGoogle, Auth: config.AuthBearer}, "k")
	if req.Header.Get("Authorization") != "Bearer k" {
		t.Fatal(req.Header.Get("Authorization"))
	}
	if req.Header.Get("x-goog-api-key") != "" {
		t.Fatal("bearer mode should not set goog api key")
	}
}

func TestGoogleADCWithInjectedTokenSource(t *testing.T) {
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.Header.Get("x-goog-api-key") != "" {
			t.Error("unexpected api key header")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{"text":"hi"}],"role":"model"}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1}}`)
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  vertex:
    kind: google
    base_url: %q
    auth: adc
    # service_account_file: /secrets/sa.json  # operator mounts read-only
defaults:
  google_dialect: vertex
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	srv := NewServer(cfg, col)
	srv.SetTokenSource("vertex", StaticTokenSource{AccessToken: "ya29.fake-adc"})
	gw := httptest.NewServer(srv.Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest("POST", gw.URL+"/v1beta/models/gemini-2.0-flash:generateContent",
		strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	req.Header.Set("Content-Type", "application/json")
	// Edge may be off; client key unused for ADC.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if gotAuth != "Bearer ya29.fake-adc" {
		t.Fatalf("auth=%q", gotAuth)
	}
	ev := col.one(t)
	if ev.Status != hooks.StatusOK {
		t.Fatalf("%+v", ev)
	}
}

func TestGoogleADCMissingTokenSource(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("upstream should not be called")
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  vertex: { kind: google, base_url: %q, auth: adc }
defaults:
  google_dialect: vertex
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest("POST", gw.URL+"/v1beta/models/gemini-2.0-flash:generateContent",
		strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "TokenSource") {
		t.Fatalf("%s", body)
	}
	col.one(t)
}

func TestConfigRejectsUnknownAuth(t *testing.T) {
	if _, err := config.Parse([]byte(`
providers:
  x: { kind: google, base_url: "https://x", auth: oauth_magic }
`)); err == nil {
		t.Fatal("expected unknown auth error")
	}
}
