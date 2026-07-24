package proxy

import (
	"strings"

	"github.com/inja-online/llm-gateway/config"
)

// claudeOAuthResult holds body transforms applied for Anthropic subscription OAuth.
type claudeOAuthResult struct {
	Body       []byte
	ReverseMap map[string]string // tool name restore for responses
	Applied    bool
}

// prepareClaudeOAuthBody applies tool rename + cloaking for Claude subscription OAuth.
// cloakMode: auto|always|never (default auto).
// accessToken is used to detect sk-ant-oat tokens when credentials id is not set.
func prepareClaudeOAuthBody(body []byte, userAgent, cloakMode, accessToken string) claudeOAuthResult {
	out := claudeOAuthResult{Body: body}
	oauth := isClaudeOAuthToken(accessToken)
	if !oauth {
		// Still rename tools if body looks like third-party agent on anthropic oauth path
		// only when we know it's oauth token.
		return out
	}

	// Tool rename always for OAuth (even Claude Code may send mixed names from tools).
	var reverse map[string]string
	body, reverse = remapOAuthToolNames(body)
	out.ReverseMap = reverse

	if shouldCloakClaude(cloakMode, userAgent) {
		body = injectClaudeCodeSystem(body, true)
		body = injectFakeUserID(body)
	} else {
		// Real Claude Code: still ensure cch is signed if billing header present.
		body = signAnthropicMessagesBody(body)
	}

	out.Body = body
	out.Applied = true
	return out
}

// providerUsesClaudeSubscription is true for anthropic + oauth.credentials claude.
func providerUsesClaudeSubscription(p config.Provider) bool {
	return subscriptionCredentialsID(p) == "claude" ||
		(p.Kind == config.KindAnthropic && p.AuthMode() == config.AuthOAuth2 && subscriptionCredentialsID(p) != "")
}

// restoreClaudeOAuthResponse rewrites tool names in a full JSON response body.
func restoreClaudeOAuthResponse(body []byte, reverse map[string]string) []byte {
	return reverseRemapOAuthToolNames(body, reverse)
}

// restoreClaudeOAuthStreamLine rewrites tool names in one SSE line.
func restoreClaudeOAuthStreamLine(line []byte, reverse map[string]string) []byte {
	return reverseRemapOAuthToolNamesFromStreamLine(line, reverse)
}

// cloakModeFromConfig reads optional providers.*.oauth.cloak or global defaults.
// Falls back to "auto".
func cloakModeFromProvider(p config.Provider) string {
	if p.OAuth != nil {
		// Extra map or future field: use Extra["cloak"] if present
		if p.OAuth.Extra != nil {
			if m := strings.TrimSpace(p.OAuth.Extra["cloak"]); m != "" {
				return m
			}
			if m := strings.TrimSpace(p.OAuth.Extra["cloak_mode"]); m != "" {
				return m
			}
		}
	}
	return "auto"
}
