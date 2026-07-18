package proxy

import (
	"fmt"
	"strings"

	"github.com/inja-online/llm-gateway/config"
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
	DialectGoogle    = "google"
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
	case DialectGoogle:
		def = cfg.Defaults.GoogleDialect
	}
	if def == "" {
		return Route{}, fmt.Errorf("model %q has no provider prefix and no default provider is configured for the %s dialect", model, dialect)
	}
	return Route{ProviderName: def, Provider: cfg.Providers[def], UpstreamModel: model}, nil
}

type CapabilityError struct {
	Msg string
}

func (e *CapabilityError) Error() string { return e.Msg }

// CheckCapability fails closed when the provider cannot serve modality.
// Callers map a non-nil error into a dialect envelope with type
// unsupported_provider_capability and must not call upstream.
func CheckCapability(p config.Provider, providerName, modality string) error {
	if p.Supports(modality) {
		return nil
	}
	return &CapabilityError{Msg: fmt.Sprintf("provider %q does not support modality %q", providerName, modality)}
}

// ResolveForModality resolves a model then enforces capability for modality.
func ResolveForModality(cfg *config.Config, dialect, model, modality string) (Route, error) {
	route, err := Resolve(cfg, dialect, model)
	if err != nil {
		return Route{}, err
	}
	if err := CheckCapability(route.Provider, route.ProviderName, modality); err != nil {
		return Route{}, err
	}
	return route, nil
}

// Gateway logical error codes mapped into dialect envelopes.
const (
	CodeUnsupportedProviderCapability = "unsupported_provider_capability"
	CodeUnsupportedModality           = "unsupported_modality"
	CodeUnsupportedRealtimeBridge     = "unsupported_realtime_bridge"
	CodeInvalidRequest                = "invalid_request_error"
	CodeInvalidMediaRequest           = "invalid_media_request"
	CodeUpstreamError                 = "upstream_error"
)

// CapabilityError marks a fail-closed capability denial.

// CheckCapability returns a CapabilityError when unsupported.
func CheckCapabilityErr(p config.Provider, providerName, modality string) error {
	if p.Supports(modality) {
		return nil
	}
	return &CapabilityError{Msg: "provider " + providerName + " does not support modality " + modality}
}
func ResolveProvider(cfg *config.Config, name string) (Route, error) {
	p, ok := cfg.Providers[name]
	if !ok {
		return Route{}, fmt.Errorf("unknown provider %q", name)
	}
	return Route{ProviderName: name, Provider: p}, nil
}
