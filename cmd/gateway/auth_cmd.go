package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/inja-online/llm-gateway/subauth"
)

// runAuth dispatches: llm-gateway auth <login|status|logout|env|import> …
func runAuth(args []string) error {
	if len(args) == 0 {
		printAuthUsage()
		return fmt.Errorf("auth: missing subcommand")
	}
	switch args[0] {
	case "login":
		return authLogin(args[1:])
	case "status":
		return authStatus()
	case "logout":
		return authLogout(args[1:])
	case "env":
		return authEnv(args[1:])
	case "import":
		return authImport(args[1:])
	case "help", "-h", "--help":
		printAuthUsage()
		return nil
	default:
		printAuthUsage()
		return fmt.Errorf("auth: unknown subcommand %q", args[0])
	}
}

func printAuthUsage() {
	fmt.Fprintf(os.Stderr, `Usage: llm-gateway auth <command>

Subscription OAuth helpers (ChatGPT Plus/Pro via Codex, Claude, SuperGrok).
Tokens are stored in $INJA_GATEWAY_AUTH_FILE or ~/.config/inja-gateway/credentials.json (0600).

Commands:
  login chatgpt [--no-browser]   Browser PKCE login (ChatGPT subscription / Codex OAuth)
  login claude                   setup-token / paste Claude subscription OAuth token
  login grok [--device]          Import ~/.grok/auth.json if present, else device-code
  import chatgpt                 Import from ~/.codex/auth.json after: codex login
  import claude                  Import from Claude Code credentials file
  import grok                    Import from ~/.grok/auth.json (Grok CLI) / Hermes / OpenClaw
  status                         Show which providers are logged in (no secrets)
  logout [provider|all]          Remove stored credentials
  env [provider]                 Print export lines (refresh/access) for debugging

Wire into gateway.yaml with:
  auth: oauth2
  oauth:
    credentials: chatgpt   # or claude | grok

See docs/claude-code-multi.md and docs/oauth-token-sources.md.
`)
}

func authPath() (string, error) {
	return subauth.DefaultPath()
}

func authLogin(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("auth login: need provider (chatgpt|claude|grok)")
	}
	provider := strings.ToLower(args[0])
	noBrowser := false
	forceDevice := false
	for _, a := range args[1:] {
		switch a {
		case "--no-browser":
			noBrowser = true
		case "--device":
			forceDevice = true
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	var (
		cred subauth.Credential
		err  error
	)
	switch provider {
	case subauth.ProviderChatGPT, "openai", "codex":
		provider = subauth.ProviderChatGPT
		cred, err = subauth.LoginChatGPT(ctx, noBrowser)
	case subauth.ProviderClaude, "anthropic":
		provider = subauth.ProviderClaude
		cred, err = subauth.LoginClaudeSetupToken()
	case subauth.ProviderGrok, "xai":
		provider = subauth.ProviderGrok
		cred, err = subauth.LoginGrok(ctx, subauth.LoginGrokOptions{ForceDevice: forceDevice})
	default:
		return fmt.Errorf("auth login: unknown provider %q (chatgpt|claude|grok)", args[0])
	}
	if err != nil {
		return err
	}
	cred.Provider = provider
	return saveCred(cred)
}

func authImport(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("auth import: need provider (chatgpt|claude|grok)")
	}
	var (
		cred subauth.Credential
		err  error
	)
	switch strings.ToLower(args[0]) {
	case subauth.ProviderChatGPT, "openai", "codex":
		cred, err = subauth.ImportChatGPTFromCodexCLI()
		cred.Provider = subauth.ProviderChatGPT
	case subauth.ProviderClaude, "anthropic":
		cred, err = subauth.ImportClaudeFromCLI()
		cred.Provider = subauth.ProviderClaude
	case subauth.ProviderGrok, "xai":
		cred, err = subauth.ImportGrokFromCLI()
		cred.Provider = subauth.ProviderGrok
	default:
		return fmt.Errorf("auth import: unknown provider %q (chatgpt|claude|grok)", args[0])
	}
	if err != nil {
		return err
	}
	return saveCred(cred)
}

func saveCred(cred subauth.Credential) error {
	path, err := authPath()
	if err != nil {
		return err
	}
	store, err := subauth.Load(path)
	if err != nil {
		return err
	}
	store.Put(cred)
	if err := store.Save(path); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Saved %s credentials → %s\n", cred.Provider, path)
	fmt.Fprintf(os.Stderr, "  source=%s  has_refresh=%v  expiry=%s\n",
		cred.Source, cred.RefreshToken != "", formatExpiry(cred.Expiry))
	return nil
}

func authStatus() error {
	path, err := authPath()
	if err != nil {
		return err
	}
	store, err := subauth.Load(path)
	if err != nil {
		return err
	}
	fmt.Println("auth file:", path)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROVIDER\tSOURCE\tREFRESH\tEXPIRY\tACCESS")
	for _, id := range subauth.ValidProviders() {
		c, ok := store.Get(id)
		if !ok {
			fmt.Fprintf(w, "%s\t-\t-\t-\tmissing\n", id)
			continue
		}
		acc := "present"
		if c.AccessToken == "" {
			acc = "empty"
		} else if !c.Expiry.IsZero() && time.Now().After(c.Expiry) {
			acc = "expired"
		}
		ref := "no"
		if c.RefreshToken != "" {
			ref = "yes"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", id, c.Source, ref, formatExpiry(c.Expiry), acc)
	}
	return w.Flush()
}

func authLogout(args []string) error {
	path, err := authPath()
	if err != nil {
		return err
	}
	store, err := subauth.Load(path)
	if err != nil {
		return err
	}
	target := "all"
	if len(args) > 0 {
		target = strings.ToLower(args[0])
	}
	if target == "all" {
		for _, id := range subauth.ValidProviders() {
			store.Delete(id)
		}
	} else {
		if !subauth.IsKnownProvider(target) {
			return fmt.Errorf("logout: unknown provider %q", target)
		}
		store.Delete(target)
	}
	if err := store.Save(path); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Logged out %s (%s)\n", target, path)
	return nil
}

func authEnv(args []string) error {
	path, err := authPath()
	if err != nil {
		return err
	}
	store, err := subauth.Load(path)
	if err != nil {
		return err
	}
	ids := subauth.ValidProviders()
	if len(args) > 0 {
		ids = []string{strings.ToLower(args[0])}
	}
	fmt.Printf("# from %s — treat as secrets\n", path)
	for _, id := range ids {
		c, ok := store.Get(id)
		if !ok {
			fmt.Printf("# %s: not logged in\n", id)
			continue
		}
		prefix := strings.ToUpper(id)
		if c.AccessToken != "" {
			fmt.Printf("export %s_ACCESS_TOKEN=%q\n", prefix, c.AccessToken)
		}
		if c.RefreshToken != "" {
			fmt.Printf("export %s_REFRESH_TOKEN=%q\n", prefix, c.RefreshToken)
		}
	}
	return nil
}

func formatExpiry(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.Local().Format(time.RFC3339)
}
