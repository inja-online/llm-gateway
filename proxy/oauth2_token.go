package proxy

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/inja-online/llm-gateway/config"
)

// DefaultOAuthSkew is subtracted from expires_in when caching so tokens are
// refreshed slightly before absolute expiry.
const DefaultOAuthSkew = 30 * time.Second

// tokenWithExpiry is implemented by sources that know absolute expiry.
// CachingTokenSource uses it when available.
type tokenWithExpiry interface {
	TokenWithExpiry(ctx context.Context) (token string, expiry time.Time, err error)
}

// OAuth2TokenSource obtains access tokens via RFC 6749 form POST to TokenURL.
// Supports client_credentials and refresh_token grants. Stdlib only (no x/oauth2).
type OAuth2TokenSource struct {
	TokenURL     string
	GrantType    string // client_credentials | refresh_token
	ClientID     string
	ClientSecret string
	RefreshToken string
	Scopes       []string
	Audience     string
	Extra        map[string]string
	HTTPClient   *http.Client // optional; default http.DefaultClient
	Skew         time.Duration
}

// NewOAuth2TokenSource builds a TokenSource from provider OAuth config.
// Values are resolved from env at construction time (and re-read for refresh
// tokens only if you rebuild — secrets are snapshotted for the process).
func NewOAuth2TokenSource(o *config.OAuthConfig) (*OAuth2TokenSource, error) {
	if o == nil {
		return nil, fmt.Errorf("oauth2: nil config")
	}
	grant, err := o.EffectiveGrant()
	if err != nil {
		return nil, err
	}
	ts := &OAuth2TokenSource{
		TokenURL:     strings.TrimSpace(o.TokenURL),
		GrantType:    grant,
		ClientID:     o.ResolvedClientID(),
		ClientSecret: o.ResolvedClientSecret(),
		RefreshToken: o.ResolvedRefreshToken(),
		Scopes:       append([]string(nil), o.Scopes...),
		Audience:     strings.TrimSpace(o.Audience),
		Extra:        copyStringMap(o.Extra),
		Skew:         DefaultOAuthSkew,
	}
	if ts.TokenURL == "" {
		return nil, fmt.Errorf("oauth2: token_url required")
	}
	switch grant {
	case config.OAuthGrantClientCredentials:
		if ts.ClientID == "" || ts.ClientSecret == "" {
			return nil, fmt.Errorf("oauth2: client_credentials requires client id and secret (check env)")
		}
	case config.OAuthGrantRefreshToken:
		if ts.RefreshToken == "" {
			return nil, fmt.Errorf("oauth2: refresh_token grant requires refresh token (check env)")
		}
	}
	return ts, nil
}

func (o *OAuth2TokenSource) Token(ctx context.Context) (string, error) {
	tok, _, err := o.TokenWithExpiry(ctx)
	return tok, err
}

func (o *OAuth2TokenSource) TokenWithExpiry(ctx context.Context) (string, time.Time, error) {
	if o == nil {
		return "", time.Time{}, fmt.Errorf("oauth2: nil source")
	}
	form := url.Values{}
	form.Set("grant_type", o.GrantType)
	switch o.GrantType {
	case config.OAuthGrantClientCredentials:
		form.Set("client_id", o.ClientID)
		form.Set("client_secret", o.ClientSecret)
		if len(o.Scopes) > 0 {
			form.Set("scope", strings.Join(o.Scopes, " "))
		}
	case config.OAuthGrantRefreshToken:
		form.Set("refresh_token", o.RefreshToken)
		if o.ClientID != "" {
			form.Set("client_id", o.ClientID)
		}
		if o.ClientSecret != "" {
			form.Set("client_secret", o.ClientSecret)
		}
		if len(o.Scopes) > 0 {
			form.Set("scope", strings.Join(o.Scopes, " "))
		}
	default:
		return "", time.Time{}, fmt.Errorf("oauth2: unsupported grant %q", o.GrantType)
	}
	if o.Audience != "" {
		form.Set("audience", o.Audience)
	}
	for k, v := range o.Extra {
		if k == "" {
			continue
		}
		form.Set(k, v)
	}
	return fetchOAuthToken(ctx, o.httpClient(), o.TokenURL, form, o.skew())
}

func (o *OAuth2TokenSource) httpClient() *http.Client {
	if o != nil && o.HTTPClient != nil {
		return o.HTTPClient
	}
	return http.DefaultClient
}

func (o *OAuth2TokenSource) skew() time.Duration {
	if o != nil && o.Skew > 0 {
		return o.Skew
	}
	return DefaultOAuthSkew
}

// oauthTokenResponse is the subset of RFC 6749 token JSON we need.
type oauthTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

