package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
)

func TestAnthropicSkillsProxy(t *testing.T) {
	var got []string
	var gotBeta string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path)
		if b := r.Header.Get("anthropic-beta"); b != "" {
			gotBeta = b
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"skill_1","type":"skill"}`))
	}))
	t.Cleanup(up.Close)

	cfg, err := config.Parse([]byte(`
providers:
  anthropic: { kind: anthropic, base_url: "` + up.URL + `" }
defaults:
  anthropic_dialect: anthropic
`))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/skills", strings.NewReader(`{"display_title":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "skills-2025-10-02")
	req.Header.Set("x-api-key", "sk-ant")
	resp, _ := http.DefaultClient.Do(req)
	io.ReadAll(resp.Body)
	resp.Body.Close()

	http.Get(gw.URL + "/v1/skills")
	http.Get(gw.URL + "/v1/skills/skill_1")
	reqV, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/skills/skill_1/versions", strings.NewReader(`{}`))
	reqV.Header.Set("Content-Type", "application/json")
	reqV.Header.Set("anthropic-version", "2023-06-01")
	http.DefaultClient.Do(reqV)

	joined := strings.Join(got, "\n")
	for _, want := range []string{
		"POST /skills",
		"GET /skills",
		"GET /skills/skill_1",
		"POST /skills/skill_1/versions",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in\n%s", want, joined)
		}
	}
	if gotBeta != "skills-2025-10-02" {
		t.Fatalf("beta not forwarded: %q", gotBeta)
	}
}
