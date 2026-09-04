package ring

import (
	"context"
	"testing"

	"github.com/inja-online/llm-gateway/hooks"
)

func TestRingOverflowDropsOldest(t *testing.T) {
	s := New(2)
	s.OnUsage(context.Background(), hooks.UsageEvent{RequestID: "a"})
	s.OnUsage(context.Background(), hooks.UsageEvent{RequestID: "b"})
	s.OnUsage(context.Background(), hooks.UsageEvent{RequestID: "c"})
	got := s.Snapshot()
	if len(got) != 2 || got[0].RequestID != "b" || got[1].RequestID != "c" {
		t.Fatalf("%+v", got)
	}
}

func TestRingConcurrentSnapshot(t *testing.T) {
	s := New(32)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			s.OnUsage(context.Background(), hooks.UsageEvent{RequestID: "x"})
		}
	}()
	for i := 0; i < 100; i++ {
		_ = s.Snapshot()
	}
	<-done
}

func TestRingDefaultSize(t *testing.T) {
	for _, size := range []int{0, -1} {
		s := New(size)
		for i := 0; i < 2001; i++ {
			s.OnUsage(context.Background(), hooks.UsageEvent{RequestID: "x"})
		}
		if s.Len() != 2000 {
			t.Fatalf("New(%d) len=%d want 2000", size, s.Len())
		}
	}
}

func TestRingResizeDropsOldest(t *testing.T) {
	s := New(3)
	s.OnUsage(context.Background(), hooks.UsageEvent{RequestID: "a"})
	s.OnUsage(context.Background(), hooks.UsageEvent{RequestID: "b"})
	s.OnUsage(context.Background(), hooks.UsageEvent{RequestID: "c"})
	s.Resize(2)
	got := s.Snapshot()
	if len(got) != 2 || got[0].RequestID != "b" || got[1].RequestID != "c" {
		t.Fatalf("%+v", got)
	}
}
