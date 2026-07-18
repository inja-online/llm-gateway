package proxy

import (
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

// newAnthropicGateway wires /v1/messages to an Anthropic-kind upstream
// (passthrough path — what Claude Code uses against a real Anthropic account).
func newAnthropicGateway(t *testing.T, upstream *httptest.Server) (*httptest.Server, *collector) {
	t.Helper()
	return newGatewayWithKind(t, upstream, "anthropic")
}

// newAnthropicToOpenAIGateway wires /v1/messages to an OpenAI-compatible
// upstream (translation path — Claude Code against DeepSeek/GPT).
func newAnthropicToOpenAIGateway(t *testing.T, upstream *httptest.Server) (*httptest.Server, *collector) {
	t.Helper()
	return newGatewayWithKind(t, upstream, "openai_compat")
}

func newGatewayWithKind(t *testing.T, upstream *httptest.Server, kind string) (*httptest.Server, *collector) {
	t.Helper()
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  up: { kind: %s, base_url: %q }
defaults:
  anthropic_dialect: up
`, kind, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	t.Cleanup(upstream.Close)
	return gw, col
}

func TestAnthropicPassthroughNonStream(t *testing.T) {
	var gotKey, gotVersion, gotPath, gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		gotPath = r.URL.Path
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		gotModel, _ = body["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5",
			"content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn",
			"usage":{"input_tokens":11,"output_tokens":4}}`)
	}))
	gw, col := newAnthropicGateway(t, upstream)

	req, _ := http.NewRequest("POST", gw.URL+"/v1/messages",
		strings.NewReader(`{"model":"up/claude-sonnet-5","max_tokens":100,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("x-api-key", "sk-ant-client")
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if gotPath != "/messages" || gotKey != "sk-ant-client" || gotVersion == "" {
		t.Errorf("upstream request: path=%q key=%q version=%q", gotPath, gotKey, gotVersion)
	}
	if gotModel != "claude-sonnet-5" {
		t.Errorf("model prefix not stripped: %q", gotModel)
	}

	body, _ := io.ReadAll(resp.Body)
	var out map[string]any
	json.Unmarshal(body, &out)
	if out["type"] != "message" || out["stop_reason"] != "end_turn" {
		t.Errorf("anthropic shape not preserved: %s", body)
	}

	ev := col.one(t)
	if ev.TokensIn != 11 || ev.TokensOut != 4 || ev.DialectIn != DialectAnthropic || ev.Status != hooks.StatusOK {
		t.Errorf("event: %+v", ev)
	}
}

func TestAnthropicPassthroughStreamMeters(t *testing.T) {
	events := []string{
		`event: message_start` + "\n" + `data: {"type":"message_start","message":{"id":"m","model":"x","usage":{"input_tokens":12,"output_tokens":0}}}` + "\n\n",
		`event: content_block_delta` + "\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}` + "\n\n",
		`event: message_delta` + "\n" + `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":6}}` + "\n\n",
		`event: message_stop` + "\n" + `data: {"type":"message_stop"}` + "\n\n",
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		for _, e := range events {
			io.WriteString(w, e)
			fl.Flush()
		}
	}))
	gw, col := newAnthropicGateway(t, upstream)

	resp, err := http.Post(gw.URL+"/v1/messages", "application/json",
		strings.NewReader(`{"model":"m","max_tokens":10,"stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)

	if string(got) != strings.Join(events, "") {
		t.Errorf("passthrough altered the stream:\ngot:  %q\nwant: %q", got, strings.Join(events, ""))
	}
	ev := col.one(t)
	if ev.TokensIn != 12 || ev.TokensOut != 6 || !ev.Stream || ev.Estimated {
		t.Errorf("usage not extracted from anthropic stream: %+v", ev)
	}
}

func TestAnthropicToOpenAINonStream(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"cmpl-1","model":"deepseek-chat",
			"choices":[{"message":{"role":"assistant","content":"salut"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":6,"completion_tokens":2}}`)
	}))
	gw, col := newAnthropicToOpenAIGateway(t, upstream)

	resp, err := http.Post(gw.URL+"/v1/messages", "application/json", strings.NewReader(`{
		"model":"m","max_tokens":50,"system":"be brief",
		"messages":[{"role":"user","content":"hi"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if gotPath != "/chat/completions" {
		t.Errorf("upstream path = %q", gotPath)
	}
	msgs, _ := gotBody["messages"].([]any)
	if len(msgs) != 2 || msgs[0].(map[string]any)["role"] != "system" {
		t.Errorf("system not mapped to a system message: %v", gotBody["messages"])
	}

	body, _ := io.ReadAll(resp.Body)
	var out struct {
		Type       string `json:"type"`
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(body, &out) != nil || out.Type != "message" {
		t.Fatalf("not an anthropic response: %s", body)
	}
	if out.Content[0].Text != "salut" || out.StopReason != "end_turn" {
		t.Errorf("translated content: %+v", out)
	}
	if out.Usage.InputTokens != 6 || out.Usage.OutputTokens != 2 {
		t.Errorf("usage not translated: %+v", out.Usage)
	}

	ev := col.one(t)
	if ev.TokensIn != 6 || ev.TokensOut != 2 || ev.Status != hooks.StatusOK {
		t.Errorf("event: %+v", ev)
	}
}

func TestAnthropicToOpenAIStream(t *testing.T) {
	chunks := []string{
		`data: {"id":"c1","model":"gpt-4o","choices":[{"delta":{"role":"assistant"}}]}` + "\n\n",
		`data: {"id":"c1","choices":[{"delta":{"content":"Hel"}}]}` + "\n\n",
		`data: {"id":"c1","choices":[{"delta":{"content":"lo"}}]}` + "\n\n",
		`data: {"id":"c1","choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n",
		`data: {"id":"c1","choices":[],"usage":{"prompt_tokens":4,"completion_tokens":2}}` + "\n\n",
		`data: [DONE]` + "\n\n",
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		for _, c := range chunks {
			io.WriteString(w, c)
			fl.Flush()
		}
	}))
	gw, col := newAnthropicToOpenAIGateway(t, upstream)

	resp, err := http.Post(gw.URL+"/v1/messages", "application/json",
		strings.NewReader(`{"model":"m","max_tokens":10,"stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	out := string(body)

	// Must be Anthropic named events — what Claude Code dispatches on.
	for _, want := range []string{
		"event: message_start", "event: content_block_start",
		"event: content_block_delta", "event: content_block_stop",
		"event: message_delta", "event: message_stop",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}

	var text strings.Builder
	for _, l := range strings.Split(out, "\n") {
		if !strings.HasPrefix(l, "data: ") {
			continue
		}
		var ev struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		}
		json.Unmarshal([]byte(strings.TrimPrefix(l, "data: ")), &ev)
		if ev.Type == "content_block_delta" && ev.Delta.Type == "text_delta" {
			text.WriteString(ev.Delta.Text)
		}
	}
	if text.String() != "Hello" {
		t.Errorf("reassembled text = %q", text.String())
	}

	ev := col.one(t)
	if ev.TokensIn != 4 || ev.TokensOut != 2 || !ev.Stream || ev.Estimated {
		t.Errorf("event: %+v", ev)
	}
}

func TestAnthropicToOpenAIToolCallRoundTrip(t *testing.T) {
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"c","model":"m","choices":[{"message":{"role":"assistant","content":null,
			"tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Paris\"}"}}]},
			"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	gw, _ := newAnthropicToOpenAIGateway(t, upstream)

	resp, err := http.Post(gw.URL+"/v1/messages", "application/json", strings.NewReader(`{
		"model":"m","max_tokens":50,
		"tools":[{"name":"get_weather","description":"d","input_schema":{"type":"object"}}],
		"messages":[{"role":"user","content":"weather?"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	tools, _ := gotBody["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["type"] != "function" {
		t.Errorf("tools not translated: %v", gotBody["tools"])
	}

	body, _ := io.ReadAll(resp.Body)
	var out struct {
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type  string          `json:"type"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	}
	json.Unmarshal(body, &out)
	if out.StopReason != "tool_use" {
		t.Errorf("stop_reason = %q", out.StopReason)
	}
	if len(out.Content) != 1 || out.Content[0].Type != "tool_use" ||
		out.Content[0].Name != "get_weather" || out.Content[0].ID != "call_1" {
		t.Fatalf("tool_use block not produced: %s", body)
	}
	if string(out.Content[0].Input) != `{"city":"Paris"}` {
		t.Errorf("tool input: %s", out.Content[0].Input)
	}
}

func TestAnthropicErrorEnvelopeOnBadRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream must not be called when max_tokens is missing")
	}))
	gw, col := newAnthropicToOpenAIGateway(t, upstream)

	// max_tokens is required by the Anthropic API.
	resp, err := http.Post(gw.URL+"/v1/messages", "application/json",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var out struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &out) != nil || out.Type != "error" {
		t.Fatalf("not an anthropic error envelope: %s", body)
	}
	if resp.StatusCode != 400 {
		t.Errorf("status = %d", resp.StatusCode)
	}
	col.one(t)
}

func TestOpenAIErrorTranslatedToAnthropicShape(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(429)
		fmt.Fprint(w, `{"error":{"message":"rate limit","type":"rate_limit_error","code":null}}`)
	}))
	gw, col := newAnthropicToOpenAIGateway(t, upstream)

	resp, err := http.Post(gw.URL+"/v1/messages", "application/json",
		strings.NewReader(`{"model":"m","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var out struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	json.Unmarshal(body, &out)
	if out.Type != "error" || out.Error.Message != "rate limit" || out.Error.Type != "rate_limit_error" {
		t.Fatalf("openai error not reshaped for anthropic client: %s", body)
	}
	if resp.StatusCode != 429 {
		t.Errorf("status = %d", resp.StatusCode)
	}
	ev := col.one(t)
	if ev.Status != hooks.StatusUpstreamError {
		t.Errorf("event: %+v", ev)
	}
}
