package proxy

import (
	"sort"
	"strings"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/subauth"
)

// Subscription model catalog (updated 2026-07-24).
// IDs match current vendor API ids and our example aliases.
// Only listed when oauth.credentials for that provider is usable.

// subscriptionCatalog maps subauth provider id → bare upstream model ids.
var subscriptionCatalog = map[string][]string{
	subauth.ProviderClaude: {
		"claude-sonnet-5",
		"claude-opus-4-8",
		"claude-haiku-4-5",
		"claude-fable-5",
		"claude-sonnet-4-6",
		"claude-opus-4-6",
		"claude-opus-4-7",
		"claude-haiku-4-5-20251001",
		"claude-sonnet-4-5-20250929",
		"claude-opus-4-5-20251101",
	},
	subauth.ProviderChatGPT: {
		"gpt-5.6-sol",
		"gpt-5.6-terra",
		"gpt-5.6-luna",
		"gpt-5.5",
		"gpt-5.4",
		"gpt-5.4-mini",
	},
	subauth.ProviderGrok: {
		"grok-4.5",
		"grok-4.3",
		"grok-build-0.1",
		"grok-composer-2.5-fast",
		"grok-3-mini",
	},
}

// providerHasUsableSubscription is true when the provider uses oauth.credentials
// and the local store can still authorize that provider.
func providerHasUsableSubscription(p config.Provider) bool {
	cred := subscriptionCredentialsID(p)
	if cred == "" {
		return false
	}
	path, err := subauth.ResolvePath()
	if err != nil {
		return false
	}
	return subauth.HasUsableCredential(path, cred)
}

// buildModelsCatalogCredentialAware derives the public model list:
//   - config aliases / targets only when the target provider is not a missing
//     subscription credential
//   - plus static subscription catalog entries for each logged-in provider
//
// Pure offline (no network). live fan-out is applied by the handler separately.
func buildModelsCatalogCredentialAware(cfg *config.Config) []modelEntry {
	if cfg == nil {
		return nil
	}
	// Which config provider names have usable subscription creds?
	usableByCred := map[string]bool{} // credentials id → usable
	providerCred := map[string]string{} // config provider name → credentials id
	for name, p := range cfg.Providers {
		if cred := subscriptionCredentialsID(p); cred != "" {
			providerCred[name] = cred
			if _, ok := usableByCred[cred]; !ok {
				usableByCred[cred] = providerHasUsableSubscription(p)
			}
		}
	}

	// Filter base catalog aliases.
	base := buildModelsCatalog(cfg)
	seen := make(map[string]modelEntry, len(base)+32)
	for _, m := range base {
		if shouldOmitCatalogEntry(cfg, m.ID, providerCred, usableByCred) {
			continue
		}
		seen[m.ID] = m
	}

	// Append static catalog models for logged-in subscription providers.
	for name, p := range cfg.Providers {
		cred, ok := providerCred[name]
		if !ok || !usableByCred[cred] {
			continue
		}
		ids := subscriptionCatalog[cred]
		caps := capabilitiesFromProvider(p)
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			public := name + "/" + id
			if _, exists := seen[public]; exists {
				continue
			}
			seen[public] = modelEntry{
				ID:           public,
				Object:       modelObject,
				Created:      modelsCreated,
				OwnedBy:      name,
				Capabilities: caps,
			}
		}
	}

	out := make([]modelEntry, 0, len(seen))
	for _, m := range seen {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// shouldOmitCatalogEntry drops alias/target entries whose provider lacks
// subscription credentials when that provider is oauth.credentials-gated.
func shouldOmitCatalogEntry(cfg *config.Config, id string, providerCred map[string]string, usableByCred map[string]bool) bool {
	// Resolve alias → target for provider prefix.
	target := id
	if t, ok := cfg.Aliases[id]; ok && t != "" {
		target = t
	}
	prov, _, ok := strings.Cut(target, "/")
	if !ok || prov == "" {
		// Bare id without provider: keep (legacy); routing uses dialects.
		return false
	}
	cred, gated := providerCred[prov]
	if !gated {
		return false
	}
	return !usableByCred[cred]
}

// modelsCatalog returns the credential-aware public model list for this server.
func (s *Server) modelsCatalog() []modelEntry {
	if s == nil || s.cfg == nil {
		return nil
	}
	return buildModelsCatalogCredentialAware(s.cfg)
}
