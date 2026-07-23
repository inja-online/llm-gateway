// Package config loads and validates the gateway's single YAML config file.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Provider kinds. A kind selects the egress adapter; a provider is a named,
// configured instance of a kind (many providers can share the openai_compat kind).
const (
	KindOpenAI       = "openai"
	KindOpenAICompat = "openai_compat"
	KindAnthropic    = "anthropic"
	KindGoogle       = "google"
)

type Provider struct {
	Kind    string `yaml:"kind"`
	BaseURL string `yaml:"base_url"` // includes version prefix, e.g. https://api.openai.com/v1
	// APIKeyEnv names an env var holding an upstream key. When set, it replaces
	// the client-forwarded key. When empty, the client's key is forwarded as-is.
	APIKeyEnv string `yaml:"api_key_env"`
	// Auth selects how the gateway authenticates to the upstream.
	// Empty or "api_key" (default): use client key / api_key_env with kind scheme.
	// "adc" / "service_account": OAuth2 access token via TokenSource (Bearer).
	// "oauth2": YAML oauth block → built-in TokenSource (client_credentials / refresh_token).
	// "client_bearer": always forward client Authorization Bearer; never replace with env.
	// "bearer": force Authorization: Bearer (useful for some Vertex-style hosts).
	Auth string `yaml:"auth"`
	// ServiceAccountFile is an optional path to a GCP service-account JSON key.
	// With auth: service_account (or adc), the binary auto-builds a JWT TokenSource
	// from this file when present (no SetTokenSource inject required).
	ServiceAccountFile string `yaml:"service_account_file"`
	// OAuth holds OAuth2 client settings when auth is oauth2 (#104).
	OAuth *OAuthConfig `yaml:"oauth"`
	// Capabilities overrides kind defaults. Nil → DefaultCapabilities(Kind).
	// openai_compat defaults to text-only; set image_gen/video_gen/… explicitly.
	Capabilities *Capabilities `yaml:"capabilities"`
}

// OAuthConfig is the YAML oauth: block for auth: oauth2 (#104).
// Secrets should come from env vars (*_env); inline fields are allowed for tests.
//
// Grant selection (when grant is empty):
//   - refresh_token if refresh_token / refresh_token_env is set
//   - else client_credentials
type OAuthConfig struct {
	TokenURL        string            `yaml:"token_url"`
	ClientID        string            `yaml:"client_id"`         // prefer client_id_env
	ClientIDEnv     string            `yaml:"client_id_env"`
	ClientSecret    string            `yaml:"client_secret"`     // prefer client_secret_env
	ClientSecretEnv string            `yaml:"client_secret_env"`
	RefreshToken    string            `yaml:"refresh_token"`     // prefer refresh_token_env
	RefreshTokenEnv string            `yaml:"refresh_token_env"`
	Scopes          []string          `yaml:"scopes"`
	Audience        string            `yaml:"audience"` // optional form field "audience"
	Extra           map[string]string `yaml:"extra"`    // extra form fields (no secrets in logs)
	// Grant overrides auto detection: "client_credentials" | "refresh_token".
	Grant string `yaml:"grant"`
}

// OAuth grant type constants.
const (
	OAuthGrantClientCredentials = "client_credentials"
	OAuthGrantRefreshToken      = "refresh_token"
)

// DefaultMaxBodyBytes is the request/response body cap when max_body_bytes is unset.
const DefaultMaxBodyBytes int64 = 32 << 20 // 32 MiB

// Realtime holds process-wide WebSocket session limits (PR5+).
type Realtime struct {
	MaxSessions       int `yaml:"max_sessions"`
	MaxSessionMinutes int `yaml:"max_session_minutes"`
}

type Defaults struct {
	OpenAIDialect    string `yaml:"openai_dialect"`    // provider for bare model ids on /v1/chat/completions
	AnthropicDialect string `yaml:"anthropic_dialect"` // provider for bare model ids on /v1/messages
	GoogleDialect    string `yaml:"google_dialect"`    // provider for bare model ids on Gemini generateContent
}

