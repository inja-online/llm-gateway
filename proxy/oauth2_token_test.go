package proxy

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

func TestOAuth2ClientCredentialsUpstreamAuth(t *testing.T) {
	var tokenCalls atomic.Int32
	var gotForm url.Values
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenCalls.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("method %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); !strings.Contains(ct, "application/x-www-form-urlencoded") {
			t.Errorf("ct %q", ct)
		}
		body, _ := io.ReadAll(r.Body)
		gotForm, _ = url.ParseQuery(string(body))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"at-cc-1","token_type":"Bearer","expires_in":3600}`)
	}))
	t.Cleanup(tokenSrv.Close)

	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"chatcmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai:
    kind: openai
    base_url: %q
    auth: oauth2
    oauth:
      token_url: %q
      client_id: test-client
      client_secret: test-secret
      scopes: ["api"]
      grant: client_credentials
defaults:
  openai_dialect: openai
`, upstream.URL, tokenSrv.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest("POST", gw.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	// Client key must not be used for oauth2.
	req.Header.Set("Authorization", "Bearer client-should-not-forward")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if gotAuth != "Bearer at-cc-1" {
		t.Fatalf("upstream auth=%q", gotAuth)
	}
	if gotForm.Get("grant_type") != "client_credentials" {
		t.Fatalf("grant=%q", gotForm.Get("grant_type"))
	}
	if gotForm.Get("client_id") != "test-client" || gotForm.Get("client_secret") != "test-secret" {
		t.Fatalf("form %#v", gotForm)
	}
	if gotForm.Get("scope") != "api" {
		t.Fatalf("scope=%q", gotForm.Get("scope"))
	}
	if tokenCalls.Load() != 1 {
		t.Fatalf("token calls=%d", tokenCalls.Load())
	}
	ev := col.one(t)
	if ev.Status != hooks.StatusOK {
		t.Fatalf("%+v", ev)
	}
	if ev.KeyHash == "" || ev.KeyHash == hashKey("client-should-not-forward") {
		t.Fatalf("key_hash should be of access token, got %q", ev.KeyHash)
	}
	if ev.KeyHash != hashKey("at-cc-1") {
		t.Fatalf("key_hash=%q want %q", ev.KeyHash, hashKey("at-cc-1"))
	}
}

func TestOAuth2RefreshTokenGrant(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		if form.Get("grant_type") != "refresh_token" {
			t.Errorf("grant %q", form.Get("grant_type"))
		}
		if form.Get("refresh_token") != "rt-1" {
			t.Errorf("refresh %q", form.Get("refresh_token"))
		}
		if form.Get("client_id") != "cid" {
			t.Errorf("client_id %q", form.Get("client_id"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"at-refresh","expires_in":120}`)
	}))
	t.Cleanup(tokenSrv.Close)

	ts, err := NewOAuth2TokenSource(&config.OAuthConfig{
		TokenURL:     tokenSrv.URL,
		ClientID:     "cid",
		ClientSecret: "sec",
		RefreshToken: "rt-1",
		Grant:        config.OAuthGrantRefreshToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	tok, exp, err := ts.TokenWithExpiry(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "at-refresh" {
		t.Fatalf("%q", tok)
	}
	if exp.IsZero() || exp.Before(time.Now()) {
		t.Fatalf("expiry %v", exp)
	}
}

func TestOAuth2CachingUsesExpiresIn(t *testing.T) {
	var n atomic.Int32
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := n.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":"tok-%d","expires_in":3600}`, c)
	}))
	t.Cleanup(tokenSrv.Close)

	inner, err := NewOAuth2TokenSource(&config.OAuthConfig{
		TokenURL:     tokenSrv.URL,
		ClientID:     "c",
		ClientSecret: "s",
		Grant:        config.OAuthGrantClientCredentials,
	})
	if err != nil {
		t.Fatal(err)
	}
	cache := &CachingTokenSource{Inner: inner}
	a, err := cache.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, err := cache.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if a != b || a != "tok-1" {
		t.Fatalf("%q %q calls=%d", a, b, n.Load())
	}
	if n.Load() != 1 {
		t.Fatalf("expected single token fetch, got %d", n.Load())
	}
	cache.Invalidate()
	c, err := cache.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if c != "tok-2" || n.Load() != 2 {
		t.Fatalf("%q calls=%d", c, n.Load())
	}
}

