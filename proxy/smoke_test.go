package proxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

// Smoke tests: full matrix of dialect × provider family for non-stream and stream.

func TestSmokeOpenAIToOpenAI(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"id":"c","choices":[{"message":{"role":"assistant","content":"pong"}}],"usage":{"prompt_tokens":4,"completion_tokens":1}}`)
	}))
	gw, col := newTestGateway(t, upstream)

	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"up/smoke","messages":[{"role":"user","content":"ping"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(string(body), "pong") {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	ev := col.one(t)
	assertOKUsage(t, ev, 4, 1, false)
}

func TestSmokeAnthropicToAnthropic(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Errorf("path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"id":"msg_1","type":"message","role":"assistant","model":"claude","content":[{"type":"text","text":"pong"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":2}}`)
	}))
	gw, col := newAnthropicGateway(t, upstream)

	resp, err := http.Post(gw.URL+"/v1/messages", "application/json",
		strings.NewReader(`{"model":"up/claude","max_tokens":32,"messages":[{"role":"user","content":"ping"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(string(body), "pong") {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	assertOKUsage(t, col.one(t), 5, 2, false)
}

func TestSmokeOpenAIToAnthropicNonStream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "claude-x" {
			t.Errorf("model %v", body["model"])
		}
		if _, ok := body["messages"]; !ok {
			t.Error("missing messages")
		}
		fmt.Fprint(w, `{"id":"msg_a","type":"message","role":"assistant","model":"claude-x","content":[{"type":"text","text":"translated"}],"stop_reason":"end_turn","usage":{"input_tokens":9,"output_tokens":3}}`)
	}))
	gw, col := newTranslateGateway(t, upstream)

	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"claude/claude-x","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	var oa struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &oa); err != nil {
		t.Fatal(err)
	}
	if len(oa.Choices) == 0 || oa.Choices[0].Message.Content != "translated" {
		t.Fatalf("body %s", body)
	}
	if oa.Usage.PromptTokens != 9 || oa.Usage.CompletionTokens != 3 {
		t.Fatalf("usage %+v", oa.Usage)
	}
	assertOKUsage(t, col.one(t), 9, 3, false)
}

func TestSmokeAnthropicToOpenAINonStream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":"chatcmpl-1","choices":[{"message":{"role":"assistant","content":"from-oa"},"finish_reason":"stop"}],"usage":{"prompt_tokens":6,"completion_tokens":2}}`)
	}))
	gw, col := newAnthropicToOpenAIGateway(t, upstream)

	resp, err := http.Post(gw.URL+"/v1/messages", "application/json",
		strings.NewReader(`{"model":"m","max_tokens":50,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	var ant struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &ant); err != nil {
		t.Fatal(err)
	}
	if len(ant.Content) == 0 || ant.Content[0].Text != "from-oa" {
		t.Fatalf("%s", body)
	}
	assertOKUsage(t, col.one(t), 6, 2, false)
}

func TestSmokeOpenAIToAnthropicStream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		chunks := []string{
			`event: message_start` + "\n" + `data: {"type":"message_start","message":{"id":"msg_s","model":"c","usage":{"input_tokens":11,"output_tokens":0}}}` + "\n\n",
			`event: content_block_start` + "\n" + `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n",
			`event: content_block_delta` + "\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}` + "\n\n",
			`event: content_block_stop` + "\n" + `data: {"type":"content_block_stop","index":0}` + "\n\n",
			`event: message_delta` + "\n" + `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}` + "\n\n",
			`event: message_stop` + "\n" + `data: {"type":"message_stop"}` + "\n\n",
		}
		for _, c := range chunks {
			io.WriteString(w, c)
			fl.Flush()
		}
	}))
	gw, col := newTranslateGateway(t, upstream)

	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"claude/c","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(raw), "hi") || !strings.Contains(string(raw), "[DONE]") {
		t.Fatalf("stream: %s", raw)
	}
	ev := col.one(t)
	if !ev.Stream || ev.Status != hooks.StatusOK {
		t.Fatalf("%+v", ev)
	}
	if ev.TokensIn != 11 || ev.TokensOut != 1 {
		t.Fatalf("tokens %+v", ev)
	}
}

func TestSmokeAnthropicToOpenAIStream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		for _, c := range []string{
			`data: {"id":"c1","choices":[{"delta":{"role":"assistant","content":"he"}}]}` + "\n\n",
			`data: {"id":"c1","choices":[{"delta":{"content":"y"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}` + "\n\n",
			"data: [DONE]\n\n",
		} {
			io.WriteString(w, c)
			fl.Flush()
		}
	}))
	gw, col := newAnthropicToOpenAIGateway(t, upstream)

	resp, err := http.Post(gw.URL+"/v1/messages", "application/json",
		strings.NewReader(`{"model":"m","max_tokens":20,"stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	// Collect named Anthropic events
	var sawStart, sawDelta, sawStop bool
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "event: message_start") {
			sawStart = true
		}
		if strings.Contains(line, "text_delta") || strings.Contains(line, `"he"`) || strings.Contains(line, `"y"`) {
			sawDelta = true
		}
		if strings.HasPrefix(line, "event: message_stop") {
			sawStop = true
		}
	}
	if !sawStart || !sawStop {
		t.Fatalf("start=%v delta=%v stop=%v", sawStart, sawDelta, sawStop)
	}
	_ = sawDelta
	ev := col.one(t)
	if !ev.Stream || ev.Status != hooks.StatusOK {
		t.Fatalf("%+v", ev)
	}
}

func TestSmokeAliasAndDefaultRouting(t *testing.T) {
	var models []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		models = append(models, fmt.Sprint(body["model"]))
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  up: { kind: openai_compat, base_url: %q }
defaults:
  openai_dialect: up
aliases:
  fast: up/deepseek-chat
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	t.Cleanup(upstream.Close)

	for _, model := range []string{"fast", "bare-model"} {
		resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json",
			strings.NewReader(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"x"}]}`, model)))
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	if len(models) != 2 || models[0] != "deepseek-chat" || models[1] != "bare-model" {
		t.Fatalf("models=%v", models)
	}
	if len(col.events) != 2 {
		t.Fatalf("events=%d", len(col.events))
	}
}

func assertOKUsage(t *testing.T, ev hooks.UsageEvent, in, out int, stream bool) {
	t.Helper()
	if ev.Status != hooks.StatusOK || ev.Estimated {
		t.Fatalf("status/est: %+v", ev)
	}
	if ev.TokensIn != in || ev.TokensOut != out {
		t.Fatalf("tokens in=%d out=%d want %d/%d", ev.TokensIn, ev.TokensOut, in, out)
	}
	if ev.Stream != stream {
		t.Fatalf("stream=%v", ev.Stream)
	}
	if ev.RequestID == "" || ev.LatencyMS < 0 {
		t.Fatalf("meta %+v", ev)
	}
}