type JSONLHook struct {
	Output string `yaml:"output"` // "stdout", "stderr", or a file path
}

type WebhookHook struct {
	URL     string        `yaml:"url"`
	Timeout time.Duration `yaml:"timeout"`
}

type Hooks struct {
	JSONL   *JSONLHook   `yaml:"jsonl"`
	Webhook *WebhookHook `yaml:"webhook"`
}

// EdgeAuth is optional gateway-edge authentication. When enabled, every route
// except GET /healthz requires a matching key in Authorization: Bearer … or
// x-api-key. Distinct from provider api_key_env (upstream credentials).
//
// Prefer keys_env in production; keys may be listed inline for local tests.
// keys_env value is comma-separated (whitespace trimmed). Empty entries ignored.
type EdgeAuth struct {
	Enabled bool     `yaml:"enabled"`
	Keys    []string `yaml:"keys"`     // optional inline keys
	KeysEnv string   `yaml:"keys_env"` // env var with comma-separated keys
}

// Auth modes for a provider. Empty / "api_key" is the historical default.
const (
	AuthAPIKey         = "api_key"         // default: x-goog-api-key / Bearer / x-api-key from client or api_key_env
	AuthADC            = "adc"             // Google ADC / injected or auto SA TokenSource → Bearer
	AuthServiceAccount = "service_account" // same as adc for token application; auto SA from file when set
	AuthOAuth2         = "oauth2"          // YAML oauth block → built-in client_credentials / refresh_token TokenSource
	AuthClientBearer   = "client_bearer"   // always forward client Bearer; never replace with api_key_env
	AuthBearer         = "bearer"          // force Bearer (OpenAI-style) even for google-shaped hosts
)

type Config struct {
	Listen       string              `yaml:"listen"`
	Providers    map[string]Provider `yaml:"providers"`
	Defaults     Defaults            `yaml:"defaults"`
	Aliases      map[string]string   `yaml:"aliases"` // public alias -> "provider/upstream-model"
	Hooks        Hooks               `yaml:"hooks"`
	Realtime     Realtime            `yaml:"realtime"`
	EdgeAuth     EdgeAuth            `yaml:"edge_auth"`
	// MaxBodyBytes caps request and response bodies (bytes). 0 / unset → DefaultMaxBodyBytes (32 MiB).
	MaxBodyBytes int64 `yaml:"max_body_bytes"`
	// ObserveDroppedFields, when true, sets response header x-gateway-dropped-fields
	// (comma-separated field names only, never payloads) on cross-dialect translate
	// paths where known vendor fields are not mapped. Default false (#152).
	ObserveDroppedFields bool `yaml:"observe_dropped_fields"`
	// HealthChecks configures optional upstream provider probes (#94/#153).
	// Distinct from GET /healthz (process liveness only).
	HealthChecks HealthChecks `yaml:"health_checks"`
	// Caching holds optional prompt-caching helpers. Default off (never invent
	// cache breakpoints without operator opt-in).
	Caching Caching `yaml:"caching"`
}

// HealthChecks gates GET /v1/health/providers (default disabled).
type HealthChecks struct {
	// Enabled must be true for the route to probe upstreams (default false).
	Enabled bool `yaml:"enabled"`
	// Timeout per-provider probe; 0 → 2s.
	Timeout time.Duration `yaml:"timeout"`
}

// Caching is optional prompt-caching behavior (default all-off).
type Caching struct {
	// AutoBreakpoints optionally inserts Anthropic cache_control on translate
	// paths that rebuild toward Anthropic (OpenAI/Google → Anthropic). Never
	// applies on passthrough. Default disabled.
	AutoBreakpoints AutoBreakpoints `yaml:"auto_breakpoints"`
}

