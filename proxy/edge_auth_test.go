package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
)

func edgeAuthGateway(t *testing.T, keys []string, keysEnv string, envVal string, upstream http.HandlerFunc) *httptest.Server {
	t.Helper()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if upstream != nil {
			upstream(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"c","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	t.Cleanup(up.Close)

	yaml := fmt.Sprintf(`
providers:
  up: { kind: openai_compat, base_url: %q, api_key_env: UPSTREAM_KEY }
defaults:
  openai_dialect: up
edge_auth:
  enabled: true
  keys: [%s]
  keys_env: %q
`, up.URL, joinYAMLStrings(keys), keysEnv)
	if keysEnv != "" && envVal != "" {
		t.Setenv(keysEnv, envVal)
	}
	t.Setenv("UPSTREAM_KEY", "sk-upstream-secret")
	cfg, err := config.Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)
	return gw
}

func joinYAMLStrings(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%q", k)
	}
	return strings.Join(parts, ", ")
}

func TestEdgeAuthDisabledByDefault(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":"c","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	t.Cleanup(up.Close)
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  up: { kind: openai_compat, base_url: %q }
defaults:
  openai_dialect: up
`, up.URL)))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	// No client credentials → still reaches handler (may fail upstream auth later).
	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		t.Fatalf("edge auth should be off by default, got 401")
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d %s", resp.StatusCode, body)
	}
}

func TestEdgeAuthRejectsMissingAndWrongKey(t *testing.T) {
	gw := edgeAuthGateway(t, []string{"edge-secret"}, "", "", nil)

	// missing
	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("missing: %d %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "invalid_edge_auth") {
		t.Fatalf("body: %s", body)
	}

	// wrong
	req, _ := http.NewRequest("POST", gw.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer wrong-key")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("wrong key: %d", resp.StatusCode)
	}
}

func TestEdgeAuthAcceptsBearerAndXAPIKey(t *testing.T) {
	var sawUpstreamAuth string
	gw := edgeAuthGateway(t, []string{"edge-secret"}, "", "", func(w http.ResponseWriter, r *http.Request) {
		sawUpstreamAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"c","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	})

	for _, setAuth := range []func(*http.Request){
		func(r *http.Request) { r.Header.Set("Authorization", "Bearer edge-secret") },
		func(r *http.Request) { r.Header.Set("x-api-key", "edge-secret") },
	} {
		sawUpstreamAuth = ""
		req, _ := http.NewRequest("POST", gw.URL+"/v1/chat/completions",
			strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
		req.Header.Set("Content-Type", "application/json")
		setAuth(req)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("status %d %s", resp.StatusCode, body)
		}
		// api_key_env replaces edge key for upstream
		if sawUpstreamAuth != "Bearer sk-upstream-secret" {
			t.Fatalf("upstream auth = %q", sawUpstreamAuth)
		}
	}
}

func TestEdgeAuthHealthzOpen(t *testing.T) {
	gw := edgeAuthGateway(t, []string{"edge-secret"}, "", "", nil)
	resp, err := http.Get(gw.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("healthz should be open: %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"status":"ok"}` {
		t.Fatalf("%s", body)
	}
}

func TestEdgeAuthKeysEnv(t *testing.T) {
	gw := edgeAuthGateway(t, nil, "GATEWAY_EDGE_KEYS", "env-key-a, env-key-b", nil)
	req, _ := http.NewRequest("POST", gw.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer env-key-b")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d", resp.StatusCode)
	}
}

func TestEdgeKeyOKConstantTime(t *testing.T) {
	keys := []string{"abc", "longer-secret-value"}
	if !edgeKeyOK("abc", keys) {
		t.Fatal("expected match")
	}
	if edgeKeyOK("ab", keys) {
		t.Fatal("length mismatch must fail")
	}
	if edgeKeyOK("xyz", keys) {
		t.Fatal("wrong value")
	}
	if edgeKeyOK("", keys) {
		t.Fatal("empty")
	}
}

func TestConfigEdgeAuthValidation(t *testing.T) {
	if _, err := config.Parse([]byte(`
providers:
  x: { kind: openai, base_url: "https://x" }
edge_auth:
  enabled: true
`)); err == nil {
		t.Fatal("expected error when enabled without keys")
	}

	t.Setenv("EDGE_TEST_KEYS", "k1")
	cfg, err := config.Parse([]byte(`
providers:
  x: { kind: openai, base_url: "https://x" }
edge_auth:
  enabled: true
  keys_env: EDGE_TEST_KEYS
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.EdgeKeys(); len(got) != 1 || got[0] != "k1" {
		t.Fatalf("%v", got)
	}
}