func TestOAuth2TokenEndpointErrorNoSecretLeak(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"invalid_client","error_description":"bad secret supersecret"}`)
	}))
	t.Cleanup(tokenSrv.Close)
	ts, err := NewOAuth2TokenSource(&config.OAuthConfig{
		TokenURL:     tokenSrv.URL,
		ClientID:     "c",
		ClientSecret: "supersecret",
		Grant:        config.OAuthGrantClientCredentials,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = ts.Token(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if strings.Contains(msg, "supersecret") {
		t.Fatalf("secret leaked in error: %s", msg)
	}
	if !strings.Contains(msg, "invalid_client") {
		t.Fatalf("%s", msg)
	}
	if strings.Contains(msg, "bad secret") {
		t.Fatalf("error_description leaked: %s", msg)
	}
}

func TestServiceAccountJWTSource(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})

	var gotAssertion string
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		if form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			t.Errorf("grant %q", form.Get("grant_type"))
		}
		gotAssertion = form.Get("assertion")
		if gotAssertion == "" || strings.Count(gotAssertion, ".") != 2 {
			t.Errorf("assertion %q", gotAssertion)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"ya29.from-sa","expires_in":3600}`)
	}))
	t.Cleanup(tokenSrv.Close)

	sa := map[string]string{
		"type":           "service_account",
		"client_email":   "gw@example.iam.gserviceaccount.com",
		"private_key":    string(pemBytes),
		"private_key_id": "kid-1",
		"token_uri":      tokenSrv.URL,
	}
	raw, _ := json.Marshal(sa)
	dir := t.TempDir()
	path := filepath.Join(dir, "sa.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ya29.from-sa" {
			t.Errorf("auth %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("x-goog-api-key") != "" {
			t.Error("must not set api key")
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
    auth: service_account
    service_account_file: %q
defaults:
  google_dialect: vertex
`, upstream.URL, path)))
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
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	col.one(t)
}

func TestClientBearerNeverReplacesWithEnv(t *testing.T) {
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"chatcmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	t.Cleanup(upstream.Close)

	t.Setenv("UPSTREAM_SHOULD_NOT_USE", "env-secret-key")
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai:
    kind: openai
    base_url: %q
    auth: client_bearer
    api_key_env: UPSTREAM_SHOULD_NOT_USE
defaults:
  openai_dialect: openai
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest("POST", gw.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer user-oauth-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if gotAuth != "Bearer user-oauth-token" {
		t.Fatalf("auth=%q (env must not replace)", gotAuth)
	}
}

func TestAPIKeyEnvStillReplacesWhenSet(t *testing.T) {
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"chatcmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	t.Cleanup(upstream.Close)

	t.Setenv("SERVER_HELD_KEY", "server-key")
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai:
    kind: openai
    base_url: %q
    api_key_env: SERVER_HELD_KEY
defaults:
  openai_dialect: openai
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest("POST", gw.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer client-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if gotAuth != "Bearer server-key" {
		t.Fatalf("auth=%q", gotAuth)
	}
}

func TestOAuth2MissingTokenSourceClearError(t *testing.T) {
	// auth oauth2 with unreachable token URL still auto-wires; missing oauth
	// is a config error. Empty TokenSource path: adc without file.
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
	// Ensure no ambient ADC file.
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
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
}

func TestApplyAuthOAuth2AndClientBearer(t *testing.T) {
	req, _ := http.NewRequest("POST", "http://x", nil)
	applyAuth(req, config.Provider{Kind: config.KindOpenAI, Auth: config.AuthOAuth2}, "at")
	if req.Header.Get("Authorization") != "Bearer at" {
		t.Fatal(req.Header.Get("Authorization"))
	}

	req, _ = http.NewRequest("POST", "http://x", nil)
	applyAuth(req, config.Provider{Kind: config.KindOpenAI, Auth: config.AuthClientBearer, APIKeyEnv: "X"}, "user")
	if req.Header.Get("Authorization") != "Bearer user" {
		t.Fatal(req.Header.Get("Authorization"))
	}
}

func TestOAuth2EnvResolution(t *testing.T) {
	t.Setenv("OAUTH_CID", "from-env-id")
	t.Setenv("OAUTH_CSEC", "from-env-sec")
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		if form.Get("client_id") != "from-env-id" || form.Get("client_secret") != "from-env-sec" {
			t.Errorf("form %#v", form)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"ok","expires_in":60}`)
	}))
	t.Cleanup(tokenSrv.Close)
	ts, err := NewOAuth2TokenSource(&config.OAuthConfig{
		TokenURL:        tokenSrv.URL,
		ClientIDEnv:     "OAUTH_CID",
		ClientSecretEnv: "OAUTH_CSEC",
		Grant:           config.OAuthGrantClientCredentials,
	})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := ts.Token(context.Background())
	if err != nil || tok != "ok" {
		t.Fatalf("%q %v", tok, err)
	}
}

