package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mamad/llm-gateway/canonical"
)

func TestSerializeResponseText(t *testing.T) {
	resp := &canonical.Response{
		ID:         "msg_1",
		Model:      "claude-x",
		Content:    []canonical.Block{{Type: canonical.BlockText, Text: "hi there"}},
		StopReason: canonical.StopEndTurn,
		Usage:      canonical.Usage{InputTokens: 10, OutputTokens: 3, HasUsage: true},
	}
	body, err := SerializeResponse(resp, 1700000000)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	json.Unmarshal(body, &out)
	if out["object"] != "chat.completion" || out["model"] != "claude-x" {
		t.Errorf("envelope: %s", body)
	}
	choices := out["choices"].([]any)
	choice := choices[0].(map[string]any)
	msg := choice["message"].(map[string]any)
	if msg["content"] != "hi there" || choice["finish_reason"] != "stop" {
		t.Errorf("message: %v", choice)
	}
	u := out["usage"].(map[string]any)
	if u["prompt_tokens"].(float64) != 10 || u["total_tokens"].(float64) != 13 {
		t.Errorf("usage: %v", u)
	}
}

func TestSerializeResponseToolCall(t *testing.T) {
	resp := &canonical.Response{
		Model: "m",
		Content: []canonical.Block{
			{Type: canonical.BlockToolUse, ID: "tu_1", Name: "get_weather", Input: json.RawMessage(`{"city":"Paris"}`)},
		},
		StopReason: canonical.StopToolUse,
	}
	body, _ := SerializeResponse(resp, 0)
	var out chatResponse
	json.Unmarshal(body, &out)
	choice := out.Choices[0]
	if *choice.FinishReason != "tool_calls" {
		t.Errorf("finish: %v", *choice.FinishReason)
	}
	tc := choice.Message.ToolCalls[0]
	if tc.ID != "tu_1" || tc.Function.Name != "get_weather" || tc.Function.Arguments != `{"city":"Paris"}` {
		t.Errorf("tool_call: %+v", tc)
	}
}

func TestSerializeNoUsage(t *testing.T) {
	resp := &canonical.Response{Model: "m", StopReason: canonical.StopEndTurn,
		Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}}}
	body, _ := SerializeResponse(resp, 0)
	if strings.Contains(string(body), "usage") {
		t.Errorf("usage should be omitted when absent: %s", body)
	}
}

func TestStopReasonMapping(t *testing.T) {
	cases := map[string]string{
		canonical.StopEndTurn:      "stop",
		canonical.StopMaxTokens:    "length",
		canonical.StopToolUse:      "tool_calls",
		canonical.StopStopSequence: "stop",
		canonical.StopRefusal:      "content_filter",
	}
	for canon, want := range cases {
		if got := stopReasonToFinish(canon); got != want {
			t.Errorf("%s -> %s, want %s", canon, got, want)
		}
	}
}
