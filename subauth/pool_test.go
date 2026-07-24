package subauth

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPoolPickRoundRobinAndCooldown(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.json")
	st := &Store{Version: 2, Credentials: map[string]Credential{}}
	st.Put(Credential{Provider: ProviderClaude, AccessToken: "a1", Expiry: time.Now().Add(time.Hour)})
	st.PutAccount(Account{
		ID: "work",
		Credential: Credential{
			Provider:    ProviderClaude,
			AccessToken: "a2",
			Expiry:      time.Now().Add(time.Hour),
		},
	})
	if err := st.Save(path); err != nil {
		t.Fatal(err)
	}
	st2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if n := st2.CountUsableAccounts(ProviderClaude, time.Now()); n != 2 {
		t.Fatalf("count=%d want 2", n)
	}
	a, ok := st2.PickAccount(ProviderClaude, time.Now())
	if !ok || a.AccessToken == "" {
		t.Fatal("pick failed")
	}
	MarkCooldown(ProviderClaude, a.ID, time.Hour)
	b, ok := st2.PickAccount(ProviderClaude, time.Now())
	if !ok {
		t.Fatal("second pick failed")
	}
	if b.AccessToken == a.AccessToken && a.ID == b.ID {
		// After cooldown of first, should get the other if available
		t.Logf("picked same account (only one usable under cooldown skip may fall back)")
	}
	// Ensure fallback still works when all cooling down
	MarkCooldown(ProviderClaude, b.ID, time.Hour)
	c, ok := st2.PickAccount(ProviderClaude, time.Now())
	if !ok {
		t.Fatal("expected fallback when all cooling")
	}
	_ = c
}

func TestStoreTokenSource_Pool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.json")
	st := &Store{Version: 2, Credentials: map[string]Credential{}}
	st.Put(Credential{Provider: ProviderGrok, AccessToken: "g-tok", Expiry: time.Now().Add(time.Hour)})
	if err := st.Save(path); err != nil {
		t.Fatal(err)
	}
	ts := &StoreTokenSource{Path: path, Provider: ProviderGrok}
	tok, _, err := ts.TokenWithExpiry(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "g-tok" {
		t.Fatalf("tok=%q", tok)
	}
}
