package main

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/inja-online/llm-gateway/config"
)

func TestRunSuccess(t *testing.T) {
	path := writeCfg(t, `
providers:
  up: { kind: openai_compat, base_url: "https://example.com/v1" }
listen: ":0"
`)
	oldServe := listenAndServe
	listenAndServe = func(addr string, h http.Handler) error {
		if addr != ":0" {
			t.Errorf("addr = %q", addr)
		}
		if h == nil {
			t.Error("nil handler")
		}
		return nil
	}
	t.Cleanup(func() { listenAndServe = oldServe })

	if err := run([]string{"-config", path}); err != nil {
		t.Fatal(err)
	}
}

func TestRunBadConfig(t *testing.T) {
	err := run([]string{"-config", filepath.Join(t.TempDir(), "missing.yaml")})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunInitError(t *testing.T) {
	path := writeCfg(t, `
providers:
  up: { kind: openai, base_url: "https://example.com/v1" }
hooks:
  jsonl: { output: "/no/such/dir/x.jsonl" }
`)
	if err := run([]string{"-config", path}); err == nil {
		t.Fatal("expected init error")
	}
}

func TestRunListenError(t *testing.T) {
	path := writeCfg(t, `
providers:
  up: { kind: openai, base_url: "https://example.com/v1" }
`)
	oldServe := listenAndServe
	listenAndServe = func(string, http.Handler) error { return errors.New("bind failed") }
	t.Cleanup(func() { listenAndServe = oldServe })

	if err := run([]string{"-config", path}); err == nil || err.Error() != "bind failed" {
		t.Fatalf("got %v", err)
	}
}

func TestRunFlagError(t *testing.T) {
	if err := run([]string{"-not-a-real-flag"}); err == nil {
		t.Fatal("expected flag error")
	}
}

func TestLoadConfigOverride(t *testing.T) {
	// exercise default loadConfig wiring by temporarily swapping
	old := loadConfig
	called := false
	loadConfig = func(path string) (*config.Config, error) {
		called = true
		return old(path)
	}
	t.Cleanup(func() { loadConfig = old })

	path := writeCfg(t, `providers: { up: { kind: openai, base_url: "https://x" } }`)
	oldServe := listenAndServe
	listenAndServe = func(string, http.Handler) error { return nil }
	t.Cleanup(func() { listenAndServe = oldServe })

	if err := run([]string{"-config", path}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("loadConfig not used")
	}
}

func writeCfg(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gateway.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMainEntrySuccess(t *testing.T) {
	path := writeCfg(t, `providers: { up: { kind: openai, base_url: "https://x" } }`)
	oldArgs := os.Args
	os.Args = []string{"gateway", "-config", path}
	oldServe := listenAndServe
	listenAndServe = func(string, http.Handler) error { return nil }
	oldFatal := fatal
	fatal = func(v ...any) { t.Fatalf("fatal called: %v", v) }
	t.Cleanup(func() {
		os.Args = oldArgs
		listenAndServe = oldServe
		fatal = oldFatal
	})
	main()
}

func TestMainEntryFatal(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"gateway", "-config", filepath.Join(t.TempDir(), "missing.yaml")}
	var fatalCalled bool
	oldFatal := fatal
	fatal = func(v ...any) { fatalCalled = true }
	t.Cleanup(func() {
		os.Args = oldArgs
		fatal = oldFatal
	})
	main()
	if !fatalCalled {
		t.Fatal("expected fatal on bad config")
	}
}
