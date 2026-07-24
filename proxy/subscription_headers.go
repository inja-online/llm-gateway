package proxy

import (
	"net/http"
	"strings"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/subauth"
)

// Subscription header defaults for consumer OAuth (Claude Code + Codex TUI).
// Updated 2026-07-24.
const (
	// claudeOAuthBeta is required for Anthropic consumer OAuth tokens.
	claudeOAuthBeta = "oauth-2025-04-20"

	// claudeDefaultBetas matches Claude Code OAuth traffic.
	claudeDefaultBetas = "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14," +
		"context-management-2025-06-27,prompt-caching-scope-2026-01-05,structured-outputs-2025-12-15," +
		"fast-mode-2026-02-01,redact-thinking-2026-02-12,token-efficient-tools-2026-03-28"

	codexDefaultUserAgent = "codex-tui/0.135.0 (Mac OS 26.5.0; arm64) iTerm.app/3.6.10 (codex-tui; 0.135.0)"
	codexOriginator       = "codex-tui"
)

// applySubscriptionHeaders injects provider-specific headers for consumer
// subscription OAuth (oauth.credentials). Called after applyAuth so Bearer is set.
// No-op when the provider is not a subscription credentials mode.
func applySubscriptionHeaders(up *http.Request, client *http.Request, p config.Provider) {
	if up == nil || p.OAuth == nil {
		return
	}
	cred := strings.ToLower(strings.TrimSpace(p.OAuth.Credentials))
	if cred == "" {
		return
	}
	switch cred {
	case subauth.ProviderClaude:
		applyClaudeSubscriptionHeaders(up, client)
	case subauth.ProviderChatGPT:
		applyChatGPTSubscriptionHeaders(up, client)
	case subauth.ProviderGrok:
		// api.x.ai accepts plain Bearer; no special headers required today.
	}
}

func applyClaudeSubscriptionHeaders(up, client *http.Request) {
	// OAuth always uses Bearer (applyAuth already set it). Ensure no x-api-key.
	up.Header.Del("x-api-key")

	if up.Header.Get("anthropic-version") == "" {
		// Prefer client-forwarded value via copyForwardHeaders; fill default if still empty.
		if client != nil {
			if v := client.Header.Get("anthropic-version"); v != "" {
				up.Header.Set("anthropic-version", v)
			}
		}
		if up.Header.Get("anthropic-version") == "" {
			up.Header.Set("anthropic-version", "2023-06-01")
		}
	}

	// Merge anthropic-beta: client value wins as base; always ensure oauth beta.
	base := ""
	if client != nil {
		base = strings.TrimSpace(client.Header.Get("anthropic-beta"))
	}
	if existing := strings.TrimSpace(up.Header.Get("anthropic-beta")); existing != "" && base == "" {
		base = existing
	}
	up.Header.Set("anthropic-beta", mergeAnthropicBetas(base, claudeDefaultBetas))

	if up.Header.Get("X-App") == "" {
		if client != nil && client.Header.Get("X-App") != "" {
			up.Header.Set("X-App", client.Header.Get("X-App"))
		} else {
			up.Header.Set("X-App", "cli")
		}
	}
}

// mergeAnthropicBetas returns a comma-separated beta list. If clientBase is
// empty, defaults are used. oauth-2025-04-20 is always present.
func mergeAnthropicBetas(clientBase, defaults string) string {
	base := strings.TrimSpace(clientBase)
	if base == "" {
		base = defaults
	}
	if !betaListContains(base, claudeOAuthBeta) {
		if base == "" {
			return claudeOAuthBeta
		}
		return base + "," + claudeOAuthBeta
	}
	return base
}

func betaListContains(list, want string) bool {
	want = strings.TrimSpace(want)
	for _, p := range strings.Split(list, ",") {
		if strings.TrimSpace(p) == want {
			return true
		}
	}
	return false
}

func applyChatGPTSubscriptionHeaders(up, client *http.Request) {
	// Codex backend is picky about User-Agent / Originator (Cloudflare).
	if up.Header.Get("User-Agent") == "" {
		ua := ""
		if client != nil {
			ua = client.Header.Get("User-Agent")
		}
		// Prefer real Codex client UA when present; otherwise spoof CLI default.
		if strings.Contains(strings.ToLower(ua), "codex") {
			up.Header.Set("User-Agent", ua)
		} else {
			up.Header.Set("User-Agent", codexDefaultUserAgent)
		}
	}
	if up.Header.Get("Originator") == "" {
		if client != nil && client.Header.Get("Originator") != "" {
			up.Header.Set("Originator", client.Header.Get("Originator"))
		} else {
			up.Header.Set("Originator", codexOriginator)
		}
	}
	if up.Header.Get("Chatgpt-Account-Id") == "" && up.Header.Get("ChatGPT-Account-Id") == "" {
		if id := loadChatGPTAccountID(); id != "" {
			up.Header.Set("Chatgpt-Account-Id", id)
		}
	}
}

func loadChatGPTAccountID() string {
	path, err := subauth.ResolvePath()
	if err != nil {
		return ""
	}
	c, ok := subauth.LoadCredential(path, subauth.ProviderChatGPT)
	if !ok {
		return ""
	}
	if id := strings.TrimSpace(c.AccountID); id != "" {
		return id
	}
	// Best-effort from live access token if store never got AccountID.
	return subauth.ParseAccountIDFromJWT(c.AccessToken)
}

// subscriptionCredentialsID returns oauth.credentials when set.
func subscriptionCredentialsID(p config.Provider) string {
	if p.OAuth == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(p.OAuth.Credentials))
}
