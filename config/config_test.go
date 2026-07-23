package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseValid(t *testing.T) {
	cfg, err := Parse([]byte(`
listen: ":9000"
providers:
  anthropic: { kind: anthropic, base_url: "https://api.anthropic.com/" }
defaults:
  anthropic_dialect: anthropic
aliases:
  best: anthropic/claude-sonnet-5
hooks:
  jsonl: { output: stdout }
observe_dropped_fields: true
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":9000" {
		t.Errorf("listen = %q", cfg.Listen)
	}
	if cfg.Providers["anthropic"].BaseURL != "https://api.anthropic.com" {
		t.Errorf("trailing slash not trimmed: %q", cfg.Providers["anthropic"].BaseURL)
	}
	if !cfg.ObserveDroppedFields {
		t.Error("observe_dropped_fields want true")
	}
}

func TestParseRejects(t *testing.T) {
	cases := map[string]string{
		"no providers":             `listen: ":1"`,
		"bad kind":                 "providers:\n  x: { kind: nope, base_url: \"https://x\" }",
		"missing base_url":         "providers:\n  x: { kind: openai }",
		"bad alias":                "providers:\n  x: { kind: openai, base_url: \"https://x\" }\naliases:\n  a: noslash",
		"alias unknown provider":   "providers:\n  x: { kind: openai, base_url: \"https://x\" }\naliases:\n  a: other/m",
		"default unknown provider": "providers:\n  x: { kind: openai, base_url: \"https://x\" }\ndefaults:\n  openai_dialect: other",
		"unknown field":            "providers:\n  x: { kind: openai, base_url: \"https://x\", nope: 1 }",
		"webhook no url":           "providers:\n  x: { kind: openai, base_url: \"https://x\" }\nhooks:\n  webhook: { timeout: 1s }",
	}
	for name, yaml := range cases {
		if _, err := Parse([]byte(yaml)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestDefaultListen(t *testing.T) {
	cfg, err := Parse([]byte("providers:\n  x: { kind: openai, base_url: \"https://x\" }"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":8787" {
		t.Errorf("default listen = %q", cfg.Listen)
	}
}

func TestMaxBodyBytesDefaultAndOverride(t *testing.T) {
	cfg, err := Parse([]byte("providers:\n  x: { kind: openai, base_url: \"https://x\" }"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxBodyBytes != DefaultMaxBodyBytes || cfg.BodyLimit() != DefaultMaxBodyBytes {
		t.Fatalf("default MaxBodyBytes=%d", cfg.MaxBodyBytes)
	}
	cfg2, err := Parse([]byte("providers:\n  x: { kind: openai, base_url: \"https://x\" }\nmax_body_bytes: 4096\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.MaxBodyBytes != 4096 {
		t.Fatalf("override=%d", cfg2.MaxBodyBytes)
	}
	// nil receiver and non-positive MaxBodyBytes → default
	var nilCfg *Config
	if nilCfg.BodyLimit() != DefaultMaxBodyBytes {
		t.Fatalf("nil BodyLimit=%d", nilCfg.BodyLimit())
	}
	cfg.MaxBodyBytes = 0
	if cfg.BodyLimit() != DefaultMaxBodyBytes {
		t.Fatalf("zero MaxBodyBytes BodyLimit=%d", cfg.BodyLimit())
	}
	cfg.MaxBodyBytes = -1
	if cfg.BodyLimit() != DefaultMaxBodyBytes {
		t.Fatalf("negative MaxBodyBytes BodyLimit=%d", cfg.BodyLimit())
	}
}

func TestParseCachingAutoBreakpoints(t *testing.T) {
	cfg, err := Parse([]byte(`
providers:
  anthropic: { kind: anthropic, base_url: "https://api.anthropic.com" }
caching:
  auto_breakpoints:
    enabled: true
    min_chars: 1024
    targets: [system, tools]
`))
	if err != nil {
		t.Fatal(err)
	}
	ab := cfg.Caching.AutoBreakpoints
	if !ab.Enabled || ab.MinChars != 1024 {
		t.Fatalf("auto_breakpoints=%+v", ab)
	}
	if ab.AutoBreakpointMinChars() != 1024 {
		t.Fatalf("min=%d", ab.AutoBreakpointMinChars())
	}
	// Default min when unset
	cfg2, err := Parse([]byte(`
providers:
  x: { kind: openai, base_url: "https://x" }
caching:
  auto_breakpoints:
    enabled: true
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.Caching.AutoBreakpoints.AutoBreakpointMinChars() != DefaultAutoBreakpointMinChars {
		t.Fatalf("default min=%d", cfg2.Caching.AutoBreakpoints.AutoBreakpointMinChars())
	}
	tg := cfg2.Caching.AutoBreakpoints.AutoBreakpointTargets()
	if len(tg) != 2 || tg[0] != "system" || tg[1] != "tools" {
		t.Fatalf("default targets=%v", tg)
	}
}

func TestParseCachingRejectsBadTarget(t *testing.T) {
	_, err := Parse([]byte(`
providers:
  x: { kind: openai, base_url: "https://x" }
caching:
  auto_breakpoints:
    enabled: true
    targets: [messages]
`))
	if err == nil {
		t.Fatal("expected unknown target error")
	}
}

func TestParseCachingRejectsNegativeMinChars(t *testing.T) {
	_, err := Parse([]byte(`
providers:
  x: { kind: openai, base_url: "https://x" }
caching:
  auto_breakpoints:
    enabled: true
    min_chars: -1
`))
	if err == nil {
		t.Fatal("expected min_chars error")
	}
}

func TestAutoBreakpointTargetsNormalize(t *testing.T) {
	ab := AutoBreakpoints{Targets: []string{" Tools ", "", "SYSTEM", "tools", "system"}}
	got := ab.AutoBreakpointTargets()
	if len(got) != 2 || got[0] != "tools" || got[1] != "system" {
		t.Fatalf("got %v", got)
	}
	// blank-only list → empty after normalize (not the default; caller may still treat as empty)
	ab2 := AutoBreakpoints{Targets: []string{"", "  "}}
	if len(ab2.AutoBreakpointTargets()) != 0 {
		t.Fatalf("blank targets: %v", ab2.AutoBreakpointTargets())
	}
}

func TestHealthTimeoutDefaultAndOverride(t *testing.T) {
	var h HealthChecks
	if h.HealthTimeout() != 2*time.Second {
		t.Fatalf("default=%v", h.HealthTimeout())
	}
	h.Timeout = 5 * time.Second
	if h.HealthTimeout() != 5*time.Second {
		t.Fatalf("override=%v", h.HealthTimeout())
	}
	cfg, err := Parse([]byte(`
providers:
  x: { kind: openai, base_url: "https://x" }
health_checks:
  enabled: true
  timeout: 1500ms
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HealthChecks.HealthTimeout() != 1500*time.Millisecond {
		t.Fatalf("parsed timeout=%v", cfg.HealthChecks.HealthTimeout())
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/gw.yaml"
	if err := os.WriteFile(path, []byte(`
providers:
  openai: { kind: openai, base_url: "https://api.openai.com/v1/" }
  google: { kind: google, base_url: "https://generativelanguage.googleapis.com" }
defaults:
  openai_dialect: openai
hooks:
  webhook:
    url: "https://billing.example/hook"
    timeout: 3s
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Providers["openai"].BaseURL != "https://api.openai.com/v1" {
		t.Errorf("base_url trim: %q", cfg.Providers["openai"].BaseURL)
	}
	if cfg.Hooks.Webhook == nil || cfg.Hooks.Webhook.URL == "" {
		t.Fatal("webhook not loaded")
	}
	if _, err := Load(dir + "/missing.yaml"); err == nil {
		t.Fatal("expected missing file error")
	}
}

func TestGatewayListenEnvOverride(t *testing.T) {
	t.Setenv("GATEWAY_LISTEN", "0.0.0.0:9999")
	cfg, err := Parse([]byte(`
providers:
  x: { kind: openai, base_url: "https://x" }
listen: ":8787"
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "0.0.0.0:9999" {
		t.Fatalf("listen = %q", cfg.Listen)
	}
}

func TestWebhookDefaultTimeout(t *testing.T) {
	cfg, err := Parse([]byte(`
providers:
  x: { kind: openai, base_url: "https://x" }
hooks:
  webhook: { url: "https://example.com/hook" }
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Hooks.Webhook.Timeout != 3*time.Second {
		t.Fatalf("timeout = %v", cfg.Hooks.Webhook.Timeout)
	}
}

func TestEdgeKeysAndAuthMode(t *testing.T) {
	t.Setenv("EDGE_KEYS_TEST", " a ,b, a , ")
	cfg, err := Parse([]byte(`
providers:
  x: { kind: openai, base_url: "https://x", auth: ADC }
edge_auth:
  enabled: true
  keys: ["k1", " k1 ", "k2"]
  keys_env: EDGE_KEYS_TEST
`))
	if err != nil {
		t.Fatal(err)
	}
	keys := cfg.EdgeKeys()
	if len(keys) < 3 {
		t.Fatalf("keys %#v", keys)
	}
	// first-occurrence de-dupe: k1 once, k2, a, b (order may include env)
	seen := map[string]int{}
	for _, k := range keys {
		seen[k]++
		if seen[k] > 1 {
			t.Fatalf("duplicate %q in %#v", k, keys)
		}
	}
	if cfg.Providers["x"].AuthMode() != AuthADC && cfg.Providers["x"].AuthMode() != "adc" {
		// AuthMode lowercases
		if got := cfg.Providers["x"].AuthMode(); got != "adc" {
			t.Fatalf("auth mode %q", got)
		}
	}
	p := Provider{}
	if p.AuthMode() != AuthAPIKey {
		t.Fatalf("default auth %q", p.AuthMode())
	}
}

func TestValidateEdgeAuthErrors(t *testing.T) {
	_, err := Parse([]byte(`
providers:
  x: { kind: openai, base_url: "https://x" }
edge_auth:
  enabled: true
`))
	if err == nil {
		t.Fatal("expected error when no keys")
	}
	t.Setenv("EMPTY_EDGE_KEYS", "")
	_, err = Parse([]byte(`
providers:
  x: { kind: openai, base_url: "https://x" }
edge_auth:
  enabled: true
  keys_env: EMPTY_EDGE_KEYS
`))
	if err == nil {
		t.Fatal("expected error when keys_env empty")
	}
}

func TestOAuth2ConfigParseAndValidate(t *testing.T) {
	cfg, err := Parse([]byte(`
providers:
  openai:
    kind: openai
    base_url: "https://api.openai.com/v1"
    auth: oauth2
    oauth:
      token_url: "https://auth.example/token"
      client_id_env: CID
      client_secret_env: CSEC
      scopes: ["api"]
`))
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Providers["openai"]
	if p.AuthMode() != AuthOAuth2 {
		t.Fatalf("auth %q", p.AuthMode())
	}
	if !p.UsesTokenSource() {
		t.Fatal("expected UsesTokenSource")
	}
	if p.OAuth == nil || p.OAuth.TokenURL == "" {
		t.Fatal("oauth block missing")
	}
	grant, err := p.OAuth.EffectiveGrant()
	if err != nil || grant != OAuthGrantClientCredentials {
		t.Fatalf("grant %q %v", grant, err)
	}
}

func TestOAuth2RefreshGrantAuto(t *testing.T) {
	cfg, err := Parse([]byte(`
providers:
  x:
    kind: openai
    base_url: "https://x"
    auth: oauth2
    oauth:
      token_url: "https://auth.example/token"
      refresh_token_env: RT
      client_id: c
`))
	if err != nil {
		t.Fatal(err)
	}
	grant, err := cfg.Providers["x"].OAuth.EffectiveGrant()
	if err != nil || grant != OAuthGrantRefreshToken {
		t.Fatalf("grant %q %v", grant, err)
	}
}

func TestOAuth2ConfigRejects(t *testing.T) {
	cases := []struct {
		name string
		yaml string
	}{
		{"missing oauth block", `
providers:
  x: { kind: openai, base_url: "https://x", auth: oauth2 }
`},
		{"missing token_url", `
providers:
  x:
    kind: openai
    base_url: "https://x"
    auth: oauth2
    oauth:
      client_id: a
      client_secret: b
`},
		{"client_credentials missing secret", `
providers:
  x:
    kind: openai
    base_url: "https://x"
    auth: oauth2
    oauth:
      token_url: "https://t"
      client_id: a
      grant: client_credentials
`},
		{"oauth without auth oauth2", `
providers:
  x:
    kind: openai
    base_url: "https://x"
    oauth:
      token_url: "https://t"
      client_id: a
      client_secret: b
`},
		{"bad grant", `
providers:
  x:
    kind: openai
    base_url: "https://x"
    auth: oauth2
    oauth:
      token_url: "https://t"
      client_id: a
      client_secret: b
      grant: password
`},
		{"unknown oauth field", `
providers:
  x:
    kind: openai
    base_url: "https://x"
    auth: oauth2
    oauth:
      token_url: "https://t"
      client_id: a
      client_secret: b
      nope: 1
`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse([]byte(tc.yaml)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestClientBearerAuthMode(t *testing.T) {
	cfg, err := Parse([]byte(`
providers:
  x: { kind: openai, base_url: "https://x", auth: client_bearer }
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Providers["x"].AuthMode() != AuthClientBearer {
		t.Fatalf("%q", cfg.Providers["x"].AuthMode())
	}
	if cfg.Providers["x"].UsesTokenSource() {
		t.Fatal("client_bearer is not TokenSource mode")
	}
}

func TestOAuthResolvedSecrets(t *testing.T) {
	t.Setenv("CID_R", "id-env")
	t.Setenv("CSEC_R", "sec-env")
	t.Setenv("RT_R", "rt-env")
	o := &OAuthConfig{
		ClientIDEnv:     "CID_R",
		ClientSecretEnv: "CSEC_R",
		RefreshTokenEnv: "RT_R",
	}
	if o.ResolvedClientID() != "id-env" || o.ResolvedClientSecret() != "sec-env" || o.ResolvedRefreshToken() != "rt-env" {
		t.Fatalf("%q %q %q", o.ResolvedClientID(), o.ResolvedClientSecret(), o.ResolvedRefreshToken())
	}
	inline := &OAuthConfig{ClientID: "i", ClientSecret: "s", RefreshToken: "r"}
	if inline.ResolvedClientID() != "i" || inline.ResolvedClientSecret() != "s" || inline.ResolvedRefreshToken() != "r" {
		t.Fatal("inline")
	}
	if (*OAuthConfig)(nil).ResolvedClientID() != "" {
		t.Fatal("nil")
	}
	if (*OAuthConfig)(nil).ResolvedClientSecret() != "" || (*OAuthConfig)(nil).ResolvedRefreshToken() != "" {
		t.Fatal("nil secrets")
	}
}

func TestVertexBaseURL(t *testing.T) {
	got := VertexBaseURL("p1", "europe-west1")
	want := "https://europe-west1-aiplatform.googleapis.com/v1/projects/p1/locations/europe-west1/publishers/google"
	if got != want {
		t.Fatalf("%s", got)
	}
	if !strings.Contains(VertexBaseURL("p", "global"), "aiplatform.googleapis.com/v1/projects/p/locations/global") {
		t.Fatal(VertexBaseURL("p", "global"))
	}
	if VertexBaseURL("", "x") != "" {
		t.Fatal("empty project")
	}
}
