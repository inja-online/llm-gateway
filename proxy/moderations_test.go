package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

func TestModerationsPassthrough(t *testing.T) {
	var gotModel, gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/moderations" {
			t.Errorf("path %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		gotModel, _ = body["model"].(string)
		fmt.Fprint(w, `{"id":"modr_1","results":[{"flagged":false}]}`)
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  openai_dialect: openai
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/moderations",
		strings.NewReader(`{"model":"openai/omni-moderation-latest","input":"hello"}`))
	req.Header.Set("Authorization", "Bearer sk-mod")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(string(body), "flagged") {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if gotModel != "omni-moderation-latest" || gotAuth != "Bearer sk-mod" {
		t.Fatalf("model=%s auth=%s", gotModel, gotAuth)
	}
	ev := col.one(t)
	if !ev.Estimated || ev.Status != hooks.StatusOK {
		t.Fatalf("%+v", ev)
	}
}

func TestModerationsNoModelUsesDefault(t *testing.T) {
	var hit bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		fmt.Fprint(w, `{"results":[]}`)
	}))
	gw, col := newTestGateway(t, upstream)
	resp, err := http.Post(gw.URL+"/v1/moderations", "application/json",
		strings.NewReader(`{"input":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !hit {
		t.Fatalf("status %d hit=%v", resp.StatusCode, hit)
	}
	col.one(t)
}

func TestModerationsRejectsGoogle(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  google: { kind: google, base_url: "https://generativelanguage.googleapis.com/v1beta" }
defaults:
  openai_dialect: google
`))
	if err != nil {
		t.Fatal(err)
	}
	// model forces google route
	cfg2, err := config.Parse([]byte(`
providers:
  google: { kind: google, base_url: "https://generativelanguage.googleapis.com/v1beta" }
  openai: { kind: openai, base_url: "https://api.openai.com/v1" }
defaults:
  openai_dialect: openai
`))
	if err != nil {
		t.Fatal(err)
	}
	_ = cfg
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg2, col).Handler())
	t.Cleanup(gw.Close)
	resp, err := http.Post(gw.URL+"/v1/moderations", "application/json",
		strings.NewReader(`{"model":"google/gemini","input":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
}

func TestModerationsUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		fmt.Fprint(w, `{"error":{"message":"boom"}}`)
	}))
	gw, col := newTestGateway(t, upstream)
	resp, err := http.Post(gw.URL+"/v1/moderations", "application/json",
		strings.NewReader(`{"model":"up/m","input":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if col.one(t).Status != hooks.StatusUpstreamError {
		t.Fatal("want upstream_error")
	}
}
