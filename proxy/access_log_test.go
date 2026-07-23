package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/hooks"
)

func TestFormatUsageLog(t *testing.T) {
	ev := hooks.UsageEvent{
		RequestID:     "abc",
		DialectIn:     "openai",
		Provider:      "chatgpt",
		Model:         "claude/fable-5",
		UpstreamModel: "claude-fable-5",
		TokensIn:      10,
		TokensOut:     3,
		HTTPStatus:    200,
		LatencyMS:     42,
		Status:        hooks.StatusOK,
		Stream:        true,
	}
	s := formatUsageLog(ev)
	for _, want := range []string{
		"usage status=ok",
		"provider=chatgpt",
		"model=claude/fable-5",
		"tokens_in=10",
		"tokens_out=3",
		"stream=true",
		"req=abc",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in %q", want, s)
		}
	}
}

func TestAccessLogSkipsHealthz(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	h := withAccessLog(inner)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d", rr.Code)
	}

	// metrics skipped too
	rrM := httptest.NewRecorder()
	h.ServeHTTP(rrM, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req2.URL.RawQuery = "debug=1"
	h.ServeHTTP(rr2, req2)
	if rr2.Code != 200 {
		t.Fatalf("status %d", rr2.Code)
	}
}

func TestStatusRecorderInterfaces(t *testing.T) {
	rr := httptest.NewRecorder()
	rec := &statusRecorder{ResponseWriter: rr, status: http.StatusOK}
	if rec.Unwrap() != rr {
		t.Fatal("unwrap")
	}
	rec.Flush() // ResponseRecorder is a Flusher
	// Write without prior WriteHeader
	rec2 := &statusRecorder{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	_, _ = rec2.Write([]byte("hi"))
	if rec2.status != http.StatusOK || rec2.bytes != 2 {
		t.Fatalf("%+v", rec2)
	}
	// Hijack on non-hijacker
	if _, _, err := rec2.Hijack(); err == nil {
		t.Fatal("want hijack error")
	}
}