// AutoBreakpoints config for opt-in Anthropic cache_control injection.
type AutoBreakpoints struct {
	// Enabled must be true; default false.
	Enabled bool `yaml:"enabled"`
	// MinChars is the minimum total character length for a target before a
	// breakpoint is added. 0 / unset → DefaultAutoBreakpointMinChars (2048).
	MinChars int `yaml:"min_chars"`
	// Targets lists surfaces to mark: "system", "tools". Empty when enabled
	// → both. Unknown names are rejected at validate time.
	Targets []string `yaml:"targets"`
}

// DefaultAutoBreakpointMinChars is used when auto_breakpoints.min_chars is 0.
const DefaultAutoBreakpointMinChars = 2048

// HealthTimeout returns the per-provider probe timeout (default 2s).
func (h HealthChecks) HealthTimeout() time.Duration {
	if h.Timeout > 0 {
		return h.Timeout
	}
	return 2 * time.Second
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(raw)
}

func Parse(raw []byte) (*Config, error) {
	var c Config
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	c.applyEnv()
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// applyEnv overlays 12-factor env vars. Kept minimal on purpose.
//
//	GATEWAY_LISTEN  — bind address (e.g. ":8787" or "0.0.0.0:8787")
func (c *Config) applyEnv() {
	if v := os.Getenv("GATEWAY_LISTEN"); v != "" {
		c.Listen = v
	}
}

func (c *Config) validate() error {
	if c.Listen == "" {
		c.Listen = ":8787"
	}
	if c.MaxBodyBytes <= 0 {
		c.MaxBodyBytes = DefaultMaxBodyBytes
	}
	if c.Realtime.MaxSessions <= 0 {
		c.Realtime.MaxSessions = 1024
	}
	if c.Realtime.MaxSessionMinutes <= 0 {
		c.Realtime.MaxSessionMinutes = 60
	}
	if c.Hooks.Webhook != nil && c.Hooks.Webhook.Timeout <= 0 {
		c.Hooks.Webhook.Timeout = 3 * time.Second
	}
	if len(c.Providers) == 0 {
		return fmt.Errorf("config: at least one provider required")
	}
	for name, p := range c.Providers {
		switch p.Kind {
		case KindOpenAI, KindOpenAICompat, KindAnthropic, KindGoogle:
		default:
			return fmt.Errorf("config: provider %q: unknown kind %q", name, p.Kind)
		}
		if p.BaseURL == "" {
			return fmt.Errorf("config: provider %q: base_url required", name)
		}
		if strings.HasSuffix(p.BaseURL, "/") {
			p.BaseURL = strings.TrimRight(p.BaseURL, "/")
			c.Providers[name] = p
		}
		switch strings.ToLower(strings.TrimSpace(p.Auth)) {
		case "", AuthAPIKey, AuthADC, AuthServiceAccount, AuthOAuth2, AuthClientBearer, AuthBearer:
			// normalize empty → leave empty (treated as api_key)
			if p.Auth != "" {
				p.Auth = strings.ToLower(strings.TrimSpace(p.Auth))
				c.Providers[name] = p
			}
		default:
			return fmt.Errorf("config: provider %q: unknown auth %q (want api_key|adc|service_account|oauth2|client_bearer|bearer)", name, p.Auth)
		}
		if err := validateProviderOAuth(name, c.Providers[name]); err != nil {
			return err
		}
	}
	for alias, target := range c.Aliases {
		prov, _, ok := strings.Cut(target, "/")
		if !ok {
			return fmt.Errorf("config: alias %q: target %q must be provider/model", alias, target)
		}
		if _, exists := c.Providers[prov]; !exists {
			return fmt.Errorf("config: alias %q: unknown provider %q", alias, prov)
		}
	}
	for dialect, prov := range map[string]string{
		"openai_dialect":    c.Defaults.OpenAIDialect,
		"anthropic_dialect": c.Defaults.AnthropicDialect,
		"google_dialect":    c.Defaults.GoogleDialect,
	} {
		if prov == "" {
			continue
		}
		if _, exists := c.Providers[prov]; !exists {
			return fmt.Errorf("config: defaults.%s: unknown provider %q", dialect, prov)
		}
	}
	if c.Hooks.Webhook != nil && c.Hooks.Webhook.URL == "" {
		return fmt.Errorf("config: hooks.webhook: url required")
	}
	if err := c.validateEdgeAuth(); err != nil {
		return err
	}
	if err := c.validateCaching(); err != nil {
		return err
	}
	return nil
}

func (c *Config) validateCaching() error {
	ab := c.Caching.AutoBreakpoints
	if ab.MinChars < 0 {
		return fmt.Errorf("config: caching.auto_breakpoints.min_chars must be >= 0")
	}
	for _, t := range ab.Targets {
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "system", "tools":
		case "":
			continue
		default:
			return fmt.Errorf("config: caching.auto_breakpoints.targets: unknown %q (want system|tools)", t)
		}
	}
	return nil
}

