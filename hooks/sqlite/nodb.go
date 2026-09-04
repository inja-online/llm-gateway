//go:build nodb

package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/inja-online/llm-gateway/hooks"
)

var _ hooks.Hook = (*Sink)(nil)

type Query struct {
	From, To                   time.Time
	Provider, Model, Status, Q string
	Limit                      int
	Cursor                     string
}

type Sink struct{}

func New(path string) (*Sink, error) {
	if path == "" {
		return &Sink{}, nil
	}
	return nil, fmt.Errorf("sqlite disabled (build with default tags)")
}

func (s *Sink) OnUsage(context.Context, hooks.UsageEvent) {}

func (s *Sink) Close() error { return nil }

func (s *Sink) Query(context.Context, Query) ([]hooks.UsageEvent, error) { return nil, nil }

func (s *Sink) SetPath(path string) error {
	if path == "" {
		return nil
	}
	return fmt.Errorf("sqlite disabled (build with default tags)")
}

func (s *Sink) Path() string { return "" }
