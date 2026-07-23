// Package jsonl writes one JSON line per usage event to stdout, stderr, or a file.
package jsonl

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"

	"github.com/inja-online/llm-gateway/hooks"
)

type Sink struct {
	mu sync.Mutex
	w  io.Writer
	c  io.Closer // nil for stdout/stderr
}

// New creates a sink for output "stdout", "stderr", or a file path (append mode).
func New(output string) (*Sink, error) {
	switch output {
	case "", "stdout":
		return &Sink{w: os.Stdout}, nil
	case "stderr":
		return &Sink{w: os.Stderr}, nil
	default:
		f, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, err
		}
		return &Sink{w: f, c: f}, nil
	}
}

func (s *Sink) OnUsage(_ context.Context, ev hooks.UsageEvent) {
	line, err := json.Marshal(ev)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.w.Write(append(line, '\n'))
	// File sinks: push usage lines promptly for `tail -f` / cc-gateway-logs.
	if f, ok := s.w.(*os.File); ok && f != os.Stdout && f != os.Stderr {
		_ = f.Sync()
	}
}

func (s *Sink) Close() error {
	if s.c != nil {
		return s.c.Close()
	}
	return nil
}
