package main

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/inja-online/llm-gateway/config"
)

func TestRunSuccess(t *testing.T) {
	path := writeCfg(t, `
providers:
  up: { kind: openai_compat, base_url: "https://example.com/v1" }
listen: ":0"
`)
	oldServe := serve
	serve = func(addr string, h http.Handler) error {
		if addr != ":0" {
			t.Errorf("addr = %q", addr)
		}
		if h == nil {
			t.Error("nil handler")
		}
		return nil
	}
	t.Cleanup(func() { serve = oldServe })

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
	oldServe := serve
	serve = func(string, http.Handler) error { return errors.New("bind failed") }
	t.Cleanup(func() { serve = oldServe })

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
	old := loadConfig
	called := false
	loadConfig = func(path string) (*config.Config, error) {
		called = true
		return old(path)
	}
	t.Cleanup(func() { loadConfig = old })

	path := writeCfg(t, `providers: { up: { kind: openai, base_url: "https://x" } }`)
	oldServe := serve
	serve = func(string, http.Handler) error { return nil }
	t.Cleanup(func() { serve = oldServe })

	if err := run([]string{"-config", path}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("loadConfig not used")
	}
}

func TestEnvOrAndConfigDefault(t *testing.T) {
	if envOr("GATEWAY_CONFIG_UNSET_XYZ", "fallback") != "fallback" {
		t.Fatal()
	}
	t.Setenv("GATEWAY_CONFIG_UNSET_XYZ", "set")
	if envOr("GATEWAY_CONFIG_UNSET_XYZ", "fallback") != "set" {
		t.Fatal()
	}
}

func TestMainEntrySuccess(t *testing.T) {
	path := writeCfg(t, `providers: { up: { kind: openai, base_url: "https://x" } }`)
	oldArgs := os.Args
	os.Args = []string{"gateway", "-config", path}
	oldServe := serve
	serve = func(string, http.Handler) error { return nil }
	oldFatal := fatal
	fatal = func(v ...any) { t.Fatalf("fatal called: %v", v) }
	t.Cleanup(func() {
		os.Args = oldArgs
		serve = oldServe
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

func TestServeGracefulShutdown(t *testing.T) {
	// Real serve() with a fake signal that fires immediately.
	oldNotify := notifySignals
	notifySignals = func() (<-chan os.Signal, func()) {
		ch := make(chan os.Signal, 1)
		ch <- os.Interrupt
		return ch, func() {}
	}
	t.Cleanup(func() { notifySignals = oldNotify })

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	// Bind an ephemeral port.
	err := serve("127.0.0.1:0", h)
	if err != nil {
		t.Fatal(err)
	}
}

func TestServeListenError(t *testing.T) {
	oldNotify := notifySignals
	// Never signal; listen on an invalid address so ListenAndServe fails.
	notifySignals = func() (<-chan os.Signal, func()) {
		return make(chan os.Signal), func() {}
	}
	t.Cleanup(func() { notifySignals = oldNotify })

	err := serve("not a valid address!!!", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	if err == nil {
		t.Fatal("expected listen error")
	}
	// Give the goroutine a moment if any.
	time.Sleep(10 * time.Millisecond)
}

func writeCfg(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gateway.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
