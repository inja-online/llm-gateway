// Command gateway runs the standalone LLM gateway.
//
// Usage:
//
//	gateway -config gateway.yaml
//	GATEWAY_CONFIG=gateway.yaml GATEWAY_LISTEN=:8787 gateway
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

// serve runs the HTTP server until shutdown or error. Overridden in tests.
var serve = func(addr string, h http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		// No overall Read/Write timeout: streams are long-lived. Client
		// disconnect cancels the request context and aborts the upstream.
	}

	errCh := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
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
	log.Printf("llm-gateway %s listening on %s (%d providers)", version, cfg.Listen, len(cfg.Providers))
	return serve(cfg.Listen, h)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
