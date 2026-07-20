package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
)

func TestAnthropicMemoryStoresProxy(t *testing.T) {
	var got []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"ms_1"}`))
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(`
providers:
  anthropic: { kind: anthropic, base_url: "` + up.URL + `" }
defaults:
  anthropic_dialect: anthropic
`))
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/memory_stores", strings.NewReader(`{"name":"n"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	http.DefaultClient.Do(req)
	http.Get(gw.URL + "/v1/memory_stores/ms_1")
	reqI, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/memory_stores/ms_1/memories", strings.NewReader(`{}`))
	reqI.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(reqI)

	j := strings.Join(got, "\n")
	for _, w := range []string{"POST /memory_stores", "GET /memory_stores/ms_1", "POST /memory_stores/ms_1/memories"} {
		if !strings.Contains(j, w) {
			t.Fatalf("missing %s in %s", w, j)
		}
	}
}
