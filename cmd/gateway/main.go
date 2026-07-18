// Command gateway runs the standalone LLM gateway.
//
// Usage: gateway -config gateway.yaml
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	gateway "github.com/inja-online/llm-gateway"
	"github.com/inja-online/llm-gateway/config"
)

// version is set via -ldflags at release time.
var version = "dev"

// listenAndServe is overridden in tests.
var listenAndServe = http.ListenAndServe

// loadConfig is overridden in tests.
var loadConfig = config.Load

// newGateway is overridden in tests.
var newGateway = gateway.New

// fatal is overridden in tests so main can be exercised without os.Exit.
var fatal = log.Fatal

func main() {
	if err := run(os.Args[1:]); err != nil {
		fatal(err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("gateway", flag.ContinueOnError)
	cfgPath := fs.String("config", "gateway.yaml", "path to config file")
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
	if err := listenAndServe(cfg.Listen, h); err != nil {
		return err
	}
	return nil
}
