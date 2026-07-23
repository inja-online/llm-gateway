package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/inja-online/llm-gateway/subauth"
)

func TestRunAuthHelpAndErrors(t *testing.T) {
	if err := runAuth(nil); err == nil {
		t.Fatal()
	}
	if err := runAuth([]string{"help"}); err != nil {
		t.Fatal(err)
	}
	if err := runAuth([]string{"nope"}); err == nil {
		t.Fatal()
	}
	if err := run([]string{"auth", "help"}); err != nil {
		t.Fatal(err)
	}
}

func TestAuthStatusLogoutEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	t.Setenv("INJA_GATEWAY_AUTH_FILE", path)

	st := &subauth.Store{Credentials: map[string]subauth.Credential{}}
	st.Put(subauth.Credential{
		Provider:    subauth.ProviderChatGPT,
		AccessToken: "at",
		Expiry:      time.Now().Add(time.Hour),
		Source:      "test",
	})
	if err := st.Save(path); err != nil {
		t.Fatal(err)
	}

	if err := authStatus(); err != nil {
		t.Fatal(err)
	}
	if err := authEnv([]string{"chatgpt"}); err != nil {
		t.Fatal(err)
	}
	if err := authEnv(nil); err != nil {
		t.Fatal(err)
	}
	if err := authLogout([]string{"chatgpt"}); err != nil {
		t.Fatal(err)
	}
	// empty status ok
	if err := authStatus(); err != nil {
		t.Fatal(err)
	}
	if err := authLogout([]string{"all"}); err != nil {
		t.Fatal(err)
	}
}

func TestAuthImportChatGPT(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", dir)
	t.Setenv("INJA_GATEWAY_AUTH_FILE", filepath.Join(dir, "gw-creds.json"))
	_ = os.WriteFile(filepath.Join(dir, "auth.json"), []byte(`{"access_token":"a","refresh_token":"r"}`), 0o600)
	if err := authImport([]string{"chatgpt"}); err != nil {
		t.Fatal(err)
	}
	if err := authLogin([]string{}); err == nil {
		t.Fatal("need provider")
	}
}

func TestAuthImportClaude(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	t.Setenv("INJA_GATEWAY_AUTH_FILE", filepath.Join(dir, "gw.json"))
	_ = os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(`{"accessToken":"ct"}`), 0o600)
	if err := authImport([]string{"claude"}); err != nil {
		t.Fatal(err)
	}
}

func TestAuthImportGrok(t *testing.T) {
	dir := t.TempDir()
	auth := filepath.Join(dir, "g.json")
	t.Setenv("GROK_AUTH_FILE", auth)
	t.Setenv("INJA_GATEWAY_AUTH_FILE", filepath.Join(dir, "gw.json"))
	_ = os.WriteFile(auth, []byte(`{"access_token":"ga","refresh_token":"gr"}`), 0o600)
	if err := authImport([]string{"grok"}); err != nil {
		t.Fatal(err)
	}
}

func TestFormatExpiry(t *testing.T) {
	if formatExpiry(time.Time{}) == "" {
		t.Fatal()
	}
	if formatExpiry(time.Now().Add(time.Hour)) == "" {
		t.Fatal()
	}
}

func TestAuthPath(t *testing.T) {
	t.Setenv("INJA_GATEWAY_AUTH_FILE", "/tmp/x.json")
	p, err := authPath()
	if err != nil || p != "/tmp/x.json" {
		t.Fatal(p, err)
	}
}

func TestSaveCred(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("INJA_GATEWAY_AUTH_FILE", filepath.Join(dir, "c.json"))
	if err := saveCred(subauth.Credential{Provider: "chatgpt", AccessToken: "z"}); err != nil {
		t.Fatal(err)
	}
}
