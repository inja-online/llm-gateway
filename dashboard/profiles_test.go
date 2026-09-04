package dashboard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/inja-online/llm-gateway/subauth"
)

func TestProfilesHidesSecretsLogoutDisable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	st := &subauth.Store{Version: 2, Credentials: map[string]subauth.Credential{}}
	st.Put(subauth.Credential{
		Provider:     subauth.ProviderChatGPT,
		AccessToken:  "SECRETTOKEN",
		RefreshToken: "SECRETREFRESH",
		ClientSecret: "SECRETSECRET",
		Expiry:       time.Now().Add(time.Hour),
		Source:       "oauth_pkce",
	})
	if err := st.Save(path); err != nil {
		t.Fatal(err)
	}

	h := New(Options{AuthPath: path, Version: "test"}).Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/profiles", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET profiles status %d body %s", rec.Code, rec.Body.Bytes())
	}
	body := rec.Body.Bytes()
	for _, secret := range []string{"SECRETTOKEN", "SECRETREFRESH", "SECRETSECRET"} {
		if bytes.Contains(body, []byte(secret)) {
			t.Fatalf("profiles body leaked %s: %s", secret, body)
		}
	}
	var wrap struct {
		Profiles []ProfileDTO `json:"profiles"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		t.Fatal(err)
	}
	p := findProfile(t, wrap.Profiles, subauth.ProviderChatGPT, "")
	if !p.Usable || p.AccessState != "present" {
		t.Fatalf("chatgpt primary %+v", p)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/dashboard/profiles/chatgpt/logout", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout status %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/profiles", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &wrap); err != nil {
		t.Fatal(err)
	}
	p = findProfile(t, wrap.Profiles, subauth.ProviderChatGPT, "")
	if p.AccessState != "missing" {
		t.Fatalf("after logout %+v", p)
	}

	st, err := subauth.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	st.PutAccount(subauth.Account{
		ID: "work",
		Credential: subauth.Credential{
			Provider:    subauth.ProviderChatGPT,
			AccessToken: "POOLSECRET",
			Expiry:      time.Now().Add(time.Hour),
		},
	})
	if err := st.Save(path); err != nil {
		t.Fatal(err)
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/dashboard/profiles/chatgpt/disable", strings.NewReader(`{"disabled":true,"account":"work"}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("disable status %d body %s", rec.Code, rec.Body.Bytes())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/profiles", nil))
	body = rec.Body.Bytes()
	if bytes.Contains(body, []byte("POOLSECRET")) {
		t.Fatalf("leaked pool token: %s", body)
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		t.Fatal(err)
	}
	p = findProfile(t, wrap.Profiles, subauth.ProviderChatGPT, "work")
	if !p.Disabled {
		t.Fatalf("pool not disabled %+v", p)
	}
}

func TestUnknownProvider404(t *testing.T) {
	h := New(Options{AuthPath: filepath.Join(t.TempDir(), "c.json")}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/dashboard/profiles/nope/logout", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("logout %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/dashboard/profiles/nope/disable", strings.NewReader(`{"disabled":true}`)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("disable %d", rec.Code)
	}
}

func TestMeta(t *testing.T) {
	h := New(Options{Version: "v0.3.0"}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/meta", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["version"] != "v0.3.0" || m["dashboard"] != true {
		t.Fatalf("%v", m)
	}
	if m["sqlite"] == m["nodb"] {
		t.Fatalf("sqlite/nodb %v", m)
	}
	if m["ring_size"] != float64(2000) || m["sqlite_path_set"] != false {
		t.Fatalf("%v", m)
	}
}

func findProfile(t *testing.T, profiles []ProfileDTO, provider, account string) ProfileDTO {
	t.Helper()
	for _, p := range profiles {
		if p.Provider == provider && p.AccountID == account {
			return p
		}
	}
	t.Fatalf("missing profile %s account=%q in %+v", provider, account, profiles)
	return ProfileDTO{}
}
