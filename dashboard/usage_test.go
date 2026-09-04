package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/inja-online/llm-gateway/hooks"
	"github.com/inja-online/llm-gateway/hooks/ring"
	"github.com/inja-online/llm-gateway/hooks/sqlite"
)

func sampleEvents() []hooks.UsageEvent {
	t1 := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 9, 4, 11, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	return []hooks.UsageEvent{
		{RequestID: "req_1", Time: t1, Provider: "openai", Model: "gpt-4", TokensIn: 10, TokensOut: 5, Status: hooks.StatusOK},
		{RequestID: "req_2", Time: t2, Provider: "anthropic", Model: "claude", TokensIn: 20, TokensOut: 10, Status: hooks.StatusOK},
		{RequestID: "req_3", Time: t3, Provider: "openai", Model: "gpt-4", TokensIn: 2, TokensOut: 1, Status: hooks.StatusUpstreamError},
	}
}

func ringWith(evs []hooks.UsageEvent) *ring.Sink {
	r := ring.New(len(evs) + 1)
	for _, ev := range evs {
		r.OnUsage(context.Background(), ev)
	}
	return r
}

type usageResp struct {
	Totals struct {
		TokensIn   int            `json:"tokens_in"`
		TokensOut  int            `json:"tokens_out"`
		Requests   int            `json:"requests"`
		ByProvider map[string]int `json:"by_provider"`
		ByModel    map[string]int `json:"by_model"`
		ByStatus   map[string]int `json:"by_status"`
	} `json:"totals"`
	Recent []hooks.UsageEvent `json:"recent"`
}

func TestUsageFromRing(t *testing.T) {
	h := New(Options{Ring: ringWith(sampleEvents())}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/usage", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
	}
	body := rec.Body.Bytes()
	if bytes.Contains(body, []byte("access_token")) {
		t.Fatalf("leaked access_token: %s", body)
	}
	var got usageResp
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Totals.TokensIn != 32 || got.Totals.TokensOut != 16 || got.Totals.Requests != 3 {
		t.Fatalf("totals %+v", got.Totals)
	}
	if got.Totals.ByProvider["openai"] != 2 || got.Totals.ByProvider["anthropic"] != 1 {
		t.Fatalf("by_provider %v", got.Totals.ByProvider)
	}
	if got.Totals.ByModel["gpt-4"] != 2 || got.Totals.ByModel["claude"] != 1 {
		t.Fatalf("by_model %v", got.Totals.ByModel)
	}
	if got.Totals.ByStatus[hooks.StatusOK] != 2 || got.Totals.ByStatus[hooks.StatusUpstreamError] != 1 {
		t.Fatalf("by_status %v", got.Totals.ByStatus)
	}
	if len(got.Recent) != 3 || got.Recent[0].RequestID != "req_3" {
		t.Fatalf("recent %+v", got.Recent)
	}
}

func TestUsageFilters(t *testing.T) {
	h := New(Options{Ring: ringWith(sampleEvents())}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/usage?provider=openai&model=gpt-4", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
	}
	var got usageResp
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Totals.Requests != 2 || got.Totals.TokensIn != 12 || got.Totals.TokensOut != 6 {
		t.Fatalf("openai totals %+v", got.Totals)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/usage?from=2026-09-04T11:00:00Z&to=2026-09-04T12:00:00Z", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Totals.Requests != 2 {
		t.Fatalf("time window requests %d recent %+v", got.Totals.Requests, got.Recent)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/usage?from=nope", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad from status %d", rec.Code)
	}
}

func TestUsageFromJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	evs := sampleEvents()[:2]
	var buf bytes.Buffer
	for _, ev := range evs {
		b, err := json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	h := New(Options{JSONLPath: path}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/usage", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
	}
	var got usageResp
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Totals.Requests != 2 || got.Totals.TokensIn != 30 || got.Totals.TokensOut != 15 {
		t.Fatalf("jsonl totals %+v", got.Totals)
	}
}

func TestUsageJSONLSkipsInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	ev, err := json.Marshal(sampleEvents()[0])
	if err != nil {
		t.Fatal(err)
	}
	raw := append([]byte("this is not json\n"), ev...)
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	h := New(Options{JSONLPath: path}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/usage", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
	}
	var got usageResp
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Totals.Requests != 1 || len(got.Recent) != 1 || got.Recent[0].RequestID != "req_1" {
		t.Fatalf("%+v", got)
	}
}

type fakeSQL struct {
	path string
	evs  []hooks.UsageEvent
}

func (f fakeSQL) Query(context.Context, sqlite.Query) ([]hooks.UsageEvent, error) {
	return f.evs, nil
}
func (f fakeSQL) Path() string         { return f.path }
func (f fakeSQL) SetPath(string) error { return nil }

func TestUsagePrefersSQLite(t *testing.T) {
	sqlEv := []hooks.UsageEvent{{
		RequestID: "sql_1",
		Time:      time.Date(2026, 9, 4, 15, 0, 0, 0, time.UTC),
		Provider:  "grok",
		Model:     "grok-2",
		TokensIn:  7,
		TokensOut: 3,
		Status:    hooks.StatusOK,
	}}
	h := New(Options{
		Ring:   ringWith(sampleEvents()),
		SQLite: fakeSQL{path: "/tmp/usage.sqlite", evs: sqlEv},
	}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/usage", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
	}
	var got usageResp
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Totals.Requests != 1 || got.Totals.TokensIn != 7 || len(got.Recent) != 1 || got.Recent[0].RequestID != "sql_1" {
		t.Fatalf("want sqlite event, got %+v", got)
	}
}