func TestOAuth2ExtraAndAudience(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		if form.Get("audience") != "aud-1" || form.Get("resource") != "res-1" {
			t.Errorf("form %#v", form)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"x","expires_in":10}`)
	}))
	t.Cleanup(tokenSrv.Close)
	ts, err := NewOAuth2TokenSource(&config.OAuthConfig{
		TokenURL:     tokenSrv.URL,
		ClientID:     "c",
		ClientSecret: "s",
		Audience:     "aud-1",
		Extra:        map[string]string{"resource": "res-1", "": "skip"},
		Grant:        config.OAuthGrantClientCredentials,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ts.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestOAuth2BadJSONAndEmptyToken(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		fmt.Fprint(w, `not-json`)
	}))
	t.Cleanup(bad.Close)
	ts, _ := NewOAuth2TokenSource(&config.OAuthConfig{
		TokenURL: bad.URL, ClientID: "c", ClientSecret: "s", Grant: config.OAuthGrantClientCredentials,
	})
	if _, err := ts.Token(context.Background()); err == nil {
		t.Fatal("expected parse error")
	}

	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"access_token":""}`)
	}))
	t.Cleanup(empty.Close)
	ts2, _ := NewOAuth2TokenSource(&config.OAuthConfig{
		TokenURL: empty.URL, ClientID: "c", ClientSecret: "s", Grant: config.OAuthGrantClientCredentials,
	})
	if _, err := ts2.Token(context.Background()); err == nil {
		t.Fatal("expected empty token error")
	}
}

func TestNewOAuth2TokenSourceErrors(t *testing.T) {
	if _, err := NewOAuth2TokenSource(nil); err == nil {
		t.Fatal("nil")
	}
	if _, err := NewOAuth2TokenSource(&config.OAuthConfig{TokenURL: ""}); err == nil {
		t.Fatal("empty url")
	}
	if _, err := NewOAuth2TokenSource(&config.OAuthConfig{
		TokenURL: "https://t", Grant: config.OAuthGrantClientCredentials, ClientID: "c",
	}); err == nil {
		t.Fatal("missing secret")
	}
	if _, err := NewOAuth2TokenSource(&config.OAuthConfig{
		TokenURL: "https://t", Grant: config.OAuthGrantRefreshToken,
	}); err == nil {
		t.Fatal("missing refresh")
	}
}

