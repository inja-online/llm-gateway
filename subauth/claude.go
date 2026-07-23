package subauth

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// LoginClaudeSetupToken runs `claude setup-token` when available, or prompts
// to paste a token from that command / Claude Code login.
//
// Anthropic restricts Free/Pro/Max OAuth to Claude Code and Claude.ai for many
// third-party products. setup-token is the supported long-lived path for
// subscription use in automation; operators must follow Anthropic's current ToS.
func LoginClaudeSetupToken() (Credential, error) {
	fmt.Fprintln(os.Stderr, "Claude subscription auth")
	fmt.Fprintln(os.Stderr, "  Preferred: run `claude setup-token` (opens browser), then paste the token here.")
	fmt.Fprintln(os.Stderr, "  Or import an existing Claude Code login (see import-claude).")
	fmt.Fprintln(os.Stderr)

	if _, err := exec.LookPath("claude"); err == nil {
		fmt.Fprintln(os.Stderr, "Launching: claude setup-token")
		cmd := exec.Command("claude", "setup-token")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		// Non-fatal if the command fails — user can still paste.
		_ = cmd.Run()
		fmt.Fprintln(os.Stderr)
	}

	fmt.Fprint(os.Stderr, "Paste Claude OAuth / setup token (sk-ant-oat… or similar), then Enter:\n> ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return Credential{}, err
	}
	tok := strings.TrimSpace(line)
	if tok == "" {
		return Credential{}, fmt.Errorf("empty token")
	}
	return Credential{
		Provider:    ProviderClaude,
		AccessToken: tok,
		// setup-token is long-lived; refresh is via re-login.
		Expiry: time.Now().Add(365 * 24 * time.Hour),
		Source: "setup_token",
	}, nil
}

// ImportClaudeFromCLI loads OAuth credentials written by Claude Code.
// Linux/Windows: ~/.claude/.credentials.json (or $CLAUDE_CONFIG_DIR).
// macOS Keychain is not read here — use setup-token paste on macOS if needed.
func ImportClaudeFromCLI() (Credential, error) {
	path, err := claudeCredentialsPath()
	if err != nil {
		return Credential{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Credential{}, fmt.Errorf("read %s: %w (run claude /login, or: llm-gateway auth login claude)", path, err)
	}
	var root struct {
		ClaudeAiOauth *struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
			ExpiresAt    int64  `json:"expiresAt"`
		} `json:"claudeAiOauth"`
		// Alternate shapes
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		OAuth        *struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
			ExpiresAt    int64  `json:"expiresAt"`
		} `json:"oauth"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		return Credential{}, err
	}
	c := Credential{Provider: ProviderClaude, Source: "import_claude_cli"}
	switch {
	case root.ClaudeAiOauth != nil && root.ClaudeAiOauth.AccessToken != "":
		c.AccessToken = root.ClaudeAiOauth.AccessToken
		c.RefreshToken = root.ClaudeAiOauth.RefreshToken
		if root.ClaudeAiOauth.ExpiresAt > 0 {
			// Claude stores ms epoch.
			ms := root.ClaudeAiOauth.ExpiresAt
			if ms > 1e12 {
				c.Expiry = time.UnixMilli(ms)
			} else {
				c.Expiry = time.Unix(ms, 0)
			}
		}
	case root.OAuth != nil && root.OAuth.AccessToken != "":
		c.AccessToken = root.OAuth.AccessToken
		c.RefreshToken = root.OAuth.RefreshToken
		if root.OAuth.ExpiresAt > 0 {
			ms := root.OAuth.ExpiresAt
			if ms > 1e12 {
				c.Expiry = time.UnixMilli(ms)
			} else {
				c.Expiry = time.Unix(ms, 0)
			}
		}
	case root.AccessToken != "":
		c.AccessToken = root.AccessToken
		c.RefreshToken = root.RefreshToken
	default:
		return Credential{}, fmt.Errorf("claude credentials: no access token in %s", path)
	}
	return c, nil
}

func claudeCredentialsPath() (string, error) {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, ".credentials.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", ".credentials.json"), nil
}
