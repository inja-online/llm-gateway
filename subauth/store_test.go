package subauth

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")

	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Credentials) != 0 {
		t.Fatalf("want empty, got %#v", s.Credentials)
	}

	s.Put(Credential{
		Provider:     ProviderChatGPT,
		AccessToken:  "at",
		RefreshToken: "rt",
		ClientID:     ChatGPTClientID,
		TokenURL:     ChatGPTTokenURL,
		Expiry:       time.Now().Add(time.Hour),
		Source:       "test",
	})
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}

	s2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := s2.Get(ProviderChatGPT)
	if !ok || c.AccessToken != "at" || c.RefreshToken != "rt" {
		t.Fatalf("reload: %#v ok=%v", c, ok)
	}

	s2.Delete(ProviderChatGPT)
	if err := s2.Save(path); err != nil {
		t.Fatal(err)
	}
	s3, _ := Load(path)
	if _, ok := s3.Get(ProviderChatGPT); ok {
		t.Fatal("expected deleted")
	}
}

func TestGeneratePKCE(t *testing.T) {
	a, err := GeneratePKCE()
	if err != nil {
		t.Fatal(err)
	}
	b, err := GeneratePKCE()
	if err != nil {
		t.Fatal(err)
	}
	if a.Verifier == "" || a.Challenge == "" {
		t.Fatal("empty pkce")
	}
	if a.Verifier == b.Verifier {
		t.Fatal("pkce not random")
	}
}
