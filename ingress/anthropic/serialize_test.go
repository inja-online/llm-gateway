package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestSerializeResponse(t *testing.T) {
	body, err := SerializeResponse(&canonical.Response{
		ID:         "msg_1",
		Model:      "gpt-4o",
		Content:    []canonical.Block{{Type: canonical.BlockText, Text: "hi"}},
		StopReason: canonical.StopEndTurn,
		Usage:      canonical.Usage{InputTokens: 3, OutputTokens: 1, HasUsage: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	var out messagesResponse
	json.Unmarshal(body, &out)
	if out.Type != "message" || out.Role != "assistant" || out.Model != "gpt-4o" {
		t.Errorf("envelope: %+v", out)
	}
	if out.Content[0].Type != "text" || out.Content[0].Text != "hi" {
		t.Errorf("content: %+v", out.Content)
	}
	if out.Usage.InputTokens != 3 || out.Usage.OutputTokens != 1 {
		t.Errorf("usage: %+v", out.Usage)
	}
}

func TestSerializeToolUse(t *testing.T) {
	body, _ := SerializeResponse(&canonical.Response{
		Model: "m",
		Content: []canonical.Block{
			{Type: canonical.BlockToolUse, ID: "tu1", Name: "f", Input: json.RawMessage(`{"a":1}`)},
		},
		StopReason: canonical.StopToolUse,
	})
	var out messagesResponse
	json.Unmarshal(body, &out)
	if out.StopReason != "tool_use" {
		t.Errorf("stop_reason: %s", out.StopReason)
	}
	b := out.Content[0]
	if b.Type != "tool_use" || b.ID != "tu1" || b.Name != "f" || string(b.Input) != `{"a":1}` {
		t.Errorf("tool_use block: %+v", b)
	}
}

func TestSerializeEmptyContentIsArray(t *testing.T) {
	// Clients index into content; it must never serialize as null.
	body, _ := SerializeResponse(&canonical.Response{Model: "m", StopReason: canonical.StopEndTurn})
	if !strings.Contains(string(body), `"content":[]`) {
		t.Errorf("empty content must be [], got: %s", body)
	}
}

func TestStreamSerializerSequence(t *testing.T) {
	s := NewStreamSerializer()
	var sb strings.Builder
	evs := []canonical.StreamEvent{
		{Type: canonical.EventStart, ID: "msg_1", Model: "m"},
		{Type: canonical.EventBlockStart, Index: 0, BlockType: canonical.BlockText},
		{Type: canonical.EventTextDelta, Index: 0, Text: "Hi"},
		{Type: canonical.EventBlockStop, Index: 0},
		{Type: canonical.EventFinish, StopReason: canonical.StopEndTurn,
			Usage: canonical.Usage{OutputTokens: 2, HasUsage: true}},
	}
	for _, ev := range evs {
		sb.Write(s.Event(ev))
	}
	out := sb.String()

	// Anthropic clients dispatch on the named event lines.
	for _, want := range []string{
		"event: message_start", "event: content_block_start",
		"event: content_block_delta", "event: content_block_stop",
		"event: message_delta", "event: message_stop",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if !strings.Contains(out, `"text":"Hi"`) {
		t.Errorf("text delta missing: %s", out)
	}
	if !strings.Contains(out, `"output_tokens":2`) {
		t.Errorf("output tokens missing: %s", out)
	}
}

func TestStreamSerializerToolUse(t *testing.T) {
	s := NewStreamSerializer()
	var sb strings.Builder
	for _, ev := range []canonical.StreamEvent{
		{Type: canonical.EventStart},
		{Type: canonical.EventBlockStart, Index: 0, BlockType: canonical.BlockToolUse, ToolID: "tu1", ToolName: "f"},
		{Type: canonical.EventJSONDelta, Index: 0, PartialJSON: `{"a":1}`},
		{Type: canonical.EventBlockStop, Index: 0},
		{Type: canonical.EventFinish, StopReason: canonical.StopToolUse},
	} {
		sb.Write(s.Event(ev))
	}
	out := sb.String()
	if !strings.Contains(out, `"type":"tool_use"`) || !strings.Contains(out, `"name":"f"`) {
		t.Errorf("tool_use block start missing: %s", out)
	}
	if !strings.Contains(out, `"type":"input_json_delta"`) || !strings.Contains(out, `"partial_json":"{\"a\":1}"`) {
		t.Errorf("input_json_delta missing: %s", out)
	}
	if !strings.Contains(out, `"stop_reason":"tool_use"`) {
		t.Errorf("stop_reason missing: %s", out)
	}
}
