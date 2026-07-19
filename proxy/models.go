package proxy

import (
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

// handleModelsList serves GET /v1/models — OpenAI list envelope from config.
// No usage event (discovery only).
func (s *Server) handleModelsList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   buildModelsCatalog(s.cfg),
	})
}

// handleModelsGet serves GET /v1/models/{id} — single model or OpenAI 404.
// {id...} allows slash-containing public ids (provider/model).
// No usage event (discovery only).
func (s *Server) handleModelsGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	for _, m := range buildModelsCatalog(s.cfg) {
		if m.ID == id {
			writeJSON(w, http.StatusOK, m)
			return
		}
	}
	writeOpenAIError(w, http.StatusNotFound, "invalid_request_error",
		"The model '"+id+"' does not exist")
}
