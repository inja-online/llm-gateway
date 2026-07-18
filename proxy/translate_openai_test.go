package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mamad/llm-gateway/config"
	"github.com/mamad/llm-gateway/hooks"
)

// newTranslateGateway wires an OpenAI-dialect client to an Anthropic-kind
// upstream so /v1/chat/completions goes through the translation path.
func newTranslateGateway(t *testing.T, upstream *httptest.Server) (*httptest.Server, *collector) {
	t.Helper()
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  claude: { kind: anthropic, base_url: %q }
defaults:
  openai_dialect: claude
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	t.Cleanup(upstream.Close)
	return gw, col
}

func TestOpenAIToAnthropicNonStream(t *testing.T) {
	var gotPath, gotVersion string
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotVersion = r.Header.Get("anthropic-version")
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5",
			"content":[{"type":"text","text":"Bonjour"}],"stop_reason":"end_turn",
			"usage":{"input_tokens":9,"output_tokens":2}}`)
	}))
	gw, col := newTranslateGateway(t, upstream)

	req, _ := http.NewRequest("POST", gw.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"claude/claude-sonnet-5","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("x-api-key", "sk-ant-client")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Upstream received an Anthropic request at /messages with version header.
	if gotPath != "/messages" {
		t.Errorf("upstream path = %q", gotPath)
	}
	if gotVersion == "" {
		t.Errorf("anthropic-version header not injected")
	}
	if gotBody["model"] != "claude-sonnet-5" {
		t.Errorf("model not stripped of prefix: %v", gotBody["model"])
	}

	// Client received an OpenAI response.
	body, _ := io.ReadAll(resp.Body)
	var out map[string]any
	json.Unmarshal(body, &out)
	if out["object"] != "chat.completion" {
		t.Fatalf("not an openai response: %s", body)
	}
	choice := out["choices"].([]any)[0].(map[string]any)
	msg := choice["message"].(map[string]any)
	if msg["content"] != "Bonjour" || choice["finish_reason"] != "stop" {
		t.Errorf("translated content: %v", choice)
	}

	ev := col.one(t)
	if ev.TokensIn != 9 || ev.TokensOut != 2 || ev.Status != hooks.StatusOK || ev.Provider != "claude" {
		t.Errorf("event: %+v", ev)
	}
}

func TestOpenAIToAnthropicStream(t *testing.T) {
	events := []string{
		`event: message_start` + "\n" + `data: {"type":"message_start","message":{"id":"msg_1","model":"claude-x","usage":{"input_tokens":5}}}` + "\n\n",
		`event: content_block_start` + "\n" + `data: {"type":"content_block_start","index":0,"content_block":{"type":"text"}}` + "\n\n",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hel"}}` + "\n\n",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"lo"}}` + "\n\n",
		`data: {"type":"content_block_stop","index":0}` + "\n\n",
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}` + "\n\n",
		`data: {"type":"message_stop"}` + "\n\n",
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["stream"] != true {
			t.Error("stream flag not forwarded to upstream")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		for _, e := range events {
			io.WriteString(w, e)
			fl.Flush()
		}
	}))
	gw, col := newTranslateGateway(t, upstream)

	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"m","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	out := string(body)

	// Output must be OpenAI SSE chunks, ending with [DONE].
	if !strings.Contains(out, "chat.completion.chunk") {
		t.Fatalf("not openai chunks:\n%s", out)
	}
	if !strings.Contains(out, "data: [DONE]") {
		t.Fatalf("missing [DONE]:\n%s", out)
	}

	// Reassemble content.
	var content strings.Builder
	for _, l := range strings.Split(out, "\n") {
		if !strings.HasPrefix(l, "data: ") || strings.Contains(l, "[DONE]") {
			continue
		}
		var c struct {
			Choices []struct {
				Delta struct {
					Content *string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		json.Unmarshal([]byte(strings.TrimPrefix(l, "data: ")), &c)
		if len(c.Choices) > 0 && c.Choices[0].Delta.Content != nil {
			content.WriteString(*c.Choices[0].Delta.Content)
		}
	}
	if content.String() != "Hello" {
		t.Errorf("reassembled content = %q", content.String())
	}

	ev := col.one(t)
	if ev.TokensIn != 5 || ev.TokensOut != 3 || !ev.Stream || ev.Status != hooks.StatusOK {
		t.Errorf("event: %+v", ev)
	}
}

func TestOpenAIToAnthropicToolCall(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		// Verify the tool definition survived translation to Anthropic shape.
		tools, _ := body["tools"].([]any)
		if len(tools) != 1 {
			t.Errorf("tools not translated: %v", body["tools"])
		} else if tools[0].(map[string]any)["input_schema"] == nil {
			t.Errorf("input_schema missing: %v", tools[0])
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"m","model":"x","content":[
			{"type":"tool_use","id":"tu1","name":"get_weather","input":{"city":"Paris"}}
		],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	gw, _ := newTranslateGateway(t, upstream)

	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{
		"model":"m","messages":[{"role":"user","content":"weather in Paris?"}],
		"tools":[{"type":"function","function":{"name":"get_weather","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var out struct {
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	json.Unmarshal(body, &out)
	tc := out.Choices[0].Message.ToolCalls
	if len(tc) != 1 || tc[0].Function.Name != "get_weather" || tc[0].ID != "tu1" {
		t.Fatalf("tool call not translated back: %s", body)
	}
	if out.Choices[0].FinishReason != "tool_calls" {
		t.Errorf("finish_reason = %s", out.Choices[0].FinishReason)
	}
}

func TestOpenAIToAnthropicErrorTranslation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"max_tokens required"}}`)
	}))
	gw, col := newTranslateGateway(t, upstream)

	resp, err := http.Post(gw.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"m","messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// Error must be re-shaped into the OpenAI envelope.
	var out struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &out) != nil || out.Error.Message != "max_tokens required" {
		t.Fatalf("error not translated to openai shape: %s", body)
	}
	if resp.StatusCode != 400 {
		t.Errorf("status = %d", resp.StatusCode)
	}
	ev := col.one(t)
	if ev.Status != hooks.StatusUpstreamError {
		t.Errorf("event status: %+v", ev)
	}
}

var _ = config.KindAnthropic
