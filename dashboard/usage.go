package dashboard

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/inja-online/llm-gateway/hooks"
	"github.com/inja-online/llm-gateway/hooks/sqlite"
)

type usageTotals struct {
	TokensIn   int            `json:"tokens_in"`
	TokensOut  int            `json:"tokens_out"`
	Requests   int            `json:"requests"`
	ByProvider map[string]int `json:"by_provider"`
	ByModel    map[string]int `json:"by_model"`
	ByStatus   map[string]int `json:"by_status"`
}

func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	q, err := queryFrom(r, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_error", "", err.Error())
		return
	}
	evs, err := s.loadEvents(r.Context(), q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "api_error", "", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"totals": sumUsage(evs), "recent": evs})
}

func sumUsage(evs []hooks.UsageEvent) usageTotals {
	t := usageTotals{
		ByProvider: map[string]int{},
		ByModel:    map[string]int{},
		ByStatus:   map[string]int{},
	}
	for _, ev := range evs {
		t.TokensIn += ev.TokensIn
		t.TokensOut += ev.TokensOut
		t.Requests++
		t.ByProvider[ev.Provider]++
		t.ByModel[ev.Model]++
		t.ByStatus[ev.Status]++
	}
	return t
}

func (s *Server) loadEvents(ctx context.Context, q sqlite.Query) ([]hooks.UsageEvent, error) {
	if s.opts.SQLite != nil && s.opts.SQLite.Path() != "" {
		evs, err := s.opts.SQLite.Query(ctx, q)
		if evs == nil && err == nil {
			evs = []hooks.UsageEvent{}
		}
		return evs, err
	}
	var evs []hooks.UsageEvent
	switch {
	case jsonlFile(s.opts.JSONLPath):
		evs = tailJSONL(s.opts.JSONLPath)
	case s.opts.Ring != nil:
		evs = s.opts.Ring.Snapshot()
	}
	evs = filterEvents(evs, q)
	sort.Slice(evs, func(i, j int) bool {
		if !evs[i].Time.Equal(evs[j].Time) {
			return evs[i].Time.After(evs[j].Time)
		}
		return evs[i].RequestID > evs[j].RequestID
	})
	if q.Cursor != "" {
		i := 0
		for i < len(evs) && evs[i].RequestID != q.Cursor {
			i++
		}
		if i < len(evs) {
			evs = evs[i+1:]
		}
	}
	if q.Limit > 0 && len(evs) > q.Limit {
		evs = evs[:q.Limit]
	}
	if evs == nil {
		evs = []hooks.UsageEvent{}
	}
	return evs, nil
}

func filterEvents(evs []hooks.UsageEvent, q sqlite.Query) []hooks.UsageEvent {
	out := evs[:0]
	for _, ev := range evs {
		if !q.From.IsZero() && ev.Time.Before(q.From) {
			continue
		}
		if !q.To.IsZero() && ev.Time.After(q.To) {
			continue
		}
		if q.Provider != "" && ev.Provider != q.Provider {
			continue
		}
		if q.Model != "" && ev.Model != q.Model {
			continue
		}
		if q.Status != "" && ev.Status != q.Status {
			continue
		}
		if q.Q != "" && !strings.Contains(ev.RequestID, q.Q) {
			continue
		}
		out = append(out, ev)
	}
	return out
}

func queryFrom(r *http.Request, logs bool) (sqlite.Query, error) {
	var q sqlite.Query
	qs := r.URL.Query()
	if v := qs.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return q, err
		}
		q.From = t
	}
	if v := qs.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return q, err
		}
		q.To = t
	}
	q.Provider = qs.Get("provider")
	q.Model = qs.Get("model")
	if logs {
		q.Status = qs.Get("status")
		q.Q = qs.Get("q")
		q.Cursor = qs.Get("cursor")
		q.Limit = parseLimit(qs.Get("limit"))
	}
	return q, nil
}

func parseLimit(s string) int {
	if s == "" {
		return 100
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 100
	}
	if n > 500 {
		return 500
	}
	return n
}
