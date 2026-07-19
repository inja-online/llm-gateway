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
	// "bearer": force Authorization: Bearer (useful for some Vertex-style hosts).
	Auth string `yaml:"auth"`
	// ServiceAccountFile is an optional path to a GCP service-account JSON key.
	// Documented for operators; real ADC wiring uses a TokenSource (see proxy).
	// When set with auth: service_account, the path is recorded for helpers.
	ServiceAccountFile string `yaml:"service_account_file"`
	// Capabilities overrides kind defaults. Nil → DefaultCapabilities(Kind).
	// openai_compat defaults to text-only; set image_gen/video_gen/… explicitly.
	Capabilities *Capabilities `yaml:"capabilities"`
}

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
	AuthAPIKey         = "api_key"          // default: x-goog-api-key / Bearer / x-api-key from client or api_key_env
	AuthADC            = "adc"              // Google ADC / injected TokenSource → Authorization: Bearer
	AuthServiceAccount = "service_account"  // same as adc for token application; file path for operators
	AuthBearer         = "bearer"           // force Bearer (OpenAI-style) even for google-shaped hosts
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
		case "", AuthAPIKey, AuthADC, AuthServiceAccount, AuthBearer:
			// normalize empty → leave empty (treated as api_key)
			if p.Auth != "" {
				p.Auth = strings.ToLower(strings.TrimSpace(p.Auth))
				c.Providers[name] = p
			}
		default:
			return fmt.Errorf("config: provider %q: unknown auth %q (want api_key|adc|service_account|bearer)", name, p.Auth)
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
	return nil
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

// BodyLimit returns the effective request/response body size cap in bytes.
func (c *Config) BodyLimit() int64 {
	if c == nil || c.MaxBodyBytes <= 0 {
		return DefaultMaxBodyBytes
	}
	return c.MaxBodyBytes
}
