package utls

import (
	"net/http"
	"testing"
)

func TestHostNeeds(t *testing.T) {
	if !HostNeeds("https://api.anthropic.com/v1") {
		t.Fatal("anthropic")
	}
	if !HostNeeds("https://chatgpt.com/backend-api/codex") {
		t.Fatal("chatgpt")
	}
	if HostNeeds("https://api.openai.com/v1") {
		t.Fatal("openai")
	}
	if HostNeeds("://bad") {
		t.Fatal("bad url")
	}
}

func TestFallbackNonProtected(t *testing.T) {
	called := false
	fb := &Fallback{
		Utls: http.DefaultTransport,
		Fallback: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			called = true
			return &http.Response{StatusCode: 200, Body: http.NoBody, Request: r}, nil
		}),
	}
	req, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	resp, err := fb.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !called {
		t.Fatal("expected fallback")
	}
}

func TestNewSubscriptionClient(t *testing.T) {
	c := NewSubscriptionClient()
	if c == nil || c.Transport == nil {
		t.Fatal("nil client")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
