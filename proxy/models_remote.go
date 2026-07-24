package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/inja-online/llm-gateway/subauth"
)

// Remote catalog URLs (override with INJA_GATEWAY_MODELS_URL, comma-separated).
// Fallback is the in-process subscriptionCatalog table.
var defaultModelsCatalogURLs = []string{
	"https://models.router-for.me/models.json",
	"https://raw.githubusercontent.com/router-for-me/models/refs/heads/main/models.json",
}

var (
	remoteCatalogMu   sync.RWMutex
	remoteCatalog     map[string][]string // provider key → model ids
	remoteCatalogAt   time.Time
	remoteCatalogOnce sync.Once
)

const remoteCatalogTTL = 6 * time.Hour

// startRemoteModelsRefresh kicks a background refresh of the subscription model catalog.
func startRemoteModelsRefresh() {
	remoteCatalogOnce.Do(func() {
		go func() {
			_ = refreshRemoteModelsCatalog(context.Background())
			t := time.NewTicker(remoteCatalogTTL)
			defer t.Stop()
			for range t.C {
				_ = refreshRemoteModelsCatalog(context.Background())
			}
		}()
	})
}

func modelsCatalogURLs() []string {
	if v := strings.TrimSpace(os.Getenv("INJA_GATEWAY_MODELS_URL")); v != "" {
		var out []string
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return defaultModelsCatalogURLs
}

// remoteCatalogJSON shape: { "claude": [{ "id": "..." }, ...], "codex-plus": [...], "xai": [...] }
type remoteCatalogJSON map[string][]struct {
	ID string `json:"id"`
}

func refreshRemoteModelsCatalog(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 12 * time.Second}
	for _, u := range modelsCatalogURLs() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "inja-llm-gateway/models-catalog")
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			continue
		}
		var raw remoteCatalogJSON
		if err := json.Unmarshal(body, &raw); err != nil {
			continue
		}
		mapped := mapRemoteCatalog(raw)
		if len(mapped) == 0 {
			continue
		}
		remoteCatalogMu.Lock()
		remoteCatalog = mapped
		remoteCatalogAt = time.Now()
		remoteCatalogMu.Unlock()
		return nil
	}
	return io.EOF
}

// mapRemoteCatalog maps remote keys to our subauth provider ids.
func mapRemoteCatalog(raw remoteCatalogJSON) map[string][]string {
	out := make(map[string][]string)
	// Prefer richer codex tiers when present.
	for _, key := range []string{"claude", "codex-pro", "codex-plus", "codex-team", "codex-free", "xai"} {
		list, ok := raw[key]
		if !ok {
			continue
		}
		var ids []string
		seen := map[string]bool{}
		for _, m := range list {
			id := strings.TrimSpace(m.ID)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
		if len(ids) == 0 {
			continue
		}
		switch key {
		case "claude":
			out[subauth.ProviderClaude] = ids
		case "codex-pro", "codex-plus", "codex-team", "codex-free":
			// First non-empty codex tier wins (pro before free in list order).
			if _, exists := out[subauth.ProviderChatGPT]; !exists {
				out[subauth.ProviderChatGPT] = ids
			}
		case "xai":
			out[subauth.ProviderGrok] = ids
		}
	}
	return out
}

// catalogIDsForProvider returns remote catalog if fresh, else static table.
func catalogIDsForProvider(cred string) []string {
	remoteCatalogMu.RLock()
	rc := remoteCatalog
	at := remoteCatalogAt
	remoteCatalogMu.RUnlock()
	if rc != nil && time.Since(at) < remoteCatalogTTL*2 {
		if ids := rc[cred]; len(ids) > 0 {
			return ids
		}
	}
	return subscriptionCatalog[cred]
}
