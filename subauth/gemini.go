package subauth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Google consumer OAuth used by Gemini CLI / Antigravity (Jetski) token files.
// Client secret is never committed; refresh reads INJA_GATEWAY_GEMINI_CLIENT_SECRET
// or a secret stored on the credential (local 0600 store only).
const (
	GeminiClientID = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"
	GeminiTokenURL = "https://oauth2.googleapis.com/token"
	geminiCLIRel   = ".gemini/jetski-standalone-oauth-token"
)

// ImportGeminiFromCLI loads Antigravity / Gemini CLI consumer OAuth.
// Override path with GEMINI_AUTH_FILE.
func ImportGeminiFromCLI() (Credential, error) {
	path, err := geminiAuthPath()
	if err != nil {
		return Credential{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Credential{}, fmt.Errorf("read %s: %w (Gemini/Antigravity login, or: llm-gateway auth login gemini)", path, err)
	}
	c, err := parseGeminiAuthFile(raw)
	if err != nil {
		return Credential{}, err
	}
	c.Provider = ProviderGemini
	c.Source = "import_gemini_cli"
	c.ClientID = GeminiClientID
	c.TokenURL = GeminiTokenURL
	if sec := strings.TrimSpace(os.Getenv("INJA_GATEWAY_GEMINI_CLIENT_SECRET")); sec != "" {
		c.ClientSecret = sec
	}
	return c, nil
}

func geminiAuthPath() (string, error) {
	if p := strings.TrimSpace(os.Getenv("GEMINI_AUTH_FILE")); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, geminiCLIRel), nil
}

func parseGeminiAuthFile(raw []byte) (Credential, error) {
	var root struct {
		AuthMethod string `json:"auth_method"`
		Token      *struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			TokenType    string `json:"token_type"`
			Expiry       string `json:"expiry"`
		} `json:"token"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		Expiry       string `json:"expiry"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		return Credential{}, err
	}
	c := Credential{TokenType: "Bearer"}
	if root.Token != nil && root.Token.AccessToken != "" {
		c.AccessToken = root.Token.AccessToken
		c.RefreshToken = root.Token.RefreshToken
		if root.Token.TokenType != "" {
			c.TokenType = root.Token.TokenType
		}
		c.Expiry = parseGeminiExpiry(root.Token.Expiry)
	} else if root.AccessToken != "" {
		c.AccessToken = root.AccessToken
		c.RefreshToken = root.RefreshToken
		if root.TokenType != "" {
			c.TokenType = root.TokenType
		}
		c.Expiry = parseGeminiExpiry(root.Expiry)
	} else {
		return Credential{}, fmt.Errorf("gemini credentials: no access token")
	}
	return c, nil
}

func parseGeminiExpiry(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// LoginGemini reuses a local Gemini/Antigravity token file (no extra browser flow).
func LoginGemini() (Credential, error) {
	c, err := ImportGeminiFromCLI()
	if err != nil {
		return Credential{}, err
	}
	fmt.Fprintln(os.Stderr, "Imported Gemini/Antigravity consumer OAuth.")
	return c, nil
}
