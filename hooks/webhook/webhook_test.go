package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/inja-online/llm-gateway/hooks"
)

func TestPostsJSONAsync(t *testing.T) {
	var mu sync.Mutex
	var got hooks.UsageEvent
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		if r.Method != http.MethodPost {
			t.Errorf("method %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type %s", ct)
		}
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		json.Unmarshal(body, &got)
		mu.Unlock()
		w.WriteHeader(204)
	}))
	t.Cleanup(srv.Close)

	s := New(srv.URL, time.Second)
	s.OnUsage(context.Background(), hooks.UsageEvent{
		RequestID: "req_1",
		Status:    hooks.StatusOK,
		TokensIn:  3,
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("webhook not received")
	}
	mu.Lock()
	defer mu.Unlock()
	if got.RequestID != "req_1" || got.TokensIn != 3 {
		t.Fatalf("%+v", got)
	}
}

func TestDefaultTimeout(t *testing.T) {
	s := New("http://example.invalid", 0)
	if s.client.Timeout != defaultTimeout {
		t.Fatalf("timeout %v", s.client.Timeout)
	}
}

func TestNon2xxLoggedNotPanic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	t.Cleanup(srv.Close)
	s := New(srv.URL, time.Second)
	s.OnUsage(context.Background(), hooks.UsageEvent{RequestID: "x"})
	time.Sleep(100 * time.Millisecond) // allow goroutine
}

func TestPostNetworkError(t *testing.T) {
	s := New("http://127.0.0.1:1", 50*time.Millisecond)
	s.OnUsage(context.Background(), hooks.UsageEvent{RequestID: "net"})
	time.Sleep(150 * time.Millisecond)
}

func TestPostStatusAndBadURL(t *testing.T) {
	// status >= 300 branch
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	t.Cleanup(srv.Close)
	s := New(srv.URL, time.Second)
	s.OnUsage(context.Background(), hooks.UsageEvent{Model: "m"})
	time.Sleep(50 * time.Millisecond)

	// bad URL build error path
	s2 := New("http://[::1]:namedbad", time.Millisecond)
	s2.post([]byte(`{}`))

	// network error
	s3 := New("http://127.0.0.1:1", 50*time.Millisecond)
	s3.post([]byte(`{}`))
}
