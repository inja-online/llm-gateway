package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/hooks"
)

func TestAnthropicMissingModel(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	gw, col := newAnthropicGateway(t, upstream)
	resp, err := http.Post(gw.URL+"/v1/messages", "application/json",
		strings.NewReader(`{"max_tokens":10,"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatal(resp.StatusCode)
	}
	if col.one(t).Status != hooks.StatusBadRequest {
		t.Fatal()
	}
}

func TestAnthropicInvalidJSON(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	gw, col := newAnthropicGateway(t, upstream)
	resp, err := http.Post(gw.URL+"/v1/messages", "application/json", strings.NewReader(`{`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatal(resp.StatusCode)
	}
	col.one(t)
}

func TestAnthropicNoUsageEstimated(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":"m","type":"message","role":"assistant","model":"c","content":[{"type":"text","text":"x"}],"stop_reason":"end_turn"}`)
	}))
	gw, col := newAnthropicGateway(t, upstream)
	resp, err := http.Post(gw.URL+"/v1/messages", "application/json",
		strings.NewReader(`{"model":"m","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	ev := col.one(t)
	if !ev.Estimated || ev.Status != hooks.StatusOK {
		t.Fatalf("%+v", ev)
	}
}

func TestAnthropicUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"nope"}}`)
	}))
	gw, col := newAnthropicGateway(t, upstream)
	resp, err := http.Post(gw.URL+"/v1/messages", "application/json",
		strings.NewReader(`{"model":"m","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 400 || !strings.Contains(string(body), "nope") {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if col.one(t).Status != hooks.StatusUpstreamError {
		t.Fatal()
	}
}

func TestAnthropicStreamPassthrough(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		io.WriteString(w, `event: message_start`+"\n"+`data: {"type":"message_start","message":{"usage":{"input_tokens":8,"output_tokens":0}}}`+"\n\n")
		fl.Flush()
		io.WriteString(w, `event: message_delta`+"\n"+`data: {"type":"message_delta","usage":{"output_tokens":2}}`+"\n\n")
		fl.Flush()
	}))
	gw, col := newAnthropicGateway(t, upstream)
	resp, err := http.Post(gw.URL+"/v1/messages", "application/json",
		strings.NewReader(`{"model":"m","max_tokens":10,"stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	ev := col.one(t)
	if ev.TokensIn != 8 || ev.TokensOut != 2 || !ev.Stream {
		t.Fatalf("%+v", ev)
	}
}

func TestOpenAIToAnthropicBadRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("no call")
	}))
	gw, col := newTranslateGateway(t, upstream)
	// missing messages content that fails parse? invalid tool_choice
	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"m","tool_choice":"nope","messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatal(resp.StatusCode)
	}
	col.one(t)
}

func TestOpenAIToAnthropicUpstreamErrorTranslated(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(529)
		fmt.Fprint(w, `{"type":"error","error":{"type":"overloaded_error","message":"busy"}}`)
	}))
	gw, col := newTranslateGateway(t, upstream)
	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 529 || !strings.Contains(string(body), "busy") {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	// OpenAI envelope
	if !strings.Contains(string(body), `"error"`) {
		t.Fatalf("%s", body)
	}
	col.one(t)
}

func TestAnthropicToOpenAIUpstreamErrorTranslated(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		fmt.Fprint(w, `{"error":{"message":"bad key","type":"invalid_api_key"}}`)
	}))
	gw, col := newAnthropicToOpenAIGateway(t, upstream)
	resp, err := http.Post(gw.URL+"/v1/messages", "application/json",
		strings.NewReader(`{"model":"m","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 401 || !strings.Contains(string(body), "bad key") {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"type":"error"`) {
		t.Fatalf("want anthropic envelope: %s", body)
	}
	col.one(t)
}

func TestAnthropicToOpenAIBadParse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	gw, col := newAnthropicToOpenAIGateway(t, upstream)
	resp, err := http.Post(gw.URL+"/v1/messages", "application/json",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`)) // no max_tokens
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatal(resp.StatusCode)
	}
	col.one(t)
}

func TestCountTokensUnknownProvider(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	gw, _ := newAnthropicGateway(t, upstream)
	resp, err := http.Post(gw.URL+"/v1/messages/count_tokens", "application/json",
		strings.NewReader(`{"model":"unknown/x","messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatal(resp.StatusCode)
	}
}

func TestCountTokensInvalidJSON(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	gw, _ := newAnthropicGateway(t, upstream)
	resp, err := http.Post(gw.URL+"/v1/messages/count_tokens", "application/json", strings.NewReader(`{`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatal(resp.StatusCode)
	}
}

func TestEstimateTokensUnparseable(t *testing.T) {
	n := estimateTokens([]byte(`not-json-at-all`))
	if n < 1 {
		t.Fatal(n)
	}
}

func TestExtractUsageHelpers(t *testing.T) {
	ev := &hooks.UsageEvent{}
	if extractOpenAIUsage([]byte(`[DONE]`), ev) {
		t.Fatal()
	}
	if extractOpenAIUsage([]byte(`not-json`), ev) {
		t.Fatal()
	}
	if !extractOpenAIUsage([]byte(`{"usage":{"prompt_tokens":1,"completion_tokens":2}}`), ev) {
		t.Fatal()
	}
	if extractAnthropicUsage([]byte(`nope`), ev) {
		t.Fatal()
	}
}

func TestEnsureIncludeUsagePreservesExisting(t *testing.T) {
	req := map[string]any{
		"stream_options": map[string]any{"include_usage": false},
	}
	ensureIncludeUsage(req)
	opts := req["stream_options"].(map[string]any)
	if opts["include_usage"] != false {
		t.Fatalf("%v", opts)
	}
}
