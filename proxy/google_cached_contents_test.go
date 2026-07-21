package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
)

func TestGoogleCachedContentsProxy(t *testing.T) {
	var got []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"cachedContents/cc1","model":"models/gemini-2.0-flash"}`))
	}))
	t.Cleanup(up.Close)

	cfg, err := config.Parse([]byte(`
providers:
  google: { kind: google, base_url: "` + up.URL + `" }
defaults:
  google_dialect: google
`))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1beta/cachedContents",
		strings.NewReader(`{"model":"models/gemini-2.0-flash","contents":[{"parts":[{"text":"sys"}]}]}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}

	http.Get(gw.URL + "/v1beta/cachedContents?provider=google")
	http.Get(gw.URL + "/v1beta/cachedContents/cc1")
	reqP, _ := http.NewRequest(http.MethodPatch, gw.URL+"/v1beta/cachedContents/cc1",
		strings.NewReader(`{"ttl":"3600s"}`))
	reqP.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(reqP)
	reqD, _ := http.NewRequest(http.MethodDelete, gw.URL+"/v1beta/cachedContents/cc1", nil)
	http.DefaultClient.Do(reqD)

	// Nested rest path (e.g. operations under a cache id).
	http.Get(gw.URL + "/v1beta/cachedContents/cc1/operations")

	j := strings.Join(got, "\n")
	for _, w := range []string{
		"POST /cachedContents",
		"GET /cachedContents",
		"GET /cachedContents/cc1",
		"PATCH /cachedContents/cc1",
		"DELETE /cachedContents/cc1",
		"GET /cachedContents/cc1/operations",
	} {
		if !strings.Contains(j, w) {
			t.Fatalf("missing %q in\n%s", w, j)
		}
	}
}

func TestGoogleCachedContentsMissingID(t *testing.T) {
	// Route requires {id}; empty path should 404 at mux, not hit handler.
	// Exercise handler path with a server that registers only the ID handler
	// is not needed — ID path always has a non-empty path value from mux.
	// Cover nested empty rest is already covered. Hit wrong method on root.
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(up.Close)
	cfg, err := config.Parse([]byte(`
providers:
  google: { kind: google, base_url: "` + up.URL + `" }
defaults:
  google_dialect: google
`))
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(cfg, nil)
	// Direct call with empty id (mux never does this, but covers guard).
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1beta/cachedContents/", nil)
	req.SetPathValue("id", "")
	s.handleGoogleCachedContentsID(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d %s", rr.Code, rr.Body.String())
	}
}
