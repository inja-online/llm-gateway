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

type Config struct {
	Listen    string              `yaml:"listen"`
	Providers map[string]Provider `yaml:"providers"`
	Defaults  Defaults            `yaml:"defaults"`
	Aliases   map[string]string   `yaml:"aliases"` // public alias -> "provider/upstream-model"
	Hooks     Hooks               `yaml:"hooks"`
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
	return nil
}
