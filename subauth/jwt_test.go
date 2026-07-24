package subauth

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestParseAccountIDFromJWT(t *testing.T) {
	payload := map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "acct_test_123",
		},
		"email": "user@example.com",
	}
	raw, _ := json.Marshal(payload)
	token := "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(raw) + ".sig"

	got := ParseAccountIDFromJWT(token)
	if got != "acct_test_123" {
		t.Fatalf("got %q want acct_test_123", got)
	}
}

func TestParseAccountIDFromJWT_Empty(t *testing.T) {
	if ParseAccountIDFromJWT("") != "" {
		t.Fatal("empty token should yield empty id")
	}
	if ParseAccountIDFromJWT("not-a-jwt") != "" {
		t.Fatal("invalid jwt should yield empty id")
	}
}

func TestAccountIDFromTokens_PrefersIDToken(t *testing.T) {
	mk := func(id string) string {
		payload := map[string]any{
			"https://api.openai.com/auth": map[string]any{
				"chatgpt_account_id": id,
			},
		}
		raw, _ := json.Marshal(payload)
		return "x." + base64.RawURLEncoding.EncodeToString(raw) + ".y"
	}
	got := AccountIDFromTokens(mk("from_access"), mk("from_id"))
	if got != "from_id" {
		t.Fatalf("got %q want from_id", got)
	}
}

func TestCredentialUsable(t *testing.T) {
	now := time.Now()
	if (Credential{}).Usable(now) {
		t.Fatal("empty should be unusable")
	}
	if !(Credential{AccessToken: "tok"}).Usable(now) {
		t.Fatal("access without expiry should be usable")
	}
	if !(Credential{RefreshToken: "r"}).Usable(now) {
		t.Fatal("refresh-only should be usable")
	}
	expired := Credential{
		AccessToken: "old",
		Expiry:      now.Add(-time.Hour),
	}
	if expired.Usable(now) {
		t.Fatal("expired access without refresh unusable")
	}
	expired.RefreshToken = "r"
	if !expired.Usable(now) {
		t.Fatal("expired access with refresh usable")
	}
}

func TestHasUsableCredential(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/creds.json"
	st := &Store{Version: 1, Credentials: map[string]Credential{}}
	st.Put(Credential{Provider: ProviderClaude, AccessToken: "oat-1"})
	if err := st.Save(path); err != nil {
		t.Fatal(err)
	}
	if !HasUsableCredential(path, ProviderClaude) {
		t.Fatal("expected usable claude")
	}
	if HasUsableCredential(path, ProviderChatGPT) {
		t.Fatal("chatgpt should be missing")
	}
}
