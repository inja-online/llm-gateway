package subauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// xAI SuperGrok / X Premium+ OAuth (device code). Public client id used by
// Grok CLI / OpenClaw (shared OIDC client). Discovery: auth.x.ai OIDC.
const (
	GrokClientID  = "b1a00492-073a-47ea-816f-4c329264a828"
	GrokIssuer    = "https://auth.x.ai"
	GrokDiscovery = GrokIssuer + "/.well-known/openid-configuration"
	GrokScope     = "openid profile email offline_access grok-cli:access api:access"
	// Discovered default (auth.x.ai uses /oauth2/* paths).
	GrokTokenURL = GrokIssuer + "/oauth2/token"
	// Default Grok CLI credential file (macOS/Linux).
	GrokCLIAuthRel = ".grok/auth.json"
)

const grokHTTPUserAgent = "inja-gateway/subauth"

type oidcDiscovery struct {
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
	TokenEndpoint               string `json:"token_endpoint"`
	AuthorizationEndpoint       string `json:"authorization_endpoint"`
}

// LoginGrokOptions controls LoginGrok behavior.
type LoginGrokOptions struct {
	// ForceDevice skips auto-import from ~/.grok/auth.json and always runs device code.
	ForceDevice bool
}

// LoginGrok runs the OAuth 2.0 device-code flow against xAI.
// Prefer `ImportGrokFromCLI` when ~/.grok/auth.json already exists (Grok CLI).
func LoginGrok(ctx context.Context, opts ...LoginGrokOptions) (Credential, error) {
	var o LoginGrokOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	// If the official Grok CLI already logged in, reuse it — device pages often
	// show "Invalid action" when the session/account state is messy.
	if !o.ForceDevice {
		if c, err := ImportGrokFromCLI(); err == nil && c.AccessToken != "" {
			fmt.Fprintln(os.Stderr, "Found existing Grok CLI login (~/.grok/auth.json) — importing it.")
			fmt.Fprintln(os.Stderr, "(Force device flow: llm-gateway auth login grok --device)")
			c.Source = "import_grok_cli"
			return c, nil
		}
	}

	disc, err := fetchGrokDiscovery(ctx)
	if err != nil {
		return Credential{}, err
	}
	form := url.Values{}
	form.Set("client_id", GrokClientID)
	form.Set("scope", GrokScope)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, disc.DeviceAuthorizationEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Credential{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", grokHTTPUserAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Credential{}, fmt.Errorf("device code request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Credential{}, err
	}
	var dc struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		ExpiresIn               int    `json:"expires_in"`
		Interval                int    `json:"interval"`
		Error                   string `json:"error"`
		ErrorDesc               string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &dc); err != nil {
		return Credential{}, fmt.Errorf("device code parse (HTTP %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || dc.DeviceCode == "" {
		msg := dc.Error
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return Credential{}, fmt.Errorf("device code: %s", msg)
	}

	// Prefer the *base* verification URI + manual code entry. Prefilling
	// ?user_code=… often yields "Invalid action" on accounts.x.ai when the
	// browser session is wrong or the code was already consumed.
	baseURI := strings.TrimSpace(dc.VerificationURI)
	if baseURI == "" {
		baseURI = "https://accounts.x.ai/oauth2/device"
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Sign in with SuperGrok / X Premium+ (xAI device code)")
	fmt.Fprintln(os.Stderr, "────────────────────────────────────────────────────")
	fmt.Fprintln(os.Stderr, "If the browser says \"Invalid action\":")
	fmt.Fprintln(os.Stderr, "  • Prefer:  llm-gateway auth import grok   (uses ~/.grok/auth.json from Grok CLI)")
	fmt.Fprintln(os.Stderr, "  • Or open the base URL (not the prefilled link), sign in, then type the code.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "  1. Open:  %s\n", baseURI)
	fmt.Fprintf(os.Stderr, "  2. Enter code:  %s\n", dc.UserCode)
	if dc.VerificationURIComplete != "" {
		fmt.Fprintf(os.Stderr, "  (alt prefilled link — often breaks): %s\n", dc.VerificationURIComplete)
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Waiting for approval…")

	// Open base URI (not complete) — more reliable on accounts.x.ai.
	_ = OpenBrowser(baseURI)

	interval := time.Duration(dc.Interval) * time.Second
	if interval < time.Second {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)
	if dc.ExpiresIn <= 0 {
		deadline = time.Now().Add(15 * time.Minute)
	}

	for {
		if time.Now().After(deadline) {
			return Credential{}, fmt.Errorf("device code expired — try: llm-gateway auth import grok  (or login again)")
		}
		select {
		case <-ctx.Done():
			return Credential{}, ctx.Err()
		case <-time.After(interval):
		}

		tform := url.Values{}
		tform.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		tform.Set("device_code", dc.DeviceCode)
		tform.Set("client_id", GrokClientID)
		tr, err := postFormTokenUA(ctx, nil, disc.TokenEndpoint, tform, grokHTTPUserAgent)
		if err != nil {
			msg := err.Error()
			if strings.Contains(msg, "authorization_pending") || strings.Contains(msg, "slow_down") {
				if strings.Contains(msg, "slow_down") {
					interval += 5 * time.Second
				}
				continue
			}
			if strings.Contains(msg, "access_denied") || strings.Contains(msg, "expired_token") {
				return Credential{}, fmt.Errorf("%w — try: llm-gateway auth import grok", err)
			}
			return Credential{}, err
		}
		c := Credential{
			Provider:     ProviderGrok,
			AccessToken:  tr.AccessToken,
			RefreshToken: tr.RefreshToken,
			TokenType:    tr.TokenType,
			ClientID:     GrokClientID,
			TokenURL:     disc.TokenEndpoint,
			Expiry:       expiryFrom(tr),
			Source:       "device_code",
		}
		if c.RefreshToken == "" {
			fmt.Fprintln(os.Stderr, "warning: no refresh_token; access token may expire without re-login")
		}
		return c, nil
	}
}

// ImportGrokFromCLI loads OAuth tokens from the official Grok CLI store
// (~/.grok/auth.json), then Hermes / OpenClaw locations if present.
func ImportGrokFromCLI() (Credential, error) {
	paths := grokImportCandidates()
	var lastErr error
	for _, path := range paths {
		c, err := importGrokAuthFile(path)
		if err == nil {
			c.Source = "import_grok_cli"
			return c, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no Grok credential file found (tried ~/.grok/auth.json)")
	}
	return Credential{}, fmt.Errorf("%w\n  tip: install/login Grok CLI, or paste via device login", lastErr)
}

func grokImportCandidates() []string {
	var out []string
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out,
			filepath.Join(home, GrokCLIAuthRel),
			filepath.Join(home, ".hermes", "auth.json"),
			filepath.Join(home, ".openclaw", "credentials", "oauth.json"),
		)
		// OpenClaw per-agent auth profiles (best-effort scan of default agent only).
		out = append(out, filepath.Join(home, ".openclaw", "agents", "main", "agent", "auth-profiles.json"))
		out = append(out, filepath.Join(home, ".openclaw", "agents", "default", "agent", "auth-profiles.json"))
	}
	if p := os.Getenv("GROK_AUTH_FILE"); p != "" {
		out = append([]string{p}, out...)
	}
	return out
}

func importGrokAuthFile(path string) (Credential, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Credential{}, err
	}
	// Shape 1: Grok CLI map keyed by "https://auth.x.ai::<client_id>"
	var grokMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &grokMap); err == nil {
		for k, v := range grokMap {
			if !strings.Contains(k, "auth.x.ai") && !strings.Contains(k, GrokClientID) && !strings.Contains(strings.ToLower(k), "xai") {
				// still try parse if single entry
				if len(grokMap) != 1 {
					continue
				}
			}
			c, err := parseGrokCLIEntry(v)
			if err == nil && (c.AccessToken != "" || c.RefreshToken != "") {
				c.Provider = ProviderGrok
				if c.ClientID == "" {
					c.ClientID = GrokClientID
				}
				if c.TokenURL == "" {
					c.TokenURL = GrokTokenURL
				}
				return c, nil
			}
		}
	}

	// Shape 2: flat / hermes-style { access_token, refresh_token } or nested providers
	var flat struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Key          string `json:"key"`
		ExpiresAt    string `json:"expires_at"`
		// hermes often nests under provider ids
		XAIOauth json.RawMessage `json:"xai-oauth"`
		XAI      json.RawMessage `json:"xai"`
		Grok     json.RawMessage `json:"grok"`
	}
	if err := json.Unmarshal(raw, &flat); err == nil {
		if flat.AccessToken != "" || flat.RefreshToken != "" || flat.Key != "" {
			c := Credential{
				Provider:     ProviderGrok,
				AccessToken:  firstNonEmpty(flat.AccessToken, flat.Key),
				RefreshToken: flat.RefreshToken,
				ClientID:     GrokClientID,
				TokenURL:     GrokTokenURL,
			}
			if t, err := time.Parse(time.RFC3339Nano, flat.ExpiresAt); err == nil {
				c.Expiry = t
			} else if t, err := time.Parse(time.RFC3339, flat.ExpiresAt); err == nil {
				c.Expiry = t
			}
			return c, nil
		}
		for _, nest := range []json.RawMessage{flat.XAIOauth, flat.XAI, flat.Grok} {
			if len(nest) == 0 {
				continue
			}
			if c, err := parseGrokCLIEntry(nest); err == nil && (c.AccessToken != "" || c.RefreshToken != "") {
				c.Provider = ProviderGrok
				if c.ClientID == "" {
					c.ClientID = GrokClientID
				}
				if c.TokenURL == "" {
					c.TokenURL = GrokTokenURL
				}
				return c, nil
			}
		}
	}

	// Shape 3: openclaw auth-profiles { profiles: { "xai:default": { type, access, refresh, … } } }
	var profiles struct {
		Profiles map[string]struct {
			Type     string `json:"type"`
			Provider string `json:"provider"`
			Access   string `json:"access"`
			Refresh  string `json:"refresh"`
			Expires  int64  `json:"expires"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(raw, &profiles); err == nil && profiles.Profiles != nil {
		for id, p := range profiles.Profiles {
			if !strings.Contains(strings.ToLower(id), "xai") &&
				!strings.Contains(strings.ToLower(p.Provider), "xai") &&
				!strings.Contains(strings.ToLower(p.Provider), "grok") {
				continue
			}
			if p.Access == "" && p.Refresh == "" {
				continue
			}
			c := Credential{
				Provider:     ProviderGrok,
				AccessToken:  p.Access,
				RefreshToken: p.Refresh,
				ClientID:     GrokClientID,
				TokenURL:     GrokTokenURL,
			}
			if p.Expires > 0 {
				// ms or seconds
				if p.Expires > 1e12 {
					c.Expiry = time.UnixMilli(p.Expires)
				} else {
					c.Expiry = time.Unix(p.Expires, 0)
				}
			}
			return c, nil
		}
	}

	return Credential{}, fmt.Errorf("unrecognized Grok auth format in %s", path)
}

func parseGrokCLIEntry(raw json.RawMessage) (Credential, error) {
	var e struct {
		Key          string `json:"key"` // access token in Grok CLI
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresAt    string `json:"expires_at"`
		OIDCIssuer   string `json:"oidc_issuer"`
		OIDCClientID string `json:"oidc_client_id"`
		AuthMode     string `json:"auth_mode"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return Credential{}, err
	}
	access := firstNonEmpty(e.AccessToken, e.Key)
	if access == "" && e.RefreshToken == "" {
		return Credential{}, fmt.Errorf("empty tokens")
	}
	c := Credential{
		AccessToken:  access,
		RefreshToken: e.RefreshToken,
		ClientID:     firstNonEmpty(e.OIDCClientID, GrokClientID),
		TokenURL:     GrokTokenURL,
	}
	if e.OIDCIssuer != "" && strings.Contains(e.OIDCIssuer, "x.ai") {
		// keep default token URL from discovery shape
	}
	if t, err := time.Parse(time.RFC3339Nano, e.ExpiresAt); err == nil {
		c.Expiry = t
	} else if t, err := time.Parse(time.RFC3339, e.ExpiresAt); err == nil {
		c.Expiry = t
	}
	return c, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func fetchGrokDiscovery(ctx context.Context) (oidcDiscovery, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, GrokDiscovery, nil)
	if err != nil {
		return oidcDiscovery{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", grokHTTPUserAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return oidcDiscovery{
			DeviceAuthorizationEndpoint: GrokIssuer + "/oauth2/device/code",
			TokenEndpoint:               GrokTokenURL,
			AuthorizationEndpoint:       GrokIssuer + "/oauth2/authorize",
		}, nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return oidcDiscovery{}, err
	}
	var d oidcDiscovery
	if err := json.Unmarshal(body, &d); err != nil {
		return oidcDiscovery{}, err
	}
	if d.TokenEndpoint == "" {
		d.TokenEndpoint = GrokTokenURL
	}
	if d.DeviceAuthorizationEndpoint == "" {
		return oidcDiscovery{}, fmt.Errorf("xAI discovery: missing device_authorization_endpoint")
	}
	if !trustedXAIURL(d.TokenEndpoint) || !trustedXAIURL(d.DeviceAuthorizationEndpoint) {
		return oidcDiscovery{}, fmt.Errorf("xAI discovery: untrusted endpoint host")
	}
	return d, nil
}

func trustedXAIURL(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme != "https" {
		return false
	}
	h := parsed.Hostname()
	return h == "x.ai" || h == "auth.x.ai" || h == "accounts.x.ai" || strings.HasSuffix(h, ".x.ai")
}

// postFormTokenUA is like postFormToken but sets a User-Agent (xAI is picky).
func postFormTokenUA(ctx context.Context, client *http.Client, tokenURL string, form url.Values, ua string) (tokenResponse, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	resp, err := client.Do(req)
	if err != nil {
		return tokenResponse{}, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return tokenResponse{}, err
	}
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return tokenResponse{}, fmt.Errorf("token parse (HTTP %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || tr.AccessToken == "" {
		msg := strings.TrimSpace(tr.Error)
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return tokenResponse{}, fmt.Errorf("token endpoint: %s", msg)
	}
	return tr, nil
}
