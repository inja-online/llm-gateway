package proxy

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// TokenSource supplies OAuth2-style access tokens for providers using auth:
// adc or service_account (and optionally bearer with a server-held token).
// Real Google ADC is optional: inject a source via Server.SetTokenSource or
// tests; the gateway does not pull in cloud SDKs by default.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// StaticTokenSource always returns the same token. Useful for tests and for
// short-lived tokens refreshed outside the gateway process.
type StaticTokenSource struct {
	AccessToken string
}

func (s StaticTokenSource) Token(context.Context) (string, error) {
	if s.AccessToken == "" {
		return "", fmt.Errorf("token source: empty access token")
	}
	return s.AccessToken, nil
}

// FuncTokenSource adapts a function to TokenSource.
type FuncTokenSource func(ctx context.Context) (string, error)

func (f FuncTokenSource) Token(ctx context.Context) (string, error) { return f(ctx) }

// CachingTokenSource wraps a source and reuses tokens until Expiry (or a
// fixed TTL when the inner source does not expose expiry). Concurrent callers
// share one in-flight refresh via singleflight-style mutex.
type CachingTokenSource struct {
	Inner TokenSource
	TTL   time.Duration // default 5m when zero

	mu     sync.Mutex
	token  string
	expiry time.Time
}

func (c *CachingTokenSource) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if c.token != "" && now.Before(c.expiry) {
		return c.token, nil
	}
	tok, err := c.Inner.Token(ctx)
	if err != nil {
		return "", err
	}
	ttl := c.TTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	c.token = tok
	c.expiry = now.Add(ttl)
	return tok, nil
}

// SetTokenSource registers a TokenSource for a provider name (e.g. "vertex").
// Used when provider.auth is adc or service_account.
func (s *Server) SetTokenSource(provider string, ts TokenSource) {
	if s.tokenSources == nil {
		s.tokenSources = make(map[string]TokenSource)
	}
	s.tokenSources[provider] = ts
}

func (s *Server) tokenSource(provider string) TokenSource {
	if s.tokenSources == nil {
		return nil
	}
	return s.tokenSources[provider]
}
