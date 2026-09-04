package dashboard

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/inja-online/llm-gateway/hooks"
	"github.com/inja-online/llm-gateway/hooks/ring"
)

type logsResp struct {
	Events     []hooks.UsageEvent `json:"events"`
	NextCursor string             `json:"next_cursor"`
}

func TestLogsNewestFirst(t *testing.T) {
	h := New(Options{Ring: ringWith(sampleEvents())}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/logs", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
	}
	body := rec.Body.Bytes()
	if bytes.Contains(body, []byte("access_token")) {
		t.Fatalf("leaked access_token: %s", body)
	}
	var got logsResp
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Events) != 3 {
		t.Fatalf("events %+v", got.Events)
	}
	if got.Events[0].RequestID != "req_3" || got.Events[1].RequestID != "req_2" || got.Events[2].RequestID != "req_1" {
		t.Fatalf("order %+v", ids(got.Events))
	}
	if got.NextCursor != "req_1" {
		t.Fatalf("next_cursor %q", got.NextCursor)
	}
}

func TestLogsQuery(t *testing.T) {
	h := New(Options{Ring: ringWith(sampleEvents())}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/logs?q=req_2", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
	}
	var got logsResp
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Events) != 1 || got.Events[0].RequestID != "req_2" {
		t.Fatalf("%+v", got.Events)
	}
}

func TestLogsCursorLimit(t *testing.T) {
	h := New(Options{Ring: ringWith(sampleEvents())}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/logs?limit=2", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
	}
	var page logsResp
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 2 || page.Events[0].RequestID != "req_3" || page.Events[1].RequestID != "req_2" {
		t.Fatalf("page1 %+v", page)
	}
	if page.NextCursor != "req_2" {
		t.Fatalf("cursor %q", page.NextCursor)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/logs?limit=2&cursor=req_2", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 || page.Events[0].RequestID != "req_1" {
		t.Fatalf("page2 %+v", page)
	}
}

func TestLogsStreamOneEvent(t *testing.T) {
	orig := heartbeat
	heartbeat = 100 * time.Millisecond
	t.Cleanup(func() { heartbeat = orig })

	r := ring.New(8)
	srv := httptest.NewServer(New(Options{Ring: r}).Handler())
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/v1/dashboard/logs/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type %q", ct)
	}

	r.OnUsage(context.Background(), hooks.UsageEvent{
		RequestID: "live_1",
		Time:      time.Date(2026, 9, 4, 16, 0, 0, 0, time.UTC),
		Provider:  "openai",
		Model:     "gpt-4",
		Status:    hooks.StatusOK,
	})

	got := make(chan hooks.UsageEvent, 1)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		var evLine string
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "data:") {
				evLine = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				var ev hooks.UsageEvent
				if json.Unmarshal([]byte(evLine), &ev) == nil && ev.RequestID != "" {
					got <- ev
					return
				}
			}
		}
	}()

	select {
	case ev := <-got:
		if ev.RequestID != "live_1" {
			t.Fatalf("event %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for sse event")
	}
	cancel()
}

func TestLogsStreamHeartbeat(t *testing.T) {
	orig := heartbeat
	heartbeat = 100 * time.Millisecond
	t.Cleanup(func() { heartbeat = orig })

	srv := httptest.NewServer(New(Options{Ring: ring.New(4)}).Handler())
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/v1/dashboard/logs/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	hit := make(chan struct{}, 1)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), ":") {
				hit <- struct{}{}
				return
			}
		}
	}()
	select {
	case <-hit:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for heartbeat comment")
	}
	cancel()
}

func ids(evs []hooks.UsageEvent) []string {
	out := make([]string, len(evs))
	for i, ev := range evs {
		out[i] = ev.RequestID
	}
	return out
}
