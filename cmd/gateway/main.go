// Command gateway runs the standalone LLM gateway.
//
// Usage:
//
//	gateway -config gateway.yaml
//	GATEWAY_CONFIG=gateway.yaml GATEWAY_LISTEN=:8787 gateway
//	GATEWAY_TLS_CERT=… GATEWAY_TLS_KEY=… gateway   # HTTPS (Claude Code local)
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	gateway "github.com/inja-online/llm-gateway"
	"github.com/inja-online/llm-gateway/config"
)

// version is set via -ldflags at release time.
var version = "dev"

// loadConfig is overridden in tests.
var loadConfig = config.Load

// newGateway is overridden in tests.
var newGateway = gateway.New

// fatal is overridden in tests so main can be exercised without os.Exit.
var fatal = log.Fatal

// notifySignals is overridden in tests.
var notifySignals = func() (<-chan os.Signal, func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	return ch, func() { signal.Stop(ch) }
}

// serve runs the HTTP or HTTPS server until shutdown or error. Overridden in tests.
var serve = func(cfg *config.Config, h http.Handler) error {
	if cfg == nil {
		return fmt.Errorf("serve: nil config")
	}
	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		// No overall Read/Write timeout: streams are long-lived. Client
		// disconnect cancels the request context and aborts the upstream.
	}

	errCh := make(chan error, 1)
	go func() {
		var err error
		if cfg.TLSEnabled() {
			err = srv.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	sigCh, stop := notifySignals()
	defer stop()

	select {
	case err := <-errCh:
		return err
	case sig := <-sigCh:
		log.Printf("llm-gateway: shutdown signal %v", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		return <-errCh
	}
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fatal(err)
	}
}

func run(args []string) error {
	// Subcommands that do not start the HTTP server.
	if len(args) > 0 {
		switch args[0] {
		case "auth":
			return runAuth(args[1:])
		case "version", "-version", "--version":
			fmt.Println("llm-gateway", version)
			return nil
		case "help", "-h", "--help":
			fmt.Fprintf(os.Stderr, `Usage:
  llm-gateway -config gateway.yaml     Run the HTTP/HTTPS gateway
  llm-gateway auth login chatgpt       ChatGPT subscription OAuth (Codex PKCE)
  llm-gateway auth login claude        Claude subscription setup-token
  llm-gateway auth login grok          SuperGrok / X Premium+ device OAuth
  llm-gateway auth status|logout|env   Manage stored subscription credentials
  llm-gateway version

Env:
  GATEWAY_CONFIG       config path
  GATEWAY_LISTEN       bind address (default :8787)
  GATEWAY_TLS_CERT     PEM cert path (with GATEWAY_TLS_KEY enables HTTPS)
  GATEWAY_TLS_KEY      PEM key path
  INJA_GATEWAY_AUTH_FILE  subscription credential store
`)
			return nil
		}
	}

	fs := flag.NewFlagSet("gateway", flag.ContinueOnError)
	defaultCfg := envOr("GATEWAY_CONFIG", "gateway.yaml")
	cfgPath := fs.String("config", defaultCfg, "path to config file (or GATEWAY_CONFIG)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	h, err := newGateway(cfg)
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}
	scheme := "http"
	if cfg.TLSEnabled() {
		scheme = "https"
	}
	log.Printf("llm-gateway %s listening on %s://%s (%d providers)", version, scheme, cfg.Listen, len(cfg.Providers))
	return serve(cfg, h)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
