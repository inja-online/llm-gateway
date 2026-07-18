package jsonl

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/inja-online/llm-gateway/hooks"
)

func TestStdoutSink(t *testing.T) {
	s, err := New("stdout")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	// empty string defaults to stdout
	s2, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	_ = s2
}

func TestStderrSink(t *testing.T) {
	s, err := New("stderr")
	if err != nil {
		t.Fatal(err)
	}
	s.OnUsage(context.Background(), hooks.UsageEvent{
		RequestID: "req_x",
		Time:      time.Now().UTC(),
		Status:    hooks.StatusOK,
	})
}

func TestFileSinkAppend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	ev := hooks.UsageEvent{
		RequestID: "req_1",
		Time:      time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC),
		DialectIn: "openai",
		Provider:  "deepseek",
		Model:     "deepseek/chat",
		TokensIn:  3,
		TokensOut: 1,
		Status:    hooks.StatusOK,
		HTTPStatus: 200,
	}
	s.OnUsage(context.Background(), ev)
	s.OnUsage(context.Background(), ev)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), raw)
	}
	var parsed hooks.UsageEvent
	if err := json.Unmarshal([]byte(lines[0]), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.RequestID != "req_1" || parsed.TokensIn != 3 {
		t.Fatalf("parsed: %+v", parsed)
	}

	// reopen appends
	s2, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	s2.OnUsage(context.Background(), ev)
	s2.Close()
	raw, _ = os.ReadFile(path)
	if n := strings.Count(string(raw), "\n"); n != 3 {
		t.Fatalf("want 3 lines after reopen, got %d", n)
	}
}

func TestFileSinkBadPath(t *testing.T) {
	_, err := New(filepath.Join(t.TempDir(), "nope", "missing", "f.jsonl"))
	if err == nil {
		t.Fatal("expected error for missing parent dir")
	}
}
