package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/inja-online/llm-gateway/subauth"
)

func TestOAuthStartChatGPTStatusComplete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	h := New(Options{AuthPath: path}).Handler()

	oldStart, oldWait := startChatGPT, waitChatGPT
	startChatGPT = func(context.Context) (*subauth.ChatGPTFlow, error) {
		return &subauth.ChatGPTFlow{AuthorizeURL: "https://auth.openai.com/oauth/authorize?fake=1"}, nil
	}
	waitChatGPT = func(context.Context, *subauth.ChatGPTFlow) (subauth.Credential, error) {
		return subauth.Credential{
			Provider:     subauth.ProviderChatGPT,
			AccessToken:  "SECRETTOKEN",
			RefreshToken: "SECRETREFRESH",
			ClientSecret: "SECRETSECRET",
			Source:       "oauth_pkce",
			Expiry:       time.Now().Add(time.Hour),
		}, nil
	}
	t.Cleanup(func() { startChatGPT, waitChatGPT = oldStart, oldWait })

	rec := postJSON(h, "/v1/dashboard/oauth/start", `{"provider":"chatgpt"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("start status %d body %s", rec.Code, rec.Body.Bytes())
	}
	assertNoSecrets(t, rec.Body.Bytes(), "SECRETTOKEN", "SECRETREFRESH", "SECRETSECRET")
	var started map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started["kind"] != "redirect" || started["state"] != "pending" {
		t.Fatalf("start %+v", started)
	}
	if started["authorize_url"] != "https://auth.openai.com/oauth/authorize?fake=1" {
		t.Fatalf("authorize_url %+v", started)
	}

	st := waitOAuthState(t, h, "chatgpt", "complete")
	assertNoSecrets(t, mustJSON(t, st), "SECRETTOKEN", "SECRETREFRESH", "SECRETSECRET")
	if st["kind"] != "redirect" {
		t.Fatalf("status %+v", st)
	}

	store, err := subauth.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := store.Get(subauth.ProviderChatGPT)
	if !ok || c.AccessToken != "SECRETTOKEN" || c.Source != "oauth_pkce" {
		t.Fatalf("saved %+v ok=%v", c, ok)
	}
}

func TestOAuthStartGrokOmitsDeviceCode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	h := New(Options{AuthPath: path}).Handler()

	oldStart, oldPoll := startGrok, pollGrok
	startGrok = func(context.Context) (*subauth.GrokDevice, error) {
		return &subauth.GrokDevice{
			UserCode:        "WDJB-MJHT",
			VerificationURI: "https://accounts.x.ai/oauth2/device",
			DeviceCode:      "DEVICESECRET",
			TokenURL:        "https://auth.x.ai/oauth2/token",
			ExpiresIn:       600,
			Interval:        5,
		}, nil
	}
	pollGrok = func(context.Context, *subauth.GrokDevice) (subauth.Credential, error) {
		return subauth.Credential{
			Provider:     subauth.ProviderGrok,
			AccessToken:  "SECRETTOKEN",
			RefreshToken: "SECRETREFRESH",
			Source:       "device_code",
		}, nil
	}
	t.Cleanup(func() { startGrok, pollGrok = oldStart, oldPoll })

	rec := postJSON(h, "/v1/dashboard/oauth/start", `{"provider":"grok"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("start status %d body %s", rec.Code, rec.Body.Bytes())
	}
	body := rec.Body.Bytes()
	assertNoSecrets(t, body, "DEVICESECRET", "SECRETTOKEN", "SECRETREFRESH")
	if bytes.Contains(bytes.ToLower(body), []byte("device_code")) {
		t.Fatalf("device_code in body: %s", body)
	}
	var started map[string]any
	if err := json.Unmarshal(body, &started); err != nil {
		t.Fatal(err)
	}
	if started["kind"] != "device" || started["user_code"] != "WDJB-MJHT" {
		t.Fatalf("start %+v", started)
	}
	if started["verification_uri"] != "https://accounts.x.ai/oauth2/device" {
		t.Fatalf("verification_uri %+v", started)
	}
	if started["expires_in"] != float64(600) || started["interval"] != float64(5) {
		t.Fatalf("timing %+v", started)
	}

	st := waitOAuthState(t, h, "grok", "complete")
	assertNoSecrets(t, mustJSON(t, st), "DEVICESECRET", "SECRETTOKEN")
	if bytes.Contains(bytes.ToLower(mustJSON(t, st)), []byte("device_code")) {
		t.Fatalf("status leaked device_code: %s", mustJSON(t, st))
	}
}

