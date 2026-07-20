package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
)

func TestGoogleFileSearchStoresProxy(t *testing.T) {
	var got []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"name":"fileSearchStores/fs1"}`))
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(`
providers:
  google: { kind: google, base_url: "` + up.URL + `" }
defaults:
  google_dialect: google
`))
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1beta/fileSearchStores", strings.NewReader(`{"displayName":"kb"}`))
	req.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req)
	http.Get(gw.URL + "/v1beta/fileSearchStores?provider=google")
	http.Get(gw.URL + "/v1beta/fileSearchStores/fs1/documents")
	reqU, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1beta/fileSearchStores/fs1/documents", strings.NewReader(`{}`))
	reqU.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(reqU)

	j := strings.Join(got, "\n")
	for _, w := range []string{"POST /fileSearchStores", "GET /fileSearchStores", "GET /fileSearchStores/fs1/documents", "POST /fileSearchStores/fs1/documents"} {
		if !strings.Contains(j, w) {
			t.Fatalf("missing %s in %s", w, j)
		}
	}
}
