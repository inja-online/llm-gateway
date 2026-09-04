package ring

import (
	"context"
	"sync"

	"github.com/inja-online/llm-gateway/hooks"
)

var _ hooks.Hook = (*Sink)(nil)

// Sink is an in-process last-N usage-event buffer. OnUsage never does IO.
type Sink struct {
	mu   sync.Mutex
	buf  []hooks.UsageEvent
	head int // oldest index
	n    int // occupied
}

// New returns a ring of the given capacity. size≤0 → 2000.
func New(size int) *Sink {
	if size <= 0 {
		size = 2000
	}
	return &Sink{buf: make([]hooks.UsageEvent, size)}
}

func (s *Sink) OnUsage(_ context.Context, ev hooks.UsageEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.n < len(s.buf) {
		s.buf[(s.head+s.n)%len(s.buf)] = ev
		s.n++
		return
	}
	s.buf[s.head] = ev
	s.head = (s.head + 1) % len(s.buf)
}

func (s *Sink) Snapshot() []hooks.UsageEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]hooks.UsageEvent, s.n)
	for i := 0; i < s.n; i++ {
		out[i] = s.buf[(s.head+i)%len(s.buf)]
	}
	return out
}

// Resize sets capacity. Shrinking drops oldest events.
func (s *Sink) Resize(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n <= 0 {
		n = 2000
	}
	if n == len(s.buf) {
		return
	}
	cur := make([]hooks.UsageEvent, s.n)
	for i := 0; i < s.n; i++ {
		cur[i] = s.buf[(s.head+i)%len(s.buf)]
	}
	if len(cur) > n {
		cur = cur[len(cur)-n:]
	}
	s.buf = make([]hooks.UsageEvent, n)
	copy(s.buf, cur)
	s.head = 0
	s.n = len(cur)
}

func (s *Sink) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}
