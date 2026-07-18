package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

// captureHook counts OnUsage calls so discovery routes can assert no metering.
type captureHook struct{ n atomic.Int64 }

func (h *captureHook) OnUsage(context.Context, hooks.UsageEvent) { h.n.Add(1) }

func modelsTestConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Parse([]byte(`
providers:
  deepseek: { kind: openai_compat, base_url: "https://api.deepseek.com" }
  xai: { kind: openai_compat, base_url: "https://api.x.ai/v1" }
  anthropic: { kind: anthropic, base_url: "https://api.anthropic.com/v1" }
aliases:
  fast: deepseek/deepseek-chat
  grok: xai/grok-3
`))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestModelsListOpenAIShape(t *testing.T) {
	h := &captureHook{}
	srv := httptest.NewServer(NewServer(modelsTestConfig(t), h).Handler())
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type=%q", ct)
	}

	var out struct {
		Object string `json:"object"`
		Data   []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("json: %v body=%s", err, body)
	}
	if out.Object != "list" {
		t.Errorf("object=%q", out.Object)
	}

	byID := map[string]string{}
	for _, m := range out.Data {
		if m.Object != "model" {
			t.Errorf("entry %q object=%q", m.ID, m.Object)
		}
		if m.Created != 0 {
			t.Errorf("entry %q created=%d want 0", m.ID, m.Created)
		}
		byID[m.ID] = m.OwnedBy
	}

	// Alias keys + unique targets.
	want := map[string]string{
		"fast":                   "llm-gateway",
		"grok":                   "llm-gateway",
		"deepseek/deepseek-chat": "deepseek",
		"xai/grok-3":             "xai",
	}
	if len(byID) != len(want) {
		t.Fatalf("got %d entries %v want %d", len(byID), byID, len(want))
	}
	for id, owner := range want {
		if byID[id] != owner {
			t.Errorf("id %q owned_by=%q want %q", id, byID[id], owner)
		}
	}
	if h.n.Load() != 0 {
		t.Errorf("usage events=%d want 0", h.n.Load())
	}
}

func TestModelsListEmptyAliases(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "https://api.openai.com/v1" }
`))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var out struct {
		Object string            `json:"object"`
		Data   []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out.Object != "list" || len(out.Data) != 0 {
		t.Fatalf("want empty list, got %s", body)
	}
}

func TestModelsListDedupesSharedTargets(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  deepseek: { kind: openai_compat, base_url: "https://api.deepseek.com" }
aliases:
  fast: deepseek/deepseek-chat
  cheap: deepseek/deepseek-chat
`))
	if err != nil {
		t.Fatal(err)
	}
	catalog := buildModelsCatalog(cfg)
	var nTarget int
	for _, m := range catalog {
		if m.ID == "deepseek/deepseek-chat" {
			nTarget++
		}
	}
	if nTarget != 1 {
		t.Fatalf("target entries=%d want 1 (catalog=%v)", nTarget, catalog)
	}
	if len(catalog) != 3 { // fast, cheap, target
		t.Fatalf("len=%d want 3", len(catalog))
	}
}

func TestModelsGetFound(t *testing.T) {
	h := &captureHook{}
	srv := httptest.NewServer(NewServer(modelsTestConfig(t), h).Handler())
	t.Cleanup(srv.Close)

	// Alias id
	resp, err := http.Get(srv.URL + "/v1/models/fast")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("alias: %d %s", resp.StatusCode, body)
	}
	var m modelEntry
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	if m.ID != "fast" || m.Object != "model" || m.OwnedBy != "llm-gateway" {
		t.Errorf("alias entry: %+v", m)
	}

	// Slash-containing public id (provider/model)
	resp2, err := http.Get(srv.URL + "/v1/models/deepseek/deepseek-chat")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("target: %d %s", resp2.StatusCode, body2)
	}
	var m2 modelEntry
	json.Unmarshal(body2, &m2)
	if m2.ID != "deepseek/deepseek-chat" || m2.OwnedBy != "deepseek" {
		t.Errorf("target entry: %+v", m2)
	}

	if h.n.Load() != 0 {
		t.Errorf("usage events=%d want 0", h.n.Load())
	}
}

func TestModelsGetNotFound(t *testing.T) {
	h := &captureHook{}
	srv := httptest.NewServer(NewServer(modelsTestConfig(t), h).Handler())
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/v1/models/does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("json: %v body=%s", err, body)
	}
	if env.Error.Type != "invalid_request_error" {
		t.Errorf("type=%q", env.Error.Type)
	}
	if env.Error.Message == "" {
		t.Error("empty error message")
	}
	if h.n.Load() != 0 {
		t.Errorf("usage events=%d want 0", h.n.Load())
	}
}

func TestBuildModelsCatalogSorted(t *testing.T) {
	catalog := buildModelsCatalog(modelsTestConfig(t))
	for i := 1; i < len(catalog); i++ {
		if catalog[i-1].ID >= catalog[i].ID {
			t.Fatalf("not sorted: %q then %q", catalog[i-1].ID, catalog[i].ID)
		}
	}
}
