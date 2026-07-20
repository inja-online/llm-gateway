package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
)

func TestAnthropicTunnelsProxy(t *testing.T) {
	var got []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"tun_1"}`))
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

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/tunnels", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	http.DefaultClient.Do(req)
	http.Get(gw.URL + "/v1/tunnels")
	http.Get(gw.URL + "/v1/tunnels/tun_1")
	reqD, _ := http.NewRequest(http.MethodDelete, gw.URL+"/v1/tunnels/tun_1", nil)
	http.DefaultClient.Do(reqD)
	_ = io.Discard
	j := strings.Join(got, "\n")
	for _, w := range []string{"POST /tunnels", "GET /tunnels", "GET /tunnels/tun_1", "DELETE /tunnels/tun_1"} {
		if !strings.Contains(j, w) {
			t.Fatalf("missing %s in %s", w, j)
		}
	}
}
