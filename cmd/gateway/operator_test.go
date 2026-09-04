package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks/ring"
	"github.com/inja-online/llm-gateway/hooks/sqlite"
)

func TestMountOperator(t *testing.T) {
	proxyH := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	cfg := &config.Config{
		Dashboard: config.Dashboard{CORSOrigin: "https://spa.example"},
		EdgeAuth:  config.EdgeAuth{Enabled: true, Keys: []string{"secret"}},
	}
	sqlSink, err := sqlite.New("")
	if err != nil {
		t.Fatal(err)
	}
	h := mountOperator(cfg, proxyH, ring.New(8), sqlSink)

	if uiEnabled {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ui/", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /ui/ status %d body %s", rec.Code, rec.Body.Bytes())
		}
		if !bytes.Contains(bytes.ToLower(rec.Body.Bytes()), []byte("dashboard")) {
			t.Fatalf("GET /ui/ body %s", rec.Body.Bytes())
		}
	} else {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ui/", nil))
		if rec.Code == http.StatusOK && bytes.Contains(bytes.ToLower(rec.Body.Bytes()), []byte("dashboard")) {
			t.Fatal("noweb served /ui")
		}
	}

	// Unauthenticated operator API -> 401
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/meta", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /v1/dashboard/meta unauth: want 401, got %d", rec.Code)
	}

	// Authenticated operator API -> 200 + CORS headers
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/dashboard/meta", nil)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Origin", "https://spa.example")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/dashboard/meta auth: want 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://spa.example" {
		t.Errorf("CORS origin: want https://spa.example, got %q", got)
	}

	// Non-operator, non-ui paths fall through to proxy handler
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	if rec.Code != http.StatusTeapot {
		t.Fatalf("proxy fallthrough: want 418, got %d", rec.Code)
	}
}
