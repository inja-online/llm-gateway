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

	j := strings.Join(got, "\n")
	for _, w := range []string{
		"POST /cachedContents",
		"GET /cachedContents",
		"GET /cachedContents/cc1",
		"PATCH /cachedContents/cc1",
		"DELETE /cachedContents/cc1",
	} {
		if !strings.Contains(j, w) {
			t.Fatalf("missing %q in\n%s", w, j)
		}
	}
}
