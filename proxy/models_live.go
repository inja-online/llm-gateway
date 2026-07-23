package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/inja-online/llm-gateway/config"
)

// liveModelsTimeout is the per-provider budget for discovery fan-out.
const liveModelsTimeout = 4 * time.Second

// wantLiveModels is true when the client asked for live upstream discovery.
func wantLiveModels(r *http.Request) bool {
	v := r.URL.Query().Get("live")
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

// mergeLiveModels fans out GET /models to configured providers and merges
// results into the config catalog. Failures are skipped (config aliases remain).
// IDs are stored as provider/upstream-id so routing works without extra aliases.
func (s *Server) mergeLiveModels(r *http.Request, base []modelEntry) []modelEntry {
	if s == nil || s.cfg == nil {
		return base
	}
	seen := make(map[string]modelEntry, len(base)+32)
	for _, m := range base {
		seen[m.ID] = m
	}

	type job struct {
		name string
		p    config.Provider
	}
	var jobs []job
	for name, p := range s.cfg.Providers {
		switch p.Kind {
		case config.KindOpenAI, config.KindOpenAICompat, config.KindAnthropic:
			jobs = append(jobs, job{name: name, p: p})
		}
	}
	if len(jobs) == 0 {
		return base
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, j := range jobs {
		j := j
		wg.Add(1)
		go func() {
			defer wg.Done()
			entries := s.fetchProviderModels(r, j.name, j.p)
			if len(entries) == 0 {
				return
			}
			mu.Lock()
			for _, e := range entries {
				if _, ok := seen[e.ID]; !ok {
					seen[e.ID] = e
				}
			}
			mu.Unlock()
		}()
	}
	wg.Wait()

	out := make([]modelEntry, 0, len(seen))
	for _, m := range seen {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Server) fetchProviderModels(r *http.Request, name string, p config.Provider) []modelEntry {
	ctx, cancel := context.WithTimeout(r.Context(), liveModelsTimeout)
	defer cancel()

	// Credential resolution mirrors resolveUpstreamKey + applyAuth:
	// TokenSource modes need a source; api_key may use api_key_env with no client key.
	var key string
	if needsTokenSource(p) {
		k, msg := s.resolveUpstreamKey(r, name, p)
		if msg != "" || k == "" {
			return nil
		}
		key = k
	} else {
		key = clientKey(r)
		if p.APIKeyEnv != "" {
			if env := envLookup(p.APIKeyEnv); env != "" {
				key = env
			}
		}
		if key == "" {
			return nil
		}
	}

	base := strings.TrimRight(p.BaseURL, "/")
	// OpenAI-compat hosts expose GET {base}/models; Anthropic same under /v1.
	url := base + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	applyAuth(req, p, key)
	// Anthropic requires version header for native /models.
	if p.Kind == config.KindAnthropic && req.Header.Get("anthropic-version") == "" {
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	copyForwardHeaders(req, r)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, s.bodyLimit()))
	if err != nil {
		return nil
	}
	ids := parseUpstreamModelIDs(body)
	if len(ids) == 0 {
		return nil
	}
	caps := capabilitiesFromProvider(p)
	out := make([]modelEntry, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		// Avoid double-prefix if upstream already returned provider/model.
		public := id
		if !strings.Contains(id, "/") {
			public = name + "/" + id
		}
		out = append(out, modelEntry{
			ID:           public,
			Object:       modelObject,
			Created:      modelsCreated,
			OwnedBy:      name,
			Capabilities: caps,
		})
	}
	return out
}

// parseUpstreamModelIDs extracts model ids from OpenAI- or Anthropic-shaped lists.
func parseUpstreamModelIDs(body []byte) []string {
	var env struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		// Some hosts return a top-level models array.
		Models []struct {
			ID string `json:"id"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil
	}
	var ids []string
	for _, d := range env.Data {
		if d.ID != "" {
			ids = append(ids, d.ID)
		}
	}
	for _, d := range env.Models {
		if d.ID != "" {
			ids = append(ids, d.ID)
		}
	}
	return ids
}
