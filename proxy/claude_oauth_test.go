package proxy

import (
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/subauth"
	"github.com/tidwall/gjson"
)

func TestRemapOAuthToolNames(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-5",
		"tools":[{"name":"bash","description":"run"},{"name":"Read","description":"read"}],
		"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]
	}`)
	out, rev := remapOAuthToolNames(body)
	if gjson.GetBytes(out, "tools.0.name").String() != "Bash" {
		t.Fatalf("bash→Bash got %s", gjson.GetBytes(out, "tools.0.name").String())
	}
	// Client already sent Read — no reverse entry required for it if unchanged
	if rev["Bash"] != "bash" {
		t.Fatalf("reverse map Bash: %v", rev)
	}
	// Restore
	resp := []byte(`{"content":[{"type":"tool_use","name":"Bash","id":"1"}]}`)
	restored := reverseRemapOAuthToolNames(resp, rev)
	if gjson.GetBytes(restored, "content.0.name").String() != "bash" {
		t.Fatalf("restore got %s", gjson.GetBytes(restored, "content.0.name").String())
	}
}

func TestPrepareClaudeOAuthBody_CloakAndTools(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-5",
		"system":"Be helpful",
		"tools":[{"name":"grep","description":"search"}],
		"messages":[{"role":"user","content":"find foo"}]
	}`)
	res := prepareClaudeOAuthBody(body, "curl/8.0", "auto", "sk-ant-oat01-test")
	if !res.Applied {
		t.Fatal("expected oauth transforms")
	}
	sys0 := gjson.GetBytes(res.Body, "system.0.text").String()
	if !strings.HasPrefix(sys0, "x-anthropic-billing-header:") {
		t.Fatalf("missing billing header: %s", sys0)
	}
	if !strings.Contains(sys0, "cch=") {
		t.Fatalf("missing cch: %s", sys0)
	}
	// cch must not be placeholder after signing
	if strings.Contains(sys0, "cch=00000") {
		t.Fatalf("cch still placeholder: %s", sys0)
	}
	if gjson.GetBytes(res.Body, "tools.0.name").String() != "Grep" {
		t.Fatalf("tool rename: %s", gjson.GetBytes(res.Body, "tools.0.name").String())
	}
	uid := gjson.GetBytes(res.Body, "metadata.user_id").String()
	if !isValidUserID(uid) {
		t.Fatalf("user_id invalid: %q", uid)
	}
	// Claude Code UA → no full cloak (system not replaced), but tools still renamed
	res2 := prepareClaudeOAuthBody(body, "claude-cli/2.1.63", "auto", "sk-ant-oat01-test")
	if gjson.GetBytes(res2.Body, "tools.0.name").String() != "Grep" {
		t.Fatal("tool rename should still run for claude-cli")
	}
	// Original system preserved when not cloaking
	if gjson.GetBytes(res2.Body, "system").String() != "Be helpful" &&
		gjson.GetBytes(res2.Body, "system.0.text").String() == "" {
		// system may stay string "Be helpful"
		sys := gjson.GetBytes(res2.Body, "system")
		if sys.Type.String() == "String" && sys.String() != "Be helpful" {
			t.Fatalf("claude-cli should keep system: %s", sys.Raw)
		}
	}
}

func TestSignAnthropicMessagesBody(t *testing.T) {
	body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.63.abc; cc_entrypoint=cli; cch=00000;"}],"messages":[]}`)
	signed := signAnthropicMessagesBody(body)
	txt := gjson.GetBytes(signed, "system.0.text").String()
	if strings.Contains(txt, "cch=00000") {
		t.Fatalf("expected signed cch, got %s", txt)
	}
}

func TestShouldCloakClaude(t *testing.T) {
	if !shouldCloakClaude("auto", "curl/8") {
		t.Fatal("auto+curl should cloak")
	}
	if shouldCloakClaude("auto", "claude-cli/2.0") {
		t.Fatal("auto+claude-cli should not cloak")
	}
	if shouldCloakClaude("never", "curl/8") {
		t.Fatal("never should not cloak")
	}
	if !shouldCloakClaude("always", "claude-cli/2.0") {
		t.Fatal("always should cloak")
	}
}

func TestRestoreStreamLineAndResponse(t *testing.T) {
	rev := map[string]string{"Bash": "bash", "Grep": "grep"}
	line := []byte(`data: {"type":"content_block_start","content_block":{"type":"tool_use","name":"Bash","id":"1"}}` + "\n")
	out := reverseRemapOAuthToolNamesFromStreamLine(line, rev)
	if !strings.Contains(string(out), `"name":"bash"`) {
		t.Fatalf("stream restore: %s", out)
	}
	// bare JSON
	bare := reverseRemapOAuthToolNamesFromStreamLine(
		[]byte(`{"content_block":{"type":"tool_reference","tool_name":"Grep"}}`), rev)
	if !strings.Contains(string(bare), `"tool_name":"grep"`) {
		t.Fatalf("bare restore: %s", bare)
	}
	resp := restoreClaudeOAuthResponse(
		[]byte(`{"content":[{"type":"tool_use","name":"Bash"}]}`), rev)
	if gjson.GetBytes(resp, "content.0.name").String() != "bash" {
		t.Fatal(string(resp))
	}
}

func TestCloakModeFromProvider(t *testing.T) {
	p := config.Provider{OAuth: &config.OAuthConfig{Extra: map[string]string{"cloak": "never"}}}
	if cloakModeFromProvider(p) != "never" {
		t.Fatal(cloakModeFromProvider(p))
	}
	if cloakModeFromProvider(config.Provider{}) != "auto" {
		t.Fatal("default auto")
	}
}

func TestPrependToFirstUserMessage(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)
	out := prependToFirstUserMessage(body, "sys")
	c := gjson.GetBytes(out, "messages.0.content")
	if c.IsArray() {
		if !strings.Contains(c.Array()[0].Get("text").String(), "sys") {
			t.Fatal(c.Raw)
		}
	} else if !strings.Contains(c.String(), "sys") {
		t.Fatal(c.Raw)
	}
}

func TestMapRemoteCatalog(t *testing.T) {
	raw := remoteCatalogJSON{
		"claude":    {{ID: "claude-sonnet-5"}, {ID: "claude-sonnet-5"}},
		"codex-pro": {{ID: "gpt-5.6-sol"}},
		"xai":       {{ID: "grok-4.5"}},
	}
	m := mapRemoteCatalog(raw)
	if len(m[subauth.ProviderClaude]) != 1 {
		t.Fatalf("claude dedupe: %v", m[subauth.ProviderClaude])
	}
	if m[subauth.ProviderChatGPT][0] != "gpt-5.6-sol" {
		t.Fatal(m[subauth.ProviderChatGPT])
	}
	if m[subauth.ProviderGrok][0] != "grok-4.5" {
		t.Fatal(m[subauth.ProviderGrok])
	}
}

func TestModelsCatalogURLsEnv(t *testing.T) {
	t.Setenv("INJA_GATEWAY_MODELS_URL", "https://example.com/a.json, https://example.com/b.json")
	u := modelsCatalogURLs()
	if len(u) != 2 || u[0] != "https://example.com/a.json" {
		t.Fatalf("%v", u)
	}
}

