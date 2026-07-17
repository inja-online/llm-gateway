package proxy

import (
	"fmt"
	"strings"

	"github.com/mamad/llm-gateway/config"
)

// Route is the result of resolving a public model id.
type Route struct {
	ProviderName  string
	Provider      config.Provider
	UpstreamModel string
}

// Dialect names used for default-provider lookup and usage events.
const (
	DialectOpenAI    = "openai"
	DialectAnthropic = "anthropic"
)

// Resolve maps a public model id to a provider and upstream model id.
//
// Resolution order:
//  1. alias table (exact match)
//  2. "provider/model" prefix
//  3. bare id -> the dialect's default provider
func Resolve(cfg *config.Config, dialect, model string) (Route, error) {
	if target, ok := cfg.Aliases[model]; ok {
		model = target
	}
	if prov, rest, ok := strings.Cut(model, "/"); ok {
		p, exists := cfg.Providers[prov]
		if !exists {
			return Route{}, fmt.Errorf("unknown provider %q in model %q", prov, model)
		}
		return Route{ProviderName: prov, Provider: p, UpstreamModel: rest}, nil
	}
	var def string
	switch dialect {
	case DialectOpenAI:
		def = cfg.Defaults.OpenAIDialect
	case DialectAnthropic:
		def = cfg.Defaults.AnthropicDialect
	}
	if def == "" {
		return Route{}, fmt.Errorf("model %q has no provider prefix and no default provider is configured for the %s dialect", model, dialect)
	}
	return Route{ProviderName: def, Provider: cfg.Providers[def], UpstreamModel: model}, nil
}
