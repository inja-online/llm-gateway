package proxy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/subauth"
)

// TokenSource supplies OAuth2-style access tokens for providers using auth:
// adc, service_account, or oauth2 (and optionally bearer with a server-held token).
// Real Google ADC is optional: inject a source via Server.SetTokenSource, set
// service_account_file / GOOGLE_APPLICATION_CREDENTIALS, or use auth: oauth2.
// The gateway does not pull in cloud SDKs by default.
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
//
// When Inner implements tokenWithExpiry, the real expires_in-derived expiry is
// used; otherwise TTL (default 5m) applies.
type CachingTokenSource struct {
	Inner TokenSource
	TTL   time.Duration // default 5m when zero and inner has no expiry

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
	tok, exp, err := fetchToken(ctx, c.Inner, c.TTL)
	if err != nil {
		return "", err
	}
	c.token = tok
	c.expiry = exp
	return tok, nil
}

// Invalidate clears the cache so the next Token() call refreshes (401 retry).
func (c *CachingTokenSource) Invalidate() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.token = ""
	c.expiry = time.Time{}
	c.mu.Unlock()
}

func fetchToken(ctx context.Context, inner TokenSource, ttl time.Duration) (string, time.Time, error) {
	if inner == nil {
		return "", time.Time{}, fmt.Errorf("token source: nil inner")
	}
	if ex, ok := inner.(tokenWithExpiry); ok {
		tok, exp, err := ex.TokenWithExpiry(ctx)
		if err != nil {
			return "", time.Time{}, err
		}
		if exp.IsZero() {
			if ttl <= 0 {
				ttl = 5 * time.Minute
			}
			exp = time.Now().Add(ttl)
		}
		return tok, exp, nil
	}
	tok, err := inner.Token(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return tok, time.Now().Add(ttl), nil
}

// SetTokenSource registers a TokenSource for a provider name (e.g. "vertex").
// Used when provider.auth is adc, service_account, or oauth2. Overrides any
// auto-wired source from config.
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

// cooldownSubscriptionAccount marks the last-used store account for cooldown (429).
func (s *Server) cooldownSubscriptionAccount(providerName string, p config.Provider) {
	if s == nil {
		return
	}
	ts := s.tokenSource(providerName)
	if ts == nil {
		return
	}
	if c, ok := ts.(*CachingTokenSource); ok && c.Inner != nil {
		ts = c.Inner
	}
	if st, ok := ts.(*subauth.StoreTokenSource); ok {
		st.CooldownLast(5 * time.Minute)
	}
	_ = p
}

// autoWireTokenSources constructs TokenSources from provider config (oauth2,
// service_account_file, GOOGLE_APPLICATION_CREDENTIALS). Errors are deferred
// to request time via a FuncTokenSource so a single bad provider does not
// prevent process start (misconfig still fails closed on first use).
func (s *Server) autoWireTokenSources() {
	if s == nil || s.cfg == nil {
		return
	}
	for name, p := range s.cfg.Providers {
		if s.tokenSource(name) != nil {
			continue
		}
		ts, err := tokenSourceFromProvider(p)
		if err != nil {
			// Capture err for Token() so operators get a clear message.
			msg := err.Error()
			s.SetTokenSource(name, FuncTokenSource(func(context.Context) (string, error) {
				return "", fmt.Errorf("auto token source %q: %s", name, msg)
			}))
			continue
		}
		if ts != nil {
			s.SetTokenSource(name, ts)
		}
	}
}

// needsTokenSource reports whether resolveUpstreamKey should use TokenSource.
func needsTokenSource(p config.Provider) bool {
	return p.UsesTokenSource()
}
