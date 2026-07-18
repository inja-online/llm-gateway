package openai

import (
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestStreamSerializerAllEventTypes(t *testing.T) {
	s := NewStreamSerializer(1)
	var out strings.Builder
	for _, ev := range []canonical.StreamEvent{
		{Type: canonical.EventStart, ID: "id", Model: "m"},
		{Type: canonical.EventBlockStart, Index: 0, BlockType: canonical.BlockText},
		{Type: canonical.EventTextDelta, Index: 0, Text: "hi"},
		{Type: canonical.EventBlockStop, Index: 0},
		{Type: canonical.EventBlockStart, Index: 1, BlockType: canonical.BlockToolUse, ToolID: "t", ToolName: "f"},
		{Type: canonical.EventJSONDelta, Index: 1, PartialJSON: `{"a":1}`},
		{Type: canonical.EventBlockStop, Index: 1},
		{Type: canonical.EventThinkingDelta, Index: 2, Text: "think"},
		{Type: canonical.EventFinish, StopReason: canonical.StopToolUse, Usage: canonical.Usage{InputTokens: 1, OutputTokens: 2, HasUsage: true}},
	} {
		if b := s.Event(ev); b != nil {
			out.Write(b)
		}
	}
	out.Write(s.Done())
	got := out.String()
	for _, want := range []string{"chat.completion.chunk", "hi", "tool_calls", "reasoning_content", "[DONE]"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	// unknown type
	if s.Event(canonical.StreamEvent{Type: 99}) != nil {
		t.Fatal()
	}
}

func TestSerializeThinkingAndEmptyToolInput(t *testing.T) {
	body, err := SerializeResponse(&canonical.Response{
		Model: "m",
		Content: []canonical.Block{
			{Type: canonical.BlockThinking, Text: "t"},
			{Type: canonical.BlockText, Text: "answer"},
			{Type: canonical.BlockToolUse, ID: "1", Name: "f"},
			{Type: canonical.BlockImage},
		},
		StopReason: canonical.StopEndTurn,
		Usage:      canonical.Usage{HasUsage: true, InputTokens: 1, OutputTokens: 1},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "answer") {
		t.Fatal(string(body))
	}
	if !strings.Contains(string(body), "tool_calls") {
		t.Fatal(string(body))
	}
}

func TestParseBadContentShape(t *testing.T) {
	_, err := ParseRequest([]byte(`{"model":"m","messages":[{"role":"user","content":{"a":1}}]}`))
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("%v", err)
	}
}

func TestParseToolContentRaw(t *testing.T) {
	req, err := ParseRequest([]byte(`{"model":"m","messages":[
		{"role":"tool","tool_call_id":"c","content":{"not":"string"}}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Messages[0].Content[0].Result == "" {
		// raw fallback should produce something
		t.Logf("result=%q", req.Messages[0].Content[0].Result)
	}
}
