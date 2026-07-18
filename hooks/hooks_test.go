package hooks

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestMultiFansOut(t *testing.T) {
	var a, b atomic.Int32
	m := Multi{
		Func(func(context.Context, UsageEvent) { a.Add(1) }),
		Func(func(context.Context, UsageEvent) { b.Add(1) }),
	}
	m.OnUsage(context.Background(), UsageEvent{RequestID: "r1", Status: StatusOK, Time: time.Now()})
	if a.Load() != 1 || b.Load() != 1 {
		t.Fatalf("a=%d b=%d", a.Load(), b.Load())
	}
}

func TestMultiEmpty(t *testing.T) {
	Multi{}.OnUsage(context.Background(), UsageEvent{})
}

func TestFuncAdapter(t *testing.T) {
	var saw string
	h := Func(func(_ context.Context, ev UsageEvent) { saw = ev.Status })
	h.OnUsage(context.Background(), UsageEvent{Status: StatusClientAbort})
	if saw != StatusClientAbort {
		t.Fatalf("got %q", saw)
	}
}
