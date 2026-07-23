package subauth

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// StoreTokenSource refreshes subscription OAuth tokens from the credential store.
// Safe for concurrent use; refreshes under a mutex and rewrites the store.
type StoreTokenSource struct {
	Path     string
	Provider string
	// Skew is subtracted from expiry when deciding to refresh (default 30s).
	Skew time.Duration

	mu sync.Mutex
}

// Token implements proxy.TokenSource.
func (s *StoreTokenSource) Token(ctx context.Context) (string, error) {
	tok, _, err := s.TokenWithExpiry(ctx)
	return tok, err
}

// TokenWithExpiry returns a valid access token, refreshing if needed.
func (s *StoreTokenSource) TokenWithExpiry(ctx context.Context) (string, time.Time, error) {
	if s == nil || s.Path == "" || s.Provider == "" {
		return "", time.Time{}, fmt.Errorf("subauth: incomplete StoreTokenSource")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	store, err := Load(s.Path)
	if err != nil {
		return "", time.Time{}, err
	}
	c, ok := store.Get(s.Provider)
	if !ok {
		return "", time.Time{}, fmt.Errorf("subauth: no credentials for %s (run: llm-gateway auth login %s)", s.Provider, s.Provider)
	}

	skew := s.Skew
	if skew <= 0 {
		skew = defaultSkew
	}
	// Usable access token?
	if c.AccessToken != "" && (c.Expiry.IsZero() || time.Now().Add(skew).Before(c.Expiry)) {
		// Claude setup-token often has no refresh; still usable until Expiry.
		return c.AccessToken, c.Expiry, nil
	}
	if c.RefreshToken == "" {
		return "", time.Time{}, fmt.Errorf("subauth: %s access token expired and no refresh_token — re-login", s.Provider)
	}
	tokenURL := c.TokenURL
	clientID := c.ClientID
	if tokenURL == "" || clientID == "" {
		tokenURL, clientID = defaultsForProvider(s.Provider)
	}
	if tokenURL == "" || clientID == "" {
		return "", time.Time{}, fmt.Errorf("subauth: %s missing token_url/client_id for refresh", s.Provider)
	}
	fresh, err := RefreshAccessToken(ctx, nil, tokenURL, clientID, c.RefreshToken)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("subauth: refresh %s: %w", s.Provider, err)
	}
	c.AccessToken = fresh.AccessToken
	if fresh.RefreshToken != "" {
		c.RefreshToken = fresh.RefreshToken
	}
	c.Expiry = fresh.Expiry
	c.TokenURL = tokenURL
	c.ClientID = clientID
	c.Provider = s.Provider
	store.Put(c)
	if err := store.Save(s.Path); err != nil {
		return "", time.Time{}, err
	}
	return c.AccessToken, c.Expiry, nil
}

func defaultsForProvider(p string) (tokenURL, clientID string) {
	switch p {
	case ProviderChatGPT:
		return ChatGPTTokenURL, ChatGPTClientID
	case ProviderGrok:
		return GrokTokenURL, GrokClientID
	default:
		return "", ""
	}
}