func fetchOAuthToken(ctx context.Context, client *http.Client, tokenURL string, form url.Values, skew time.Duration) (string, time.Time, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("oauth2: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("oauth2: token request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("oauth2: read token response: %w", err)
	}
	var tr oauthTokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", time.Time{}, fmt.Errorf("oauth2: parse token response (HTTP %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || tr.AccessToken == "" {
		// Only surface the stable error code — never error_description or raw
		// body (token endpoints sometimes echo client secrets).
		msg := strings.TrimSpace(tr.Error)
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return "", time.Time{}, fmt.Errorf("oauth2: token endpoint error: %s", msg)
	}
	expiry := time.Time{}
	if tr.ExpiresIn > 0 {
		d := time.Duration(tr.ExpiresIn) * time.Second
		if skew > 0 && d > skew {
			d -= skew
		}
		expiry = time.Now().Add(d)
	}
	return tr.AccessToken, expiry, nil
}

// ServiceAccountJWTSource exchanges a Google-style service-account JWT for an
// access token (urn:ietf:params:oauth:grant-type:jwt-bearer). Stdlib only.
type ServiceAccountJWTSource struct {
	Email      string
	PrivateKey *rsa.PrivateKey
	TokenURL   string
	Scopes     []string
	HTTPClient *http.Client
	Skew       time.Duration
	// KeyID is optional kid header claim.
	KeyID string
}

// googleSAFile is the subset of GCP service-account JSON we need.
type googleSAFile struct {
	Type        string `json:"type"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	PrivateKeyID string `json:"private_key_id"`
	TokenURI    string `json:"token_uri"`
}

// DefaultGoogleTokenURL is used when the SA JSON omits token_uri.
const DefaultGoogleTokenURL = "https://oauth2.googleapis.com/token"

// DefaultGoogleSAScopes is used when no scopes are configured.
var DefaultGoogleSAScopes = []string{"https://www.googleapis.com/auth/cloud-platform"}

// NewServiceAccountJWTSourceFromFile loads a GCP service-account JSON key file.
func NewServiceAccountJWTSourceFromFile(path string, scopes []string) (*ServiceAccountJWTSource, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("service_account: read %s: %w", path, err)
	}
	return NewServiceAccountJWTSourceFromJSON(raw, scopes)
}

// NewServiceAccountJWTSourceFromJSON parses SA JSON bytes.
func NewServiceAccountJWTSourceFromJSON(raw []byte, scopes []string) (*ServiceAccountJWTSource, error) {
	var f googleSAFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("service_account: parse json: %w", err)
	}
	if f.ClientEmail == "" || f.PrivateKey == "" {
		return nil, fmt.Errorf("service_account: client_email and private_key required")
	}
	key, err := parseRSAPrivateKey(f.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("service_account: private_key: %w", err)
	}
	tokenURL := strings.TrimSpace(f.TokenURI)
	if tokenURL == "" {
		tokenURL = DefaultGoogleTokenURL
	}
	if len(scopes) == 0 {
		scopes = append([]string(nil), DefaultGoogleSAScopes...)
	}
	return &ServiceAccountJWTSource{
		Email:      f.ClientEmail,
		PrivateKey: key,
		TokenURL:   tokenURL,
		Scopes:     scopes,
		KeyID:      f.PrivateKeyID,
		Skew:       DefaultOAuthSkew,
	}, nil
}

func (s *ServiceAccountJWTSource) Token(ctx context.Context) (string, error) {
	tok, _, err := s.TokenWithExpiry(ctx)
	return tok, err
}

func (s *ServiceAccountJWTSource) TokenWithExpiry(ctx context.Context) (string, time.Time, error) {
	if s == nil || s.PrivateKey == nil || s.Email == "" {
		return "", time.Time{}, fmt.Errorf("service_account: incomplete source")
	}
	tokenURL := s.TokenURL
	if tokenURL == "" {
		tokenURL = DefaultGoogleTokenURL
	}
	assertion, err := s.signedJWT(tokenURL)
	if err != nil {
		return "", time.Time{}, err
	}
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)
	return fetchOAuthToken(ctx, s.httpClient(), tokenURL, form, s.skew())
}

func (s *ServiceAccountJWTSource) httpClient() *http.Client {
	if s != nil && s.HTTPClient != nil {
		return s.HTTPClient
	}
	return http.DefaultClient
}

func (s *ServiceAccountJWTSource) skew() time.Duration {
	if s != nil && s.Skew > 0 {
		return s.Skew
	}
	return DefaultOAuthSkew
}

func (s *ServiceAccountJWTSource) signedJWT(audience string) (string, error) {
	now := time.Now()
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	if s.KeyID != "" {
		header["kid"] = s.KeyID
	}
	claims := map[string]any{
		"iss":   s.Email,
		"sub":   s.Email,
		"aud":   audience,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
		"scope": strings.Join(s.Scopes, " "),
	}
	hb, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	cb, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding
	signingInput := enc.EncodeToString(hb) + "." + enc.EncodeToString(cb)
	sum := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.PrivateKey, crypto.SHA256, sum[:])
	if err != nil {
		return "", fmt.Errorf("service_account: sign jwt: %w", err)
	}
	return signingInput + "." + enc.EncodeToString(sig), nil
}

func parseRSAPrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("no PEM block")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rk, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("not RSA private key")
		}
		return rk, nil
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

func copyStringMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// tokenSourceFromProvider builds a TokenSource from provider auth config, or
// nil when the mode does not auto-wire (api_key / bearer / client_bearer, or
// adc/service_account without a credential file).
func tokenSourceFromProvider(p config.Provider) (TokenSource, error) {
	switch p.AuthMode() {
	case config.AuthOAuth2:
		inner, err := NewOAuth2TokenSource(p.OAuth)
		if err != nil {
			return nil, err
		}
		return &CachingTokenSource{Inner: inner}, nil
	case config.AuthServiceAccount, config.AuthADC:
		path := strings.TrimSpace(p.ServiceAccountFile)
		if path == "" {
			// Optional: standard Google ADC file env (binary convenience).
			path = strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))
		}
		if path == "" {
			return nil, nil
		}
		inner, err := NewServiceAccountJWTSourceFromFile(path, nil)
		if err != nil {
			return nil, err
		}
		return &CachingTokenSource{Inner: inner}, nil
	default:
		return nil, nil
	}
}
