// Package gateway is the library entrypoint. Embed the gateway in your own
// process with New, or run the standalone binary in cmd/gateway.
package gateway

import (
	"fmt"
	"net/http"

	"github.com/mamad/llm-gateway/config"
	"github.com/mamad/llm-gateway/hooks"
	"github.com/mamad/llm-gateway/hooks/jsonl"
	"github.com/mamad/llm-gateway/proxy"
)

type Option func(*options)

type options struct {
	extraHooks []hooks.Hook
}

// WithHook registers an additional in-process usage hook.
func WithHook(h hooks.Hook) Option {
	return func(o *options) { o.extraHooks = append(o.extraHooks, h) }
}

// New builds the gateway HTTP handler from config. Hooks configured in the
// YAML (jsonl, webhook) are wired automatically; WithHook adds more.
func New(cfg *config.Config, opts ...Option) (http.Handler, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	var all hooks.Multi
	if cfg.Hooks.JSONL != nil {
		sink, err := jsonl.New(cfg.Hooks.JSONL.Output)
		if err != nil {
			return nil, fmt.Errorf("hooks.jsonl: %w", err)
		}
		all = append(all, sink)
	}
	all = append(all, o.extraHooks...)
	return proxy.NewServer(cfg, all).Handler(), nil
}
