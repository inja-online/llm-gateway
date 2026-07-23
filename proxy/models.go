package proxy

import (
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/inja-online/llm-gateway/config"
)

// Synthetic catalog fields for config-derived model entries (no upstream fan-out).
const (
	modelObject          = "model"
	modelsOwnedByGateway = "llm-gateway"
	// modelsCreated is fixed so discovery responses are deterministic.
	modelsCreated int64 = 0
)

// modelCapabilities is OpenAI-list-friendly modality metadata derived from
// provider kind defaults + YAML capabilities overrides. Field names match
// config modality vocabulary except text → chat (SDK-friendly).
type modelCapabilities struct {
	Chat            bool `json:"chat"`
	ImageGen        bool `json:"image_gen"`
	VideoGen        bool `json:"video_gen"`
	AudioSpeech     bool `json:"audio_speech"`
	AudioTranscribe bool `json:"audio_transcribe"`
	Realtime        bool `json:"realtime"`
}

// modelEntry is the OpenAI Models API object shape plus optional capabilities.
type modelEntry struct {
	ID           string             `json:"id"`
	Object       string             `json:"object"`
	Created      int64              `json:"created"`
	OwnedBy      string             `json:"owned_by"`
	Capabilities *modelCapabilities `json:"capabilities,omitempty"`
}

func capabilitiesFromProvider(p config.Provider) *modelCapabilities {
	c := p.EffectiveCapabilities()
	return &modelCapabilities{
		Chat:            c.Text,
		ImageGen:        c.ImageGen,
		VideoGen:        c.VideoGen,
		AudioSpeech:     c.AudioSpeech,
		AudioTranscribe: c.AudioTranscribe,
		Realtime:        c.Realtime,
	}
}

// resolveCatalogProvider picks the provider for capability flags on a catalog id.
// Alias keys resolve via alias target; provider/model uses the prefix; bare
// ids without a resolvable provider omit capabilities.
func resolveCatalogProvider(cfg *config.Config, id string) (config.Provider, bool) {
	if target, ok := cfg.Aliases[id]; ok {
		id = target
	}
	prov, _, ok := strings.Cut(id, "/")
	if !ok || prov == "" {
		return config.Provider{}, false
	}
	p, exists := cfg.Providers[prov]
	return p, exists
}

// buildModelsCatalog derives the public model list from config only:
// every alias key (owned_by=llm-gateway) plus every unique alias target
// as stored (owned_by=provider prefix). No live upstream calls.
// Each entry includes capabilities from the resolved provider when known.
func buildModelsCatalog(cfg *config.Config) []modelEntry {
	seen := make(map[string]modelEntry, len(cfg.Aliases)*2)

	for alias, target := range cfg.Aliases {
		entry := modelEntry{
			ID:      alias,
			Object:  modelObject,
			Created: modelsCreated,
			OwnedBy: modelsOwnedByGateway,
		}
		if p, ok := resolveCatalogProvider(cfg, alias); ok {
			entry.Capabilities = capabilitiesFromProvider(p)
		}
		seen[alias] = entry

		if target == "" {
			continue
		}
		if _, exists := seen[target]; exists {
			continue
		}
		owner := modelsOwnedByGateway
		if prov, _, ok := strings.Cut(target, "/"); ok && prov != "" {
			owner = prov
		}
		tEntry := modelEntry{
			ID:      target,
			Object:  modelObject,
			Created: modelsCreated,
			OwnedBy: owner,
		}
		if p, ok := resolveCatalogProvider(cfg, target); ok {
			tEntry.Capabilities = capabilitiesFromProvider(p)
		}
		seen[target] = tEntry
	}

	out := make([]modelEntry, 0, len(seen))
	for _, m := range seen {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// handleModelsList serves GET /v1/models.
// Default: OpenAI list envelope from config aliases only (no network).
//
// Live discovery:
//   - anthropic-version header → pure Anthropic upstream proxy (#126).
//   - ?live=1 → fan-out GET /models to all openai/openai_compat/anthropic
//     providers that have credentials, merge with config aliases as
//     provider/model ids. Failed providers are skipped.
//
// No usage event (discovery only).
func (s *Server) handleModelsList(w http.ResponseWriter, r *http.Request) {
	// Pure Anthropic dialect clients still get byte-proxied Anthropic /models.
	if r.Header.Get("anthropic-version") != "" {
		s.proxyAnthropicModels(w, r, "/models")
		return
	}
	catalog := buildModelsCatalog(s.cfg)
	if wantLiveModels(r) {
		catalog = s.mergeLiveModels(r, catalog)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   catalog,
	})
}

// handleModelsGet serves GET /v1/models/{id} — single model or OpenAI 404.
// {id...} allows slash-containing public ids (provider/model).
// Live Anthropic path: proxy GET {base}/models/{id} when anthropic-version or ?live=1.
// No usage event (discovery only).
func (s *Server) handleModelsGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.wantAnthropicLiveModels(r) {
		// Strip provider/ prefix for bare upstream model ids when present.
		upID := id
		if _, rest, ok := strings.Cut(id, "/"); ok && rest != "" {
			upID = rest
		}
		s.proxyAnthropicModels(w, r, "/models/"+upID)
		return
	}
	for _, m := range buildModelsCatalog(s.cfg) {
		if m.ID == id {
			writeJSON(w, http.StatusOK, m)
			return
		}
	}
	writeOpenAIError(w, http.StatusNotFound, "invalid_request_error",
		"The model '"+id+"' does not exist")
}

func (s *Server) wantAnthropicLiveModels(r *http.Request) bool {
	// Single-model GET still uses Anthropic live path for ?live=1 (proxy one id).
	if r.Header.Get("anthropic-version") != "" {
		return true
	}
	return wantLiveModels(r)
}

// proxyAnthropicModels forwards discovery to a kind:anthropic provider.
func (s *Server) proxyAnthropicModels(w http.ResponseWriter, r *http.Request, path string) {
	route, err := s.resolveAnthropicProvider(r)
	if err != nil {
		// OpenAI-shaped when no anthropic-version; Anthropic-shaped when present.
		if r.Header.Get("anthropic-version") != "" {
			writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	key := clientKey(r)
	if k, msg := s.resolveUpstreamKey(r, route.ProviderName, route.Provider); msg == "" && k != "" {
		key = k
	} else if msg != "" && key == "" {
		writeOpenAIError(w, http.StatusBadGateway, "api_error", msg)
		return
	}
	upReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, route.Provider.BaseURL+path+stripProviderQuery(r), nil)
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "api_error", "failed to build upstream request")
		return
	}
	applyAuth(upReq, route.Provider, key)
	copyForwardHeaders(upReq, r)
	resp, err := s.client.Do(upReq)
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "api_error", "upstream request failed")
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, s.bodyLimit()))
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "api_error", "failed to read upstream response")
		return
	}
	copyAllowlistedResponseHeaders(w.Header(), resp.Header)
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}