func TestServiceAccountTokenMethodAndBadJSON(t *testing.T) {
	if _, err := NewServiceAccountJWTSourceFromJSON([]byte(`{`), nil); err == nil {
		t.Fatal("bad json")
	}
	if _, err := NewServiceAccountJWTSourceFromJSON([]byte(`{"client_email":"a"}`), nil); err == nil {
		t.Fatal("missing key")
	}
	if _, err := NewServiceAccountJWTSourceFromFile("/no/such/file.json", nil); err == nil {
		t.Fatal("missing file")
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, _ := x509.MarshalPKCS8PrivateKey(key)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"access_token":"via-token-method","expires_in":60}`)
	}))
	t.Cleanup(tokenSrv.Close)
	raw, _ := json.Marshal(map[string]string{
		"client_email": "a@b.com",
		"private_key":  string(pemBytes),
		"token_uri":    tokenSrv.URL,
	})
	src, err := NewServiceAccountJWTSourceFromJSON(raw, []string{"https://www.googleapis.com/auth/cloud-platform"})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := src.Token(context.Background())
	if err != nil || tok != "via-token-method" {
		t.Fatalf("%q %v", tok, err)
	}

	// incomplete source
	if _, err := (*ServiceAccountJWTSource)(nil).Token(context.Background()); err == nil {
		t.Fatal("nil sa")
	}
}

func TestAutoWireOAuth2BadEnvDeferredError(t *testing.T) {
	// Config validates env *names*; missing values fail at Token time via auto-wire.
	t.Setenv("MISSING_OAUTH_CID", "")
	t.Setenv("MISSING_OAUTH_CSEC", "")
	cfg, err := config.Parse([]byte(`
providers:
  openai:
    kind: openai
    base_url: "https://example.invalid"
    auth: oauth2
    oauth:
      token_url: "https://example.invalid/token"
      client_id_env: MISSING_OAUTH_CID
      client_secret_env: MISSING_OAUTH_CSEC
defaults:
  openai_dialect: openai
`))
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(cfg, &collector{})
	ts := s.tokenSource("openai")
	if ts == nil {
		t.Fatal("expected deferred error source")
	}
	if _, err := ts.Token(context.Background()); err == nil {
		t.Fatal("expected auto token source error")
	}
}

func TestCachingTokenSourceNilInnerAndExpiryFromInner(t *testing.T) {
	c := &CachingTokenSource{Inner: nil}
	if _, err := c.Token(context.Background()); err == nil {
		t.Fatal("nil inner")
	}
	(*CachingTokenSource)(nil).Invalidate()

	// Inner without TokenWithExpiry uses TTL
	var n atomic.Int32
	c2 := &CachingTokenSource{
		Inner: FuncTokenSource(func(context.Context) (string, error) {
			n.Add(1)
			return "t", nil
		}),
		TTL: time.Hour,
	}
	if _, err := c2.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := c2.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n.Load() != 1 {
		t.Fatalf("calls %d", n.Load())
	}
}

func TestTokenSourceFromProviderAPIKeyNil(t *testing.T) {
	ts, err := tokenSourceFromProvider(config.Provider{Auth: config.AuthAPIKey})
	if err != nil || ts != nil {
		t.Fatalf("%v %v", ts, err)
	}
}

func TestPKCS1PrivateKeyParse(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	got, err := parseRSAPrivateKey(string(pemBytes))
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if _, err := parseRSAPrivateKey("not-pem"); err == nil {
		t.Fatal("expected error")
	}
}

func TestOAuth2CustomHTTPClientAndSkew(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"access_token":"c","expires_in":100}`)
	}))
	t.Cleanup(tokenSrv.Close)
	ts, err := NewOAuth2TokenSource(&config.OAuthConfig{
		TokenURL: tokenSrv.URL, ClientID: "c", ClientSecret: "s", Grant: config.OAuthGrantClientCredentials,
	})
	if err != nil {
		t.Fatal(err)
	}
	ts.HTTPClient = tokenSrv.Client()
	ts.Skew = time.Second
	tok, exp, err := ts.TokenWithExpiry(context.Background())
	if err != nil || tok != "c" || exp.IsZero() {
		t.Fatalf("%q %v %v", tok, exp, err)
	}

	// no expires_in → zero expiry from fetch; CachingTokenSource fills TTL
	noExp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"access_token":"ne"}`)
	}))
	t.Cleanup(noExp.Close)
	ts2, _ := NewOAuth2TokenSource(&config.OAuthConfig{
		TokenURL: noExp.URL, ClientID: "c", ClientSecret: "s", Grant: config.OAuthGrantClientCredentials,
	})
	cache := &CachingTokenSource{Inner: ts2, TTL: time.Minute}
	if tok, err := cache.Token(context.Background()); err != nil || tok != "ne" {
		t.Fatalf("%q %v", tok, err)
	}
}

func TestServiceAccountCustomClientSkewDefaults(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	pkcs8, _ := x509.MarshalPKCS8PrivateKey(key)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"access_token":"sa","expires_in":3600}`)
	}))
	t.Cleanup(tokenSrv.Close)
	raw, _ := json.Marshal(map[string]string{
		"client_email": "a@b.com",
		"private_key":  string(pemBytes),
		"token_uri":    tokenSrv.URL,
	})
	src, err := NewServiceAccountJWTSourceFromJSON(raw, nil) // default scopes
	if err != nil {
		t.Fatal(err)
	}
	src.HTTPClient = tokenSrv.Client()
	src.Skew = time.Second
	tok, exp, err := src.TokenWithExpiry(context.Background())
	if err != nil || tok != "sa" || exp.IsZero() {
		t.Fatalf("%q %v %v", tok, exp, err)
	}
	// default token URL when empty
	src2 := &ServiceAccountJWTSource{Email: "a@b.com", PrivateKey: key, TokenURL: "", HTTPClient: tokenSrv.Client()}
	// will hit DefaultGoogleTokenURL which is real network — don't call.
	// instead verify skew/httpClient helpers
	if src2.skew() != DefaultOAuthSkew {
		t.Fatal(src2.skew())
	}
	if src2.httpClient() == nil {
		t.Fatal("client")
	}
}

