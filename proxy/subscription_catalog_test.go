package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/subauth"
)

func subscriptionCatalogConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Parse([]byte(`
providers:
  deepseek:
    kind: openai_compat
    base_url: "https://api.deepseek.com"
  anthropic:
    kind: anthropic
    base_url: "https://api.anthropic.com/v1"
    auth: oauth2
    oauth:
      credentials: claude
  chatgpt:
    kind: openai_compat
    base_url: "https://chatgpt.com/backend-api/codex"
    auth: oauth2
    oauth:
      credentials: chatgpt
aliases:
  fast: deepseek/deepseek-chat
  sonnet: anthropic/claude-sonnet-5
  gpt: chatgpt/gpt-5.6-terra
`))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestModelsCatalog_FiltersMissingSubscription(t *testing.T) {
	// Empty store: subscription aliases omitted; deepseek remains.
	dir := t.TempDir()
	path := filepath.Join(dir, "creds.json")
	t.Setenv("INJA_GATEWAY_AUTH_FILE", path)

	cfg := subscriptionCatalogConfig(t)
	catalog := buildModelsCatalogCredentialAware(cfg)
	byID := map[string]bool{}
	for _, m := range catalog {
		byID[m.ID] = true
	}
	if !byID["fast"] {
		t.Fatal("expected non-subscription alias fast")
	}
	if byID["sonnet"] || byID["gpt"] {
		t.Fatalf("subscription aliases should be filtered when logged out: %v", byID)
	}
	if byID["anthropic/claude-sonnet-5"] || byID["chatgpt/gpt-5.6-terra"] {
		t.Fatalf("subscription targets should be filtered: %v", byID)
	}
}

func TestModelsCatalog_IncludesWhenLoggedIn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "creds.json")
	st := &subauth.Store{Version: 1, Credentials: map[string]subauth.Credential{}}
	st.Put(subauth.Credential{
		Provider:    subauth.ProviderClaude,
		AccessToken: "oat",
		Expiry:      time.Now().Add(time.Hour),
	})
	if err := st.Save(path); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INJA_GATEWAY_AUTH_FILE", path)

	cfg := subscriptionCatalogConfig(t)
	catalog := buildModelsCatalogCredentialAware(cfg)
	byID := map[string]bool{}
	for _, m := range catalog {
		byID[m.ID] = true
	}
	if !byID["sonnet"] {
		t.Fatal("expected sonnet when claude logged in")
	}
	if !byID["anthropic/claude-sonnet-5"] {
		t.Fatal("expected anthropic/claude-sonnet-5")
	}
	// Catalog extras from static table
	if !byID["anthropic/claude-fable-5"] {
		t.Fatal("expected catalog id anthropic/claude-fable-5")
	}
	if byID["gpt"] {
		t.Fatal("chatgpt not logged in — gpt alias should be absent")
	}
}

func TestModelsListHTTP_CredentialAware(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "creds.json")
	t.Setenv("INJA_GATEWAY_AUTH_FILE", path)

	h := &captureHook{}
	srv := httptest.NewServer(NewServer(subscriptionCatalogConfig(t), h).Handler())
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
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	for _, m := range out.Data {
		if m.ID == "sonnet" || m.ID == "gpt" {
			t.Fatalf("unexpected subscription model while logged out: %s", m.ID)
		}
	}
}
