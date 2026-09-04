//go:build nodb

package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/hooks"
)

func TestAvailableFalse(t *testing.T) {
	if Available() {
		t.Fatal("want Available false")
	}
}

func TestNewEmptyNoOp(t *testing.T) {
	s, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	if s.Path() != "" {
		t.Fatalf("Path = %q", s.Path())
	}
	s.OnUsage(context.Background(), hooks.UsageEvent{RequestID: "x"})
	got, err := s.Query(context.Background(), Query{})
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("want nil, got %+v", got)
	}
}

func TestNewNonEmptyError(t *testing.T) {
	_, err := New("/tmp/usage.sqlite")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "sqlite disabled (build with default tags)") {
		t.Fatalf("err = %v", err)
	}
}