func TestADCAutoWireFromEnvFile(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	pkcs8, _ := x509.MarshalPKCS8PrivateKey(key)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"access_token":"from-gac","expires_in":60}`)
	}))
	t.Cleanup(tokenSrv.Close)
	raw, _ := json.Marshal(map[string]string{
		"client_email": "a@b.com",
		"private_key":  string(pemBytes),
		"token_uri":    tokenSrv.URL,
	})
	path := filepath.Join(t.TempDir(), "gac.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", path)

	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{"text":"hi"}],"role":"model"}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1}}`)
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
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if gotAuth != "Bearer from-gac" {
		t.Fatalf("auth=%q", gotAuth)
	}
}

func TestOAuth2UnsupportedGrantAtTokenTime(t *testing.T) {
	ts := &OAuth2TokenSource{TokenURL: "http://127.0.0.1:1", GrantType: "password", ClientID: "c", ClientSecret: "s"}
	if _, err := ts.Token(context.Background()); err == nil {
		t.Fatal("expected unsupported grant")
	}
}

func TestOAuth2UnauthorizedForceRefreshRetry(t *testing.T) {
	var tokenCalls atomic.Int32
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := tokenCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		// First token is "stale"; second after Invalidate is fresh.
		fmt.Fprintf(w, `{"access_token":"tok-%d","expires_in":3600}`, n)
	}))
	t.Cleanup(tokenSrv.Close)

	var upCalls atomic.Int32
	var auths []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := upCalls.Add(1)
		auth := r.Header.Get("Authorization")
		auths = append(auths, auth)
		if n == 1 {
			// Simulate expired access token.
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":{"message":"invalid_token","type":"invalid_request_error"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"chatcmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai:
    kind: openai
    base_url: %q
    auth: oauth2
    oauth:
      token_url: %q
      client_id: c
      client_secret: s
      grant: client_credentials
defaults:
  openai_dialect: openai
`, upstream.URL, tokenSrv.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest("POST", gw.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if upCalls.Load() != 2 {
		t.Fatalf("upstream calls=%d want 2", upCalls.Load())
	}
	if tokenCalls.Load() != 2 {
		t.Fatalf("token calls=%d want 2 (initial + after invalidate)", tokenCalls.Load())
	}
	if len(auths) != 2 || auths[0] != "Bearer tok-1" || auths[1] != "Bearer tok-2" {
		t.Fatalf("auths=%v", auths)
	}
	col.one(t)
}

func TestOAuth2UnauthorizedNoRetryWithoutInvalidator(t *testing.T) {
	// StaticTokenSource has no Invalidate — 401 is returned as-is (one attempt).
	var upCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upCalls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"nope"}}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai:
    kind: openai
    base_url: %q
    auth: oauth2
    oauth:
      token_url: "https://example.invalid/token"
      client_id: c
      client_secret: s
defaults:
  openai_dialect: openai
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	srv := NewServer(cfg, col)
	// Override auto-wired OAuth with static (no Invalidate).
	srv.SetTokenSource("openai", StaticTokenSource{AccessToken: "static"})
	gw := httptest.NewServer(srv.Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest("POST", gw.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if upCalls.Load() != 1 {
		t.Fatalf("calls=%d", upCalls.Load())
	}
}

func TestTokenInvalidatorHelpers(t *testing.T) {
	if tokenInvalidatorFor(nil) != nil {
		t.Fatal("nil")
	}
	if tokenInvalidatorFor(StaticTokenSource{AccessToken: "x"}) != nil {
		t.Fatal("static has no Invalidate")
	}
	c := &CachingTokenSource{Inner: StaticTokenSource{AccessToken: "x"}}
	if tokenInvalidatorFor(c) == nil {
		t.Fatal("cache should invalidate")
	}
}

func TestOAuth2SkewAndHTTPClientDefaults(t *testing.T) {
	var o *OAuth2TokenSource
	if o.skew() != DefaultOAuthSkew {
		t.Fatal(o.skew())
	}
	if o.httpClient() == nil {
		t.Fatal("client")
	}
	o2 := &OAuth2TokenSource{}
	if o2.skew() != DefaultOAuthSkew || o2.httpClient() == nil {
		t.Fatal("defaults")
	}
}

func TestFileTokenSourceAndWIFWire(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tok")
	if err := os.WriteFile(path, []byte("  wif-access-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ts := FileTokenSource{Path: path}
	tok, err := ts.Token(context.Background())
	if err != nil || tok != "wif-access-token" {
		t.Fatalf("%q %v", tok, err)
	}
	if _, err := (FileTokenSource{}).Token(context.Background()); err == nil {
		t.Fatal("empty path")
	}
	if _, err := (FileTokenSource{Path: filepath.Join(dir, "missing")}).Token(context.Background()); err == nil {
		t.Fatal("missing file")
	}
	empty := filepath.Join(dir, "empty")
	_ = os.WriteFile(empty, []byte("  \n"), 0o600)
	if _, err := (FileTokenSource{Path: empty}).Token(context.Background()); err == nil {
		t.Fatal("empty token")
	}

	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"chatcmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai:
    kind: openai
    base_url: %q
    auth: adc
    token_file: %q
defaults:
  openai_dialect: openai
`, upstream.URL, path)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	req, _ := http.NewRequest("POST", gw.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if gotAuth != "Bearer wif-access-token" {
		t.Fatalf("auth=%q", gotAuth)
	}
	col.one(t)
}
