package subauth

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// chatgptAuthClaim is the custom OpenAI claim used on Codex/ChatGPT tokens.
// Nested under "https://api.openai.com/auth" (Codex / ChatGPT OAuth tokens).
type chatgptAuthClaim struct {
	ChatGPTAccountID string `json:"chatgpt_account_id"`
}

// ParseAccountIDFromJWT extracts ChatGPT account id from an access or id JWT.
// Signature is not verified — tokens are already obtained from OpenAI's
// token endpoint; we only need the account id claim for upstream headers.
func ParseAccountIDFromJWT(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Some encoders include padding.
		raw, err = base64.URLEncoding.DecodeString(padBase64(parts[1]))
		if err != nil {
			return ""
		}
	}
	var claims struct {
		Auth chatgptAuthClaim `json:"https://api.openai.com/auth"`
		// Fallbacks seen in some token shapes.
		ChatGPTAccountID string `json:"chatgpt_account_id"`
		AccountID        string `json:"account_id"`
	}
	if err := json.Unmarshal(raw, &claims); err != nil {
		return ""
	}
	if id := strings.TrimSpace(claims.Auth.ChatGPTAccountID); id != "" {
		return id
	}
	if id := strings.TrimSpace(claims.ChatGPTAccountID); id != "" {
		return id
	}
	return strings.TrimSpace(claims.AccountID)
}

// AccountIDFromTokens prefers id_token then access_token JWT claims.
func AccountIDFromTokens(accessToken, idToken string) string {
	if id := ParseAccountIDFromJWT(idToken); id != "" {
		return id
	}
	return ParseAccountIDFromJWT(accessToken)
}

func padBase64(s string) string {
	switch len(s) % 4 {
	case 2:
		return s + "=="
	case 3:
		return s + "="
	default:
		return s
	}
}
