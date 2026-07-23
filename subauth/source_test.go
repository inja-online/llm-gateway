package subauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreTokenSourceRefresh(t *testing.T) {
	var n int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "refresh_token" {
			t.Errorf("grant %q", r.Form.Get("grant_type"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"expires_in":    3600,
			"token_type":    "Bearer",
		})
	}))
	t.Cleanup(srv.Close)

	path := filepath.Join(t.TempDir(), "cred.json")
	st := &Store{Version: 1, Credentials: map[string]Credential{}}
	st.Put(Credential{
		Provider:     ProviderGrok,
		AccessToken:  "old",
		RefreshToken: "rt",
		ClientID:     "cid",
		TokenURL:     srv.URL,
		Expiry:       time.Now().Add(-time.Minute), // force refresh
		Source:       "test",
	})
	if err := st.Save(path); err != nil {
		t.Fatal(err)
	}

	ts := &StoreTokenSource{Path: path, Provider: ProviderGrok}
	tok, exp, err := ts.TokenWithExpiry(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "new-access" {
		t.Fatalf("token %q", tok)
	}
	if exp.IsZero() || exp.Before(time.Now()) {
		t.Fatalf("expiry %v", exp)
	}
	if n != 1 {
		t.Fatalf("refresh calls %d", n)
	}

	// Second call should use cache on disk without another refresh (still valid).
	tok2, _, err := ts.TokenWithExpiry(context.Background())
	if err != nil || tok2 != "new-access" {
		t.Fatalf("tok2=%q err=%v", tok2, err)
	}
	if n != 1 {
		t.Fatalf("unexpected second refresh n=%d", n)
	}

	reloaded, _ := Load(path)
	c, _ := reloaded.Get(ProviderGrok)
	if c.RefreshToken != "new-refresh" || c.AccessToken != "new-access" {
		t.Fatalf("store not updated: %#v", c)
	}
}
