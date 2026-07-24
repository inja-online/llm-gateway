package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/subauth"
)

func TestMergeAnthropicBetas(t *testing.T) {
	got := mergeAnthropicBetas("", claudeDefaultBetas)
	if !strings.Contains(got, claudeOAuthBeta) {
		t.Fatalf("defaults missing oauth beta: %s", got)
	}
	got = mergeAnthropicBetas("interleaved-thinking-2025-05-14", claudeDefaultBetas)
	if !strings.Contains(got, claudeOAuthBeta) {
		t.Fatalf("client base should still get oauth: %s", got)
	}
	if !strings.Contains(got, "interleaved-thinking-2025-05-14") {
		t.Fatalf("client beta lost: %s", got)
	}
	// Already present: no duplicate.
	got = mergeAnthropicBetas("a,"+claudeOAuthBeta+",b", claudeDefaultBetas)
	if strings.Count(got, claudeOAuthBeta) != 1 {
		t.Fatalf("duplicate oauth beta: %s", got)
	}
}

func TestApplyClaudeSubscriptionHeaders(t *testing.T) {
	up := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", nil)
	up.Header.Set("Authorization", "Bearer sk-ant-oat-test")
	client := httptest.NewRequest(http.MethodPost, "http://gw/v1/messages", nil)
	client.Header.Set("anthropic-beta", "skills-2025-10-02")

	applyClaudeSubscriptionHeaders(up, client)

	if up.Header.Get("x-api-key") != "" {
		t.Fatal("oauth must not send x-api-key")
	}
	if up.Header.Get("anthropic-version") != "2023-06-01" {
		t.Fatalf("version=%q", up.Header.Get("anthropic-version"))
	}
	beta := up.Header.Get("anthropic-beta")
	if !strings.Contains(beta, "skills-2025-10-02") {
		t.Fatalf("client beta missing: %s", beta)
	}
	if !strings.Contains(beta, claudeOAuthBeta) {
		t.Fatalf("oauth beta missing: %s", beta)
	}
	if up.Header.Get("X-App") != "cli" {
		t.Fatalf("X-App=%q", up.Header.Get("X-App"))
	}
}

func TestApplyChatGPTSubscriptionHeaders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "creds.json")
	st := &subauth.Store{Version: 1, Credentials: map[string]subauth.Credential{}}
	st.Put(subauth.Credential{
		Provider:    subauth.ProviderChatGPT,
		AccessToken: "tok",
		AccountID:   "acct_xyz",
		Expiry:      time.Now().Add(time.Hour),
	})
	if err := st.Save(path); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INJA_GATEWAY_AUTH_FILE", path)

	up := httptest.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
	up.Header.Set("Authorization", "Bearer tok")
	applyChatGPTSubscriptionHeaders(up, nil)

	if up.Header.Get("User-Agent") != codexDefaultUserAgent {
		t.Fatalf("User-Agent=%q", up.Header.Get("User-Agent"))
	}
	if up.Header.Get("Originator") != codexOriginator {
		t.Fatalf("Originator=%q", up.Header.Get("Originator"))
	}
	if up.Header.Get("Chatgpt-Account-Id") != "acct_xyz" {
		t.Fatalf("account=%q", up.Header.Get("Chatgpt-Account-Id"))
	}
}

func TestSubscriptionHeadersOnUpstream(t *testing.T) {
	// End-to-end: Claude oauth provider request injects betas.
	var gotAuth, gotBeta, gotXAPI string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotBeta = r.Header.Get("anthropic-beta")
		gotXAPI = r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"hi"}],"model":"claude-sonnet-5","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	t.Cleanup(up.Close)

	dir := t.TempDir()
	path := filepath.Join(dir, "creds.json")
	st := &subauth.Store{Version: 1, Credentials: map[string]subauth.Credential{}}
	st.Put(subauth.Credential{
		Provider:    subauth.ProviderClaude,
		AccessToken: "sk-ant-oat-live",
		Expiry:      time.Now().Add(24 * time.Hour),
	})
	if err := st.Save(path); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INJA_GATEWAY_AUTH_FILE", path)

	cfg, err := config.Parse([]byte(`
providers:
  anthropic:
    kind: anthropic
    base_url: "` + up.URL + `"
    auth: oauth2
    oauth:
      credentials: claude
aliases:
  sonnet: anthropic/claude-sonnet-5
`))
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(cfg, nil)
	h := httptest.NewServer(srv.Handler())
	t.Cleanup(h.Close)

	body := `{"model":"sonnet","messages":[{"role":"user","content":"hi"}],"max_tokens":16}`
	resp, err := http.Post(h.URL+"/v1/messages", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, raw)
	}
	if !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Fatalf("Authorization=%q", gotAuth)
	}
	if gotXAPI != "" {
		t.Fatalf("x-api-key should be empty, got %q", gotXAPI)
	}
	if !strings.Contains(gotBeta, claudeOAuthBeta) {
		t.Fatalf("beta=%q missing oauth", gotBeta)
	}
}

func TestApplySubscriptionHeaders_NoOpWithoutCredentials(t *testing.T) {
	up := httptest.NewRequest(http.MethodPost, "https://api.openai.com/v1/chat/completions", nil)
	p := config.Provider{Kind: config.KindOpenAI, Auth: config.AuthAPIKey}
	applySubscriptionHeaders(up, nil, p)
	if up.Header.Get("User-Agent") != "" {
		t.Fatal("should not set UA without subscription credentials")
	}
}