func TestOAuthStartClaudeGemini400(t *testing.T) {
	h := New(Options{AuthPath: filepath.Join(t.TempDir(), "c.json")}).Handler()
	for _, p := range []string{"claude", "gemini"} {
		rec := postJSON(h, "/v1/dashboard/oauth/start", `{"provider":"`+p+`"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s start %d body %s", p, rec.Code, rec.Body.Bytes())
		}
		assertNoSecrets(t, rec.Body.Bytes())
	}
}

func TestOAuthStatusIdle(t *testing.T) {
	h := New(Options{AuthPath: filepath.Join(t.TempDir(), "c.json")}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/oauth/status?provider=chatgpt", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
	}
	assertNoSecrets(t, rec.Body.Bytes())
	var st map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st["state"] != "idle" {
		t.Fatalf("%+v", st)
	}
}

func TestOAuthCompleteClaude(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	h := New(Options{AuthPath: path}).Handler()

	rec := postJSON(h, "/v1/dashboard/oauth/complete", `{"provider":"claude","token":"sk-ant-oat-SECRETTOKEN"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("complete %d body %s", rec.Code, rec.Body.Bytes())
	}
	assertNoSecrets(t, rec.Body.Bytes(), "SECRETTOKEN", "sk-ant-oat-SECRETTOKEN")

	store, err := subauth.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := store.Get(subauth.ProviderClaude)
	if !ok || c.AccessToken != "sk-ant-oat-SECRETTOKEN" || c.Source != "setup_token" {
		t.Fatalf("saved %+v ok=%v", c, ok)
	}

	rec = postJSON(h, "/v1/dashboard/oauth/complete", `{"provider":"claude","token":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty token %d", rec.Code)
	}
}

func TestOAuthImportChatGPT(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", dir)
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(`{"access_token":"SECRETTOKEN","refresh_token":"SECRETREFRESH"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "credentials.json")
	h := New(Options{AuthPath: path}).Handler()

	rec := postJSON(h, "/v1/dashboard/oauth/import", `{"provider":"chatgpt"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("import %d body %s", rec.Code, rec.Body.Bytes())
	}
	assertNoSecrets(t, rec.Body.Bytes(), "SECRETTOKEN", "SECRETREFRESH")

	store, err := subauth.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := store.Get(subauth.ProviderChatGPT)
	if !ok || c.AccessToken != "SECRETTOKEN" {
		t.Fatalf("saved %+v ok=%v", c, ok)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/profiles", nil))
	assertNoSecrets(t, rec.Body.Bytes(), "SECRETTOKEN", "SECRETREFRESH")
}

func TestOAuthImportMissing400(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	h := New(Options{AuthPath: filepath.Join(t.TempDir(), "c.json")}).Handler()
	rec := postJSON(h, "/v1/dashboard/oauth/import", `{"provider":"chatgpt"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing import %d body %s", rec.Code, rec.Body.Bytes())
	}
	assertNoSecrets(t, rec.Body.Bytes())
}

func postJSON(h http.Handler, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	h.ServeHTTP(rec, req)
	return rec
}

func waitOAuthState(t *testing.T, h http.Handler, provider, want string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var last map[string]any
	for time.Now().Before(deadline) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/oauth/status?provider="+provider, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
		}
		assertNoSecrets(t, rec.Body.Bytes())
		last = map[string]any{}
		if err := json.Unmarshal(rec.Body.Bytes(), &last); err != nil {
			t.Fatal(err)
		}
		if last["state"] == want {
			return last
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for state %s last %+v", want, last)
	return last
}

func assertNoSecrets(t *testing.T, body []byte, extra ...string) {
	t.Helper()
	for _, s := range append([]string{"access_token", "refresh_token", "client_secret", "DeviceCode"}, extra...) {
		if s == "" {
			continue
		}
		if bytes.Contains(body, []byte(s)) {
			t.Fatalf("body contains %q: %s", s, body)
		}
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
