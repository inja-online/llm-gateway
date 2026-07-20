package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

func TestOpenAIBatchesCRUDProxy(t *testing.T) {
	var createPath, createAuth, createBody, listPath, getPath, cancelPath string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/batches":
			createPath = r.URL.Path
			createAuth = r.Header.Get("Authorization")
			b, _ := io.ReadAll(r.Body)
			createBody = string(b)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"batch_1","object":"batch","status":"validating"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/batches":
			listPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"batch_1"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/batches/batch_1":
			getPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"batch_1","status":"completed"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/batches/batch_1/cancel":
			cancelPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"batch_1","status":"cancelling"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(up.Close)

	cfg, err := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "` + up.URL + `" }
defaults:
  openai_dialect: openai
`))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	// Create
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/batches",
		strings.NewReader(`{"input_file_id":"file-abc","endpoint":"/v1/chat/completions","completion_window":"24h"}`))
	req.Header.Set("Authorization", "Bearer sk-batch")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "batch_1") {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	if createPath != "/batches" || createAuth != "Bearer sk-batch" {
		t.Fatalf("create upstream path/auth %q %q", createPath, createAuth)
	}
	if !strings.Contains(createBody, "file-abc") {
		t.Fatalf("create body not forwarded: %s", createBody)
	}
	ev := col.one(t)
	if ev.Status != hooks.StatusOK || ev.Provider != "openai" || !ev.Estimated {
		t.Fatalf("create event: %+v", ev)
	}

	// List
	resp2, err := http.Get(gw.URL + "/v1/batches?provider=openai")
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 200 || listPath != "/batches" {
		t.Fatalf("list: %d path=%q body=%s", resp2.StatusCode, listPath, b2)
	}

	// Get
	resp3, err := http.Get(gw.URL + "/v1/batches/batch_1")
	if err != nil {
		t.Fatal(err)
	}
	b3, _ := io.ReadAll(resp3.Body)
	resp3.Body.Close()
	if resp3.StatusCode != 200 || getPath != "/batches/batch_1" {
		t.Fatalf("get: %d path=%q body=%s", resp3.StatusCode, getPath, b3)
	}

	// Cancel
	req4, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/batches/batch_1/cancel", nil)
	req4.Header.Set("Authorization", "Bearer sk-batch")
	resp4, err := http.DefaultClient.Do(req4)
	if err != nil {
		t.Fatal(err)
	}
	b4, _ := io.ReadAll(resp4.Body)
	resp4.Body.Close()
	if resp4.StatusCode != 200 || cancelPath != "/batches/batch_1/cancel" {
		t.Fatalf("cancel: %d path=%q body=%s", resp4.StatusCode, cancelPath, b4)
	}
}

func TestOpenAIBatchesWrongKind(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  anthropic: { kind: anthropic, base_url: "http://127.0.0.1:9" }
defaults:
  openai_dialect: anthropic
`))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	resp, err := http.Get(gw.URL + "/v1/batches")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		t.Fatal("want non-openai family rejected")
	}
}
