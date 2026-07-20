package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
)

func TestMoonshotEstimateTokenProxy(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"total_tokens":12}]}`))
	}))
	t.Cleanup(up.Close)

	cfg, err := config.Parse([]byte(`
providers:
  moonshot: { kind: openai_compat, base_url: "` + up.URL + `" }
defaults:
  openai_dialect: moonshot
`))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/tokenizers/estimate-token-count",
		strings.NewReader(`{"model":"kimi-latest","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer sk-moon")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d %s", resp.StatusCode, out)
	}
	if gotPath != "/tokenizers/estimate-token-count" {
		t.Fatalf("path=%q", gotPath)
	}
	if gotAuth != "Bearer sk-moon" {
		t.Fatalf("auth=%q", gotAuth)
	}
	if !strings.Contains(gotBody, "kimi-latest") {
		t.Fatalf("body=%s", gotBody)
	}
	if !strings.Contains(string(out), "total_tokens") {
		t.Fatalf("response=%s", out)
	}
}

func TestMoonshotBalanceProxy(t *testing.T) {
	var gotPath string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"available_balance":1.5}}`))
	}))
	t.Cleanup(up.Close)

	cfg, err := config.Parse([]byte(`
providers:
  moonshot_cn: { kind: openai_compat, base_url: "` + up.URL + `" }
defaults:
  openai_dialect: moonshot_cn
`))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodGet, gw.URL+"/v1/users/me/balance?provider=moonshot_cn", nil)
	req.Header.Set("Authorization", "Bearer sk-cn")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d %s", resp.StatusCode, out)
	}
	if gotPath != "/users/me/balance" {
		t.Fatalf("path=%q", gotPath)
	}
}

func TestMoonshotHelpersWrongKind(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  anthropic: { kind: anthropic, base_url: "http://127.0.0.1:9" }
defaults:
  openai_dialect: anthropic
`))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	resp, err := http.Get(gw.URL + "/v1/users/me/balance")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		t.Fatal("want error for non-openai family default")
	}
}
