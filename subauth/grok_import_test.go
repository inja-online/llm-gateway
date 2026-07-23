package subauth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestImportGrokCLIAuthJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	exp := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339Nano)
	payload := map[string]any{
		"https://auth.x.ai::" + GrokClientID: map[string]any{
			"key":            "access-token-value",
			"auth_mode":      "oidc",
			"refresh_token":  "refresh-token-value",
			"expires_at":     exp,
			"oidc_issuer":    GrokIssuer,
			"oidc_client_id": GrokClientID,
			"email":          "user@example.com",
		},
	}
	raw, _ := json.Marshal(payload)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := importGrokAuthFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.AccessToken != "access-token-value" || c.RefreshToken != "refresh-token-value" {
		t.Fatalf("%#v", c)
	}
	if c.ClientID != GrokClientID {
		t.Fatalf("client %q", c.ClientID)
	}
	if c.Expiry.IsZero() {
		t.Fatal("expected expiry")
	}
}

func TestImportGrokOpenClawProfiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth-profiles.json")
	raw := []byte(`{
  "profiles": {
    "xai:default": {
      "type": "oauth",
      "provider": "xai",
      "access": "at",
      "refresh": "rt",
      "expires": 1893456000
    }
  }
}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := importGrokAuthFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.AccessToken != "at" || c.RefreshToken != "rt" {
		t.Fatalf("%#v", c)
	}
}
