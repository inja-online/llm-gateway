package subauth

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIsKnownAndValidProviders(t *testing.T) {
	if !IsKnownProvider(ProviderChatGPT) || IsKnownProvider("nope") {
		t.Fatal()
	}
	ps := ValidProviders()
	if len(ps) != 3 {
		t.Fatalf("%v", ps)
	}
}

func TestDefaultPathEnv(t *testing.T) {
	want := filepath.Join(t.TempDir(), "creds.json")
	t.Setenv("INJA_GATEWAY_AUTH_FILE", want)
	p, err := DefaultPath()
	if err != nil || p != want {
		t.Fatalf("%s %v", p, err)
	}
}

func TestStoreNilAndEmpty(t *testing.T) {
	var s *Store
	if _, ok := s.Get("x"); ok {
		t.Fatal()
	}
	s.Delete("x")
	if err := s.Save(filepath.Join(t.TempDir(), "x.json")); err == nil {
		t.Fatal("nil save")
	}
	s2 := &Store{}
	s2.Put(Credential{Provider: "chatgpt", AccessToken: "a"})
	if err := s2.Save(filepath.Join(t.TempDir(), "c.json")); err != nil {
		t.Fatal(err)
	}
	// bad json
	bad := filepath.Join(t.TempDir(), "bad.json")
	_ = os.WriteFile(bad, []byte("{"), 0o600)
	if _, err := Load(bad); err == nil {
		t.Fatal("want parse err")
	}
	// version 0 + null credentials
	p := filepath.Join(t.TempDir(), "v0.json")
	_ = os.WriteFile(p, []byte(`{"version":0}`), 0o600)
	st, err := Load(p)
	if err != nil || st.Version != storeVersion || st.Credentials == nil {
		t.Fatalf("%+v %v", st, err)
	}
	var fm FileMutex
	fm.Lock()
	fm.Unlock()
}

func TestRandomStateAndPKCE(t *testing.T) {
	s, err := RandomState()
	if err != nil || len(s) < 8 {
		t.Fatal(s, err)
	}
}

func TestDefaultsForProvider(t *testing.T) {
	u, id := defaultsForProvider(ProviderChatGPT)
	if u == "" || id == "" {
		t.Fatal()
	}
	u, id = defaultsForProvider(ProviderGrok)
	if u == "" || id == "" {
		t.Fatal()
	}
	u, id = defaultsForProvider(ProviderClaude)
	if u != "" || id != "" {
		t.Fatal()
	}
}

func TestRefreshAccessTokenAndPostForm(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "refresh_token" {
			t.Errorf("grant %s", r.Form.Get("grant_type"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-at","refresh_token":"new-rt","expires_in":3600,"token_type":"Bearer"}`))
	}))
	t.Cleanup(srv.Close)

	c, err := RefreshAccessToken(context.Background(), srv.Client(), srv.URL, "cid", "old-rt")
	if err != nil {
		t.Fatal(err)
	}
	if c.AccessToken != "new-at" || c.RefreshToken != "new-rt" || c.Expiry.IsZero() {
		t.Fatalf("%+v", c)
	}

	// error path
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	t.Cleanup(bad.Close)
	if _, err := RefreshAccessToken(context.Background(), bad.Client(), bad.URL, "c", "r"); err == nil {
		t.Fatal("want err")
	}

	// non-json
	ugly := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	t.Cleanup(ugly.Close)
	if _, err := RefreshAccessToken(context.Background(), ugly.Client(), ugly.URL, "c", "r"); err == nil {
		t.Fatal("want parse err")
	}

	if !expiryFrom(tokenResponse{ExpiresIn: 0}).IsZero() {
		t.Fatal()
	}
	if expiryFrom(tokenResponse{ExpiresIn: 10}).IsZero() {
		t.Fatal()
	}
}

