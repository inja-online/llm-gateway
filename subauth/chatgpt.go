package subauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// OpenAI Codex / ChatGPT subscription OAuth constants (public Codex CLI values).
// Source: github.com/openai/codex codex-rs/login (CLIENT_ID, issuer, scopes).
const (
	ChatGPTClientID   = "app_EMoamEEZ73f0CkXaXp7hrann"
	ChatGPTIssuer     = "https://auth.openai.com"
	ChatGPTTokenURL   = ChatGPTIssuer + "/oauth/token"
	ChatGPTScope      = "openid profile email offline_access api.connectors.read api.connectors.invoke"
	ChatGPTDefaultPort = 1455
)

// LoginChatGPT runs the browser PKCE flow and returns a Credential.
// If noBrowser is true, prints the URL and waits for the callback only.
func LoginChatGPT(ctx context.Context, noBrowser bool) (Credential, error) {
	pkce, err := GeneratePKCE()
	if err != nil {
		return Credential{}, err
	}
	state, err := RandomState()
	if err != nil {
		return Credential{}, err
	}

	ln, port, err := listenLoopback(ChatGPTDefaultPort)
	if err != nil {
		return Credential{}, err
	}
	defer ln.Close()

	redirectURI := fmt.Sprintf("http://localhost:%d/auth/callback", port)
	authURL := buildChatGPTAuthorizeURL(ChatGPTClientID, redirectURI, pkce.Challenge, state)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/callback" {
			http.NotFound(w, r)
			return
		}
		if e := r.URL.Query().Get("error"); e != "" {
			// Do not echo error_description.
			errCh <- fmt.Errorf("oauth denied: %s", e)
			fmt.Fprint(w, "Login failed. You can close this tab.")
			return
		}
		if r.URL.Query().Get("state") != state {
			errCh <- fmt.Errorf("oauth state mismatch")
			fmt.Fprint(w, "Login failed (state). You can close this tab.")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("oauth missing code")
			fmt.Fprint(w, "Login failed (no code). You can close this tab.")
			return
		}
		codeCh <- code
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><title>Inja gateway</title>
<p>ChatGPT login successful. You can close this tab and return to the terminal.</p>`)
	})}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		shctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shctx)
	}()

	fmt.Fprintf(os.Stderr, "Open this URL to sign in with ChatGPT (subscription):\n\n  %s\n\n", authURL)
	if !noBrowser {
		if err := OpenBrowser(authURL); err != nil {
			fmt.Fprintf(os.Stderr, "(could not open browser automatically: %v)\n", err)
		}
	}
	fmt.Fprintln(os.Stderr, "Waiting for browser callback on", redirectURI, "…")

	var code string
	select {
	case <-ctx.Done():
		return Credential{}, ctx.Err()
	case err := <-errCh:
		return Credential{}, err
	case code = <-codeCh:
	case <-time.After(10 * time.Minute):
		return Credential{}, fmt.Errorf("oauth timeout waiting for callback")
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", ChatGPTClientID)
	form.Set("code_verifier", pkce.Verifier)

	tr, err := postFormToken(ctx, nil, ChatGPTTokenURL, form)
	if err != nil {
		return Credential{}, err
	}
	c := Credential{
		Provider:     ProviderChatGPT,
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		TokenType:    tr.TokenType,
		ClientID:     ChatGPTClientID,
		TokenURL:     ChatGPTTokenURL,
		Expiry:       expiryFrom(tr),
		Source:       "oauth_pkce",
	}
	if c.RefreshToken == "" {
		return Credential{}, fmt.Errorf("chatgpt oauth: no refresh_token (need offline_access)")
	}
	return c, nil
}

func buildChatGPTAuthorizeURL(clientID, redirectURI, challenge, state string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", ChatGPTScope)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("id_token_add_organizations", "true")
	q.Set("codex_cli_simplified_flow", "true")
	q.Set("state", state)
	return ChatGPTIssuer + "/oauth/authorize?" + q.Encode()
}

func listenLoopback(preferred int) (net.Listener, int, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", preferred))
	if err == nil {
		return ln, preferred, nil
	}
	ln, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, 0, fmt.Errorf("listen: %w", err)
	}
	return ln, ln.Addr().(*net.TCPAddr).Port, nil
}

// ImportChatGPTFromCodexCLI loads tokens from ~/.codex/auth.json (after `codex login`).
func ImportChatGPTFromCodexCLI() (Credential, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Credential{}, err
	}
	path := filepath.Join(home, ".codex", "auth.json")
	if p := os.Getenv("CODEX_HOME"); p != "" {
		path = filepath.Join(p, "auth.json")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Credential{}, fmt.Errorf("read %s: %w (run: codex login)", path, err)
	}
	// Flexible shape: tokens may be nested under tokens / auth / flat.
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return Credential{}, err
	}
	access, refresh := pickCodexTokens(root, raw)
	if access == "" && refresh == "" {
		return Credential{}, fmt.Errorf("codex auth.json: no access/refresh token found in %s", path)
	}
	return Credential{
		Provider:     ProviderChatGPT,
		AccessToken:  access,
		RefreshToken: refresh,
		ClientID:     ChatGPTClientID,
		TokenURL:     ChatGPTTokenURL,
		Source:       "import_codex_cli",
	}, nil
}

func pickCodexTokens(root map[string]json.RawMessage, raw []byte) (access, refresh string) {
	// Flat fields
	var flat struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Tokens       *struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		} `json:"tokens"`
	}
	_ = json.Unmarshal(raw, &flat)
	if flat.AccessToken != "" || flat.RefreshToken != "" {
		return flat.AccessToken, flat.RefreshToken
	}
	if flat.Tokens != nil {
		return flat.Tokens.AccessToken, flat.Tokens.RefreshToken
	}
	// Nested "OPENAI_API_KEY" style stores sometimes use different keys.
	type tok struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
	}
	for _, key := range []string{"tokens", "auth", "chatgpt", "openai"} {
		if v, ok := root[key]; ok {
			var t tok
			if json.Unmarshal(v, &t) == nil && (t.Access != "" || t.Refresh != "") {
				return t.Access, t.Refresh
			}
		}
	}
	return "", ""
}

