package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
)

func TestGoogleTunedModelsProxy(t *testing.T) {
	var got []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"name":"tunedModels/tm1"}`))
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

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1beta/tunedModels", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req)
	http.Get(gw.URL + "/v1beta/tunedModels")
	http.Get(gw.URL + "/v1beta/tunedModels/tm1")
	reqP, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1beta/tunedModels/tm1:generateContent", strings.NewReader(`{}`))
	reqP.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(reqP)

	j := strings.Join(got, "\n")
	// note: path param id may include :generateContent if rest empty - we use id with colon as full segment
	for _, w := range []string{"POST /tunedModels", "GET /tunedModels", "GET /tunedModels/tm1"} {
		if !strings.Contains(j, w) {
			t.Fatalf("missing %s in %s", w, j)
		}
	}
}