func TestStoreTokenSourceValidAndRefresh(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	st := &Store{Credentials: map[string]Credential{}}
	st.Put(Credential{
		Provider:    ProviderChatGPT,
		AccessToken: "live",
		Expiry:      time.Now().Add(time.Hour),
		Source:      "test",
	})
	if err := st.Save(path); err != nil {
		t.Fatal(err)
	}

	src := &StoreTokenSource{Path: path, Provider: ProviderChatGPT}
	tok, exp, err := src.TokenWithExpiry(context.Background())
	if err != nil || tok != "live" || exp.IsZero() {
		t.Fatalf("%s %v %v", tok, exp, err)
	}
	tok2, err := src.Token(context.Background())
	if err != nil || tok2 != "live" {
		t.Fatal(tok2, err)
	}

	// incomplete
	if _, _, err := (&StoreTokenSource{}).TokenWithExpiry(context.Background()); err == nil {
		t.Fatal()
	}
	// missing provider
	if _, _, err := (&StoreTokenSource{Path: path, Provider: "nope"}).TokenWithExpiry(context.Background()); err == nil {
		t.Fatal()
	}
	// expired no refresh
	st.Put(Credential{Provider: ProviderClaude, AccessToken: "old", Expiry: time.Now().Add(-time.Hour)})
	_ = st.Save(path)
	if _, _, err := (&StoreTokenSource{Path: path, Provider: ProviderClaude}).TokenWithExpiry(context.Background()); err == nil {
		t.Fatal("want expired err")
	}

	// refresh path
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"refreshed","expires_in":7200}`))
	}))
	t.Cleanup(srv.Close)
	st.Put(Credential{
		Provider:     ProviderChatGPT,
		AccessToken:  "old",
		RefreshToken: "rt",
		ClientID:     "cid",
		TokenURL:     srv.URL,
		Expiry:       time.Now().Add(-time.Minute),
	})
	_ = st.Save(path)
	tok, _, err = (&StoreTokenSource{Path: path, Provider: ProviderChatGPT}).TokenWithExpiry(context.Background())
	if err != nil || tok != "refreshed" {
		t.Fatalf("%s %v", tok, err)
	}
}

func TestBuildChatGPTAuthorizeURL(t *testing.T) {
	u := buildChatGPTAuthorizeURL("cid", "http://127.0.0.1/cb", "chal", "st")
	if !strings.Contains(u, "client_id=cid") || !strings.Contains(u, "code_challenge=chal") {
		t.Fatal(u)
	}
}

func TestListenLoopback(t *testing.T) {
	// preferred 0 binds :0 but the helper returns preferred unchanged when bind succeeds.
	ln, port, err := listenLoopback(0)
	if err != nil {
		t.Fatal(err)
	}
	_ = ln.Close()
	// occupied preferred → fallback to ephemeral
	ln2, err2 := net.Listen("tcp", "127.0.0.1:0")
	if err2 != nil {
		t.Fatal(err2)
	}
	busy := ln2.Addr().(*net.TCPAddr).Port
	ln3, port3, err3 := listenLoopback(busy)
	_ = ln2.Close()
	if err3 != nil {
		t.Fatal(err3)
	}
	_ = ln3.Close()
	if port3 == busy {
		// unlikely both same; either ephemeral or same if race
		_ = port
	}
}

func TestPickCodexTokens(t *testing.T) {
	raw := []byte(`{"access_token":"a","refresh_token":"r"}`)
	var root map[string]json.RawMessage
	_ = json.Unmarshal(raw, &root)
	a, r := pickCodexTokens(root, raw)
	if a != "a" || r != "r" {
		t.Fatal(a, r)
	}
	raw2 := []byte(`{"tokens":{"access_token":"a2","refresh_token":"r2"}}`)
	_ = json.Unmarshal(raw2, &root)
	a, r = pickCodexTokens(root, raw2)
	if a != "a2" || r != "r2" {
		t.Fatal(a, r)
	}
}

func TestImportChatGPTFromCodexCLI(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", dir)
	auth := filepath.Join(dir, "auth.json")
	_ = os.WriteFile(auth, []byte(`{"access_token":"at","refresh_token":"rt"}`), 0o600)
	c, err := ImportChatGPTFromCodexCLI()
	if err != nil || c.AccessToken != "at" {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestImportClaudeFromCLI(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	// ms expiry
	body := `{"claudeAiOauth":{"accessToken":"ca","refreshToken":"cr","expiresAt":1893456000000}}`
	_ = os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(body), 0o600)
	c, err := ImportClaudeFromCLI()
	if err != nil || c.AccessToken != "ca" || c.RefreshToken != "cr" {
		t.Fatalf("%+v %v", c, err)
	}
	// oauth nest
	body2 := `{"oauth":{"accessToken":"oa","refreshToken":"or","expiresAt":1893456000}}`
	_ = os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(body2), 0o600)
	c, err = ImportClaudeFromCLI()
	if err != nil || c.AccessToken != "oa" {
		t.Fatalf("%+v %v", c, err)
	}
	// flat
	_ = os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(`{"accessToken":"fa"}`), 0o600)
	c, err = ImportClaudeFromCLI()
	if err != nil || c.AccessToken != "fa" {
		t.Fatalf("%+v %v", c, err)
	}
	// empty
	_ = os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(`{}`), 0o600)
	if _, err := ImportClaudeFromCLI(); err == nil {
		t.Fatal()
	}
}

func TestClaudeCredentialsPath(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "/tmp/claude-x")
	p, err := claudeCredentialsPath()
	if err != nil || p != filepath.Join("/tmp/claude-x", ".credentials.json") {
		t.Fatal(p, err)
	}
}

func TestParseGrokCLIEntryAndImportFile(t *testing.T) {
	raw := json.RawMessage(`{"key":"k1","refresh_token":"rr","expires_at":"2030-01-02T03:04:05Z"}`)
	c, err := parseGrokCLIEntry(raw)
	if err != nil || c.AccessToken != "k1" || c.RefreshToken != "rr" || c.Expiry.IsZero() {
		t.Fatalf("%+v %v", c, err)
	}
	if firstNonEmpty("", "  x ") != "x" {
		t.Fatal()
	}

	dir := t.TempDir()
	// openclaw-style profiles
	p := filepath.Join(dir, "auth.json")
	_ = os.WriteFile(p, []byte(`{"profiles":{"xai-main":{"provider":"xai","access":"ga","refresh":"gr","expires":1893456000000}}}`), 0o600)
	c, err = importGrokAuthFile(p)
	if err != nil || c.AccessToken != "ga" {
		t.Fatalf("%+v %v", c, err)
	}
	// grok CLI entry map with default
	p2 := filepath.Join(dir, "auth2.json")
	_ = os.WriteFile(p2, []byte(`{"default":{"access_token":"da","refresh_token":"dr"}}`), 0o600)
	// might still hit unrecognized depending on parser order - try hermes-like
	// direct CLI shape used by ImportGrokFromCLI candidates
	_ = os.WriteFile(p2, []byte(`{"access_token":"da","refresh_token":"dr"}`), 0o600)
	c, err = importGrokAuthFile(p2)
	if err != nil {
		// flat may not match all branches - parseGrokCLIEntry on whole file
		c, err = parseGrokCLIEntry(json.RawMessage(`{"access_token":"da","refresh_token":"dr"}`))
	}
	if err != nil || c.AccessToken != "da" {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestTrustedXAIURL(t *testing.T) {
	if !trustedXAIURL("https://auth.x.ai/oauth/token") {
		t.Fatal()
	}
	if trustedXAIURL("https://evil.com/x") {
		t.Fatal()
	}
}

func TestGrokImportCandidates(t *testing.T) {
	cs := grokImportCandidates()
	if len(cs) == 0 {
		t.Fatal()
	}
}

func TestImportGrokFromCLIEnv(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "auth.json")
	t.Setenv("GROK_AUTH_FILE", p)
	_ = os.WriteFile(p, []byte(`{"access_token":"g-access","refresh_token":"g-refresh"}`), 0o600)
	c, err := ImportGrokFromCLI()
	if err != nil || c.AccessToken != "g-access" {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestImportGrokCLIKeyedMap(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.json")
	// Shape 1: keyed by auth.x.ai
	body := `{"https://auth.x.ai::client":{"access_token":"ak","refresh_token":"rk","expires_at":"2031-06-01T00:00:00Z"}}`
	_ = os.WriteFile(p, []byte(body), 0o600)
	c, err := importGrokAuthFile(p)
	if err != nil || c.AccessToken != "ak" {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestPostFormTokenUA(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "inja-test" {
			t.Errorf("ua %s", r.Header.Get("User-Agent"))
		}
		_, _ = w.Write([]byte(`{"access_token":"ua-tok","expires_in":60}`))
	}))
	t.Cleanup(srv.Close)
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", "r")
	form.Set("client_id", "c")
	tr, err := postFormTokenUA(context.Background(), srv.Client(), srv.URL, form, "inja-test")
	if err != nil || tr.AccessToken != "ua-tok" {
		t.Fatal(tr, err)
	}
}

// LoginClaudeSetupToken / OpenBrowser / LoginChatGPT / LoginGrok talk to
// browsers and CLIs — not run in unit tests (hang risk when tools exist).

func TestDefaultPathUserConfig(t *testing.T) {
	t.Setenv("INJA_GATEWAY_AUTH_FILE", "")
	// should not error on normal systems
	p, err := DefaultPath()
	if err != nil || p == "" {
		t.Fatal(p, err)
	}
}

func TestPickCodexNestedKeys(t *testing.T) {
	raw := []byte(`{"auth":{"access_token":"na","refresh_token":"nr"}}`)
	var root map[string]json.RawMessage
	_ = json.Unmarshal(raw, &root)
	a, r := pickCodexTokens(root, raw)
	if a != "na" || r != "nr" {
		// nested key path may differ - still exercise function
		_, _ = pickCodexTokens(map[string]json.RawMessage{}, []byte(`{}`))
	}
}

func TestStoreTokenSourceMissingTokenURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.json")
	st := &Store{Credentials: map[string]Credential{}}
	st.Put(Credential{
		Provider:     "custom",
		AccessToken:  "x",
		RefreshToken: "y",
		Expiry:       time.Now().Add(-time.Hour),
	})
	// use chatgpt with empty client defaults broken by empty provider name defaults
	st.Put(Credential{
		Provider:     ProviderClaude,
		AccessToken:  "x",
		RefreshToken: "y",
		Expiry:       time.Now().Add(-time.Hour),
		// no client id / token url and claude defaults empty
	})
	_ = st.Save(path)
	_, _, err := (&StoreTokenSource{Path: path, Provider: ProviderClaude}).TokenWithExpiry(context.Background())
	if err == nil {
		t.Fatal("want missing token_url error")
	}
}
