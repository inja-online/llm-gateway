// Command gateway runs the standalone LLM gateway.
//
// Usage: gateway -config gateway.yaml
package main

import (
	"flag"
	"log"
	"net/http"

	gateway "github.com/mamad/llm-gateway"
	"github.com/mamad/llm-gateway/config"
)

func main() {
	cfgPath := flag.String("config", "gateway.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	h, err := gateway.New(cfg)
	if err != nil {
		log.Fatalf("init: %v", err)
	}
	log.Printf("llm-gateway listening on %s (%d providers)", cfg.Listen, len(cfg.Providers))
	if err := http.ListenAndServe(cfg.Listen, h); err != nil {
		log.Fatal(err)
	}
}
