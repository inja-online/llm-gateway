package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
)

func TestOpenRouterHelpersProxy(t *testing.T) {
	var got []string
	var gotReferer string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		if v := r.Header.Get("HTTP-Referer"); v != "" {
			gotReferer = v
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(`
providers:
  openrouter: { kind: openai_compat, base_url: "` + up.URL + `" }
defaults:
  openai_dialect: openrouter
`))
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodGet, gw.URL+"/v1/credits", nil)
	req.Header.Set("HTTP-Referer", "https://example.com")
	http.DefaultClient.Do(req)
	http.Get(gw.URL + "/v1/key")
	http.Get(gw.URL + "/v1/generation?id=gen-1")

	j := strings.Join(got, "\n")
	for _, w := range []string{"GET /credits", "GET /key", "GET /generation?id=gen-1"} {
		if !strings.Contains(j, w) {
			t.Fatalf("missing %s in %s", w, j)
		}
	}
	if gotReferer != "https://example.com" {
		t.Fatalf("referer %q", gotReferer)
	}
}
