package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks/ring"
	"github.com/inja-online/llm-gateway/hooks/sqlite"
)

func TestMountOperator(t *testing.T) {
	proxyH := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		http.NotFound(w, r)
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
		if !bytes.Contains(rec.Body.Bytes(), []byte("Dashboard")) {
			t.Fatalf("GET /ui/ body %s", rec.Body.Bytes())
		}
	} else {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ui/", nil))
		if rec.Code == http.StatusOK && bytes.Contains(rec.Body.Bytes(), []byte("Dashboard")) {
			t.Fatal("noweb served /ui")
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/dashboard/meta", nil)
	req.Header.Set("Authorization", "Bearer secret")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET meta status %d body %s", rec.Code, rec.Body.Bytes())
	}
	var meta struct {
		Dashboard bool `json:"dashboard"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &meta); err != nil {
		t.Fatal(err)
	}
	if !meta.Dashboard {
		t.Fatalf("meta %s", rec.Body.Bytes())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("healthz status %d body %s", rec.Code, rec.Body.Bytes())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, "/v1/dashboard/meta", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS status %d body %s", rec.Code, rec.Body.Bytes())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/meta", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth GET meta status %d", rec.Code)
	}
}

func TestBuildHandlerDisabled(t *testing.T) {
	off := false
	cfg := &config.Config{
		Providers: map[string]config.Provider{
			"up": {Kind: config.KindOpenAICompat, BaseURL: "https://example.com/v1"},
		},
		Dashboard: config.Dashboard{Enabled: &off},
	}
	h, err := buildHandler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/meta", nil))
	if rec.Code == http.StatusOK {
		t.Fatalf("disabled dashboard served meta: %s", rec.Body.Bytes())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ui/", nil))
	if rec.Code == http.StatusOK && bytes.Contains(rec.Body.Bytes(), []byte("Dashboard")) {
		t.Fatal("disabled dashboard served /ui")
	}
}
