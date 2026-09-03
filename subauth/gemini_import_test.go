package subauth

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestImportGeminiJetskiToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jetski-standalone-oauth-token")
	exp := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339Nano)
	raw := []byte(`{
  "auth_method": "consumer",
  "token": {
    "access_token": "ya29.access",
    "refresh_token": "1//refresh",
    "token_type": "Bearer",
    "expiry": "` + exp + `"
  }
}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GEMINI_AUTH_FILE", path)
	c, err := ImportGeminiFromCLI()
	if err != nil {
		t.Fatal(err)
	}
	if c.AccessToken != "ya29.access" || c.RefreshToken != "1//refresh" {
		t.Fatalf("%#v", c)
	}
	if c.Provider != ProviderGemini {
		t.Fatalf("provider %q", c.Provider)
	}
	if c.ClientID != GeminiClientID {
		t.Fatalf("client %q", c.ClientID)
	}
	if c.TokenURL != GeminiTokenURL {
		t.Fatalf("token url %q", c.TokenURL)
	}
	if c.Expiry.IsZero() {
		t.Fatal("expected expiry")
	}
}

func TestImportGeminiClientSecretFromEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jetski.json")
	raw := []byte(`{"token":{"access_token":"at","refresh_token":"rt"}}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GEMINI_AUTH_FILE", path)
	t.Setenv("INJA_GATEWAY_GEMINI_CLIENT_SECRET", "secret-from-env")
	c, err := ImportGeminiFromCLI()
	if err != nil {
		t.Fatal(err)
	}
	if c.ClientSecret != "secret-from-env" {
		t.Fatalf("client secret %q", c.ClientSecret)
	}
}
