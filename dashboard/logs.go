package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/inja-online/llm-gateway/hooks"
)

var heartbeat = 15 * time.Second

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	q, err := queryFrom(r, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	evs, err := s.loadEvents(r.Context(), q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}
	next := ""
	if n := len(evs); n > 0 {
		next = evs[n-1].RequestID
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": evs, "next_cursor": next})
}

func (s *Server) handleLogStream(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "api_error", "stream unsupported")
		return
	}
	seen := map[string]struct{}{}
	if s.opts.Ring != nil {
		for _, ev := range s.opts.Ring.Snapshot() {
			seen[ev.RequestID] = struct{}{}
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	fl.Flush()

	poll := time.NewTicker(100 * time.Millisecond)
	defer poll.Stop()
	heart := time.NewTicker(heartbeat)
	defer heart.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heart.C:
			fmt.Fprint(w, ": \n\n")
			fl.Flush()
		case <-poll.C:
			if s.opts.Ring == nil {
				continue
			}
			for _, ev := range s.opts.Ring.Snapshot() {
				if _, ok := seen[ev.RequestID]; ok {
					continue
				}
				seen[ev.RequestID] = struct{}{}
				writeSSE(w, fl, ev)
			}
		}
	}
}

func writeSSE(w http.ResponseWriter, fl http.Flusher, ev hooks.UsageEvent) {
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: usage\ndata: %s\n\n", b)
	fl.Flush()
}
