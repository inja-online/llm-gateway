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

	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	h.ServeHTTP(rr2, req2)
	if rr2.Code != 200 {
		t.Fatalf("status %d", rr2.Code)
	}
}
