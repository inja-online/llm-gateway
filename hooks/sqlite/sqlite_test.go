//go:build !nodb

package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/inja-online/llm-gateway/hooks"
)

func TestInsertQueryRequestID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	ev := hooks.UsageEvent{
		RequestID: "req_1",
		Time:      time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC),
		Provider:  "openai",
		Model:     "gpt-4",
		Status:    hooks.StatusOK,
	}
	s.OnUsage(context.Background(), ev)

	got, err := s.Query(context.Background(), Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].RequestID != "req_1" {
		t.Fatalf("%+v", got)
	}
}

func TestEmptyPathNoOp(t *testing.T) {
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

func TestAvailable(t *testing.T) {
	if !Available() {
		t.Fatal("want Available true")
	}
}

func TestFileMode0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	s.OnUsage(context.Background(), hooks.UsageEvent{
		RequestID: "req_mode",
		Time:      time.Now().UTC(),
	})
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o want 0600", st.Mode().Perm())
	}
}

func TestDroppedFieldsAndMediaRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	ev := hooks.UsageEvent{
		RequestID:     "req_media",
		Time:          time.Date(2026, 9, 4, 13, 0, 0, 0, time.UTC),
		DroppedFields: []string{"tools", "stop"},
		Media:         &hooks.MediaUsage{Units: 2, UnitKind: hooks.MediaUnitImage, Size: "1024x1024"},
	}
	s.OnUsage(context.Background(), ev)

	got, err := s.Query(context.Background(), Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("%+v", got)
	}
	if len(got[0].DroppedFields) != 2 || got[0].DroppedFields[0] != "tools" {
		t.Fatalf("dropped = %#v", got[0].DroppedFields)
	}
	if got[0].Media == nil || got[0].Media.Units != 2 || got[0].Media.UnitKind != hooks.MediaUnitImage {
		t.Fatalf("media = %#v", got[0].Media)
	}
}

func TestSetPathEmptyDisables(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.sqlite")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	s.OnUsage(context.Background(), hooks.UsageEvent{RequestID: "a", Time: time.Now().UTC()})
	if err := s.SetPath(""); err != nil {
		t.Fatal(err)
	}
	if s.Path() != "" {
		t.Fatalf("Path = %q", s.Path())
	}
	s.OnUsage(context.Background(), hooks.UsageEvent{RequestID: "b", Time: time.Now().UTC()})
	got, err := s.Query(context.Background(), Query{})
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("want nil after disable, got %+v", got)
	}
}

func TestQueryFilters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	t1 := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 9, 4, 11, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	s.OnUsage(context.Background(), hooks.UsageEvent{RequestID: "req_a", Time: t1, Provider: "openai", Model: "gpt-4", Status: hooks.StatusOK})
	s.OnUsage(context.Background(), hooks.UsageEvent{RequestID: "req_b", Time: t2, Provider: "anthropic", Model: "claude", Status: hooks.StatusUpstreamError})
	s.OnUsage(context.Background(), hooks.UsageEvent{RequestID: "req_c", Time: t3, Provider: "openai", Model: "gpt-4", Status: hooks.StatusOK})

	got, err := s.Query(context.Background(), Query{Provider: "openai"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("provider filter: %+v", got)
	}

	got, err = s.Query(context.Background(), Query{Q: "req_b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].RequestID != "req_b" {
		t.Fatalf("q filter: %+v", got)
	}

	got, err = s.Query(context.Background(), Query{From: t2, To: t3})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("time filter: %+v", got)
	}
}