// AutoBreakpointMinChars returns the effective min_chars (default 2048).
func (a AutoBreakpoints) AutoBreakpointMinChars() int {
	if a.MinChars > 0 {
		return a.MinChars
	}
	return DefaultAutoBreakpointMinChars
}

// AutoBreakpointTargets returns normalized targets; empty config → system+tools
// when enabled is considered by the caller.
func (a AutoBreakpoints) AutoBreakpointTargets() []string {
	if len(a.Targets) == 0 {
		return []string{"system", "tools"}
	}
	seen := make(map[string]bool, len(a.Targets))
	var out []string
	for _, t := range a.Targets {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

func (c *Config) validateEdgeAuth() error {
	if !c.EdgeAuth.Enabled {
		return nil
	}
	if len(c.EdgeAuth.Keys) == 0 && c.EdgeAuth.KeysEnv == "" {
		return fmt.Errorf("config: edge_auth.enabled requires keys and/or keys_env")
	}
	// Resolve env at validate time so misconfiguration fails fast; missing env
	// with only keys_env is an error when enabled.
	if keys := c.EdgeKeys(); len(keys) == 0 {
		return fmt.Errorf("config: edge_auth.enabled but no keys resolved (check keys / keys_env)")
	}
	return nil
}

// EdgeKeys returns the configured edge-auth secrets (inline + keys_env),
// trimmed, non-empty, de-duplicated by first occurrence.
func (c *Config) EdgeKeys() []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(k string) {
		k = strings.TrimSpace(k)
		if k == "" {
			return
		}
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	for _, k := range c.EdgeAuth.Keys {
		add(k)
	}
	if c.EdgeAuth.KeysEnv != "" {
		if raw := os.Getenv(c.EdgeAuth.KeysEnv); raw != "" {
			for _, part := range strings.Split(raw, ",") {
				add(part)
			}
		}
	}
	return out
}

// AuthMode returns the effective auth mode for the provider (default api_key).
func (p Provider) AuthMode() string {
	a := strings.ToLower(strings.TrimSpace(p.Auth))
	if a == "" {
		return AuthAPIKey
	}
	return a
}

// UsesTokenSource reports whether the provider authenticates via a server-held
// TokenSource (ADC, service_account, or oauth2) rather than a client/env API key.
func (p Provider) UsesTokenSource() bool {
	switch p.AuthMode() {
	case AuthADC, AuthServiceAccount, AuthOAuth2:
		return true
	default:
		return false
	}
}

// BodyLimit returns the effective request/response body size cap in bytes.
func (c *Config) BodyLimit() int64 {
	if c == nil || c.MaxBodyBytes <= 0 {
		return DefaultMaxBodyBytes
	}
	return c.MaxBodyBytes
}

func validateProviderOAuth(name string, p Provider) error {
	mode := p.AuthMode()
	if mode == AuthOAuth2 {
		if p.OAuth == nil {
			return fmt.Errorf("config: provider %q: auth oauth2 requires oauth block", name)
		}
		return p.OAuth.validate(name)
	}
	// oauth block only valid with auth: oauth2
	if p.OAuth != nil && mode != AuthOAuth2 {
		return fmt.Errorf("config: provider %q: oauth block requires auth: oauth2", name)
	}
	return nil
}

func (o *OAuthConfig) validate(providerName string) error {
	if o == nil {
		return fmt.Errorf("config: provider %q: oauth block required", providerName)
	}
	if strings.TrimSpace(o.TokenURL) == "" {
		return fmt.Errorf("config: provider %q: oauth.token_url required", providerName)
	}
	grant, err := o.EffectiveGrant()
	if err != nil {
		return fmt.Errorf("config: provider %q: %w", providerName, err)
	}
	switch grant {
	case OAuthGrantClientCredentials:
		if !o.hasClientID() {
			return fmt.Errorf("config: provider %q: oauth client_credentials requires client_id or client_id_env", providerName)
		}
		if !o.hasClientSecret() {
			return fmt.Errorf("config: provider %q: oauth client_credentials requires client_secret or client_secret_env", providerName)
		}
	case OAuthGrantRefreshToken:
		if !o.hasRefreshToken() {
			return fmt.Errorf("config: provider %q: oauth refresh_token grant requires refresh_token or refresh_token_env", providerName)
		}
	}
	return nil
}

// EffectiveGrant returns the OAuth grant type (auto or explicit).
// Auto: refresh_token when a refresh credential is configured; else client_credentials.
func (o *OAuthConfig) EffectiveGrant() (string, error) {
	if o == nil {
		return "", fmt.Errorf("oauth: nil config")
	}
	g := strings.ToLower(strings.TrimSpace(o.Grant))
	switch g {
	case OAuthGrantClientCredentials, OAuthGrantRefreshToken:
		return g, nil
	case "":
		if o.hasRefreshToken() {
			return OAuthGrantRefreshToken, nil
		}
		return OAuthGrantClientCredentials, nil
	default:
		return "", fmt.Errorf("oauth.grant %q unknown (want client_credentials|refresh_token)", o.Grant)
	}
}

func (o *OAuthConfig) hasClientID() bool {
	return o != nil && (strings.TrimSpace(o.ClientID) != "" || strings.TrimSpace(o.ClientIDEnv) != "")
}

func (o *OAuthConfig) hasClientSecret() bool {
	return o != nil && (strings.TrimSpace(o.ClientSecret) != "" || strings.TrimSpace(o.ClientSecretEnv) != "")
}

func (o *OAuthConfig) hasRefreshToken() bool {
	return o != nil && (strings.TrimSpace(o.RefreshToken) != "" || strings.TrimSpace(o.RefreshTokenEnv) != "")
}

// ResolvedClientID returns inline client_id or the value of client_id_env.
func (o *OAuthConfig) ResolvedClientID() string {
	if o == nil {
		return ""
	}
	if v := strings.TrimSpace(o.ClientID); v != "" {
		return v
	}
	if o.ClientIDEnv != "" {
		return strings.TrimSpace(os.Getenv(o.ClientIDEnv))
	}
	return ""
}

// ResolvedClientSecret returns inline client_secret or the value of client_secret_env.
func (o *OAuthConfig) ResolvedClientSecret() string {
	if o == nil {
		return ""
	}
	if v := strings.TrimSpace(o.ClientSecret); v != "" {
		return v
	}
	if o.ClientSecretEnv != "" {
		return strings.TrimSpace(os.Getenv(o.ClientSecretEnv))
	}
	return ""
}

// ResolvedRefreshToken returns inline refresh_token or the value of refresh_token_env.
func (o *OAuthConfig) ResolvedRefreshToken() string {
	if o == nil {
		return ""
	}
	if v := strings.TrimSpace(o.RefreshToken); v != "" {
		return v
	}
	if o.RefreshTokenEnv != "" {
		return strings.TrimSpace(os.Getenv(o.RefreshTokenEnv))
	}
	return ""
}
