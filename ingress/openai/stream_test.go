package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

// collect runs a sequence of canonical events through the serializer and
// returns the concatenated SSE output including the [DONE] sentinel.
func collect(s *StreamSerializer, evs []canonical.StreamEvent) string {
	var sb strings.Builder
	for _, ev := range evs {
		if b := s.Event(ev); b != nil {
			sb.Write(b)
		}
	}
	sb.Write(s.Done())
	return sb.String()
}

func dataLines(out string) []string {
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "data: ") {
			lines = append(lines, strings.TrimPrefix(l, "data: "))
		}
	}
	return lines
}

func TestStreamText(t *testing.T) {
	s := NewStreamSerializer(1700000000)
	out := collect(s, []canonical.StreamEvent{
		{Type: canonical.EventStart, ID: "msg_1", Model: "claude-x"},
		{Type: canonical.EventBlockStart, Index: 0, BlockType: canonical.BlockText},
		{Type: canonical.EventTextDelta, Index: 0, Text: "Hel"},
		{Type: canonical.EventTextDelta, Index: 0, Text: "lo"},
		{Type: canonical.EventBlockStop, Index: 0},
		{Type: canonical.EventFinish, StopReason: canonical.StopEndTurn,
			Usage: canonical.Usage{InputTokens: 5, OutputTokens: 2, HasUsage: true}},
	})
	lines := dataLines(out)
	if lines[len(lines)-1] != "[DONE]" {
		t.Fatalf("missing [DONE]: %q", out)
	}

	// Reassemble content from delta chunks.
	var content strings.Builder
	var finish string
	var sawUsage bool
	for _, l := range lines[:len(lines)-1] {
		var c chatResponse
		if json.Unmarshal([]byte(l), &c) != nil {
			t.Fatalf("bad chunk: %s", l)
		}
		if c.Object != "chat.completion.chunk" {
			t.Errorf("object: %s", c.Object)
		}
		if len(c.Choices) > 0 && c.Choices[0].Delta != nil && c.Choices[0].Delta.Content != nil {
			content.WriteString(*c.Choices[0].Delta.Content)
		}
		if len(c.Choices) > 0 && c.Choices[0].FinishReason != nil {
			finish = *c.Choices[0].FinishReason
		}
		if c.Usage != nil {
			sawUsage = true
		}
	}
	if content.String() != "Hello" {
		t.Errorf("content = %q", content.String())
	}
	if finish != "stop" {
		t.Errorf("finish = %q", finish)
	}
	if !sawUsage {
		t.Error("usage not emitted in final chunk")
	}
}

func TestStreamToolCall(t *testing.T) {
	s := NewStreamSerializer(0)
	out := collect(s, []canonical.StreamEvent{
		{Type: canonical.EventStart, ID: "m", Model: "x"},
		{Type: canonical.EventBlockStart, Index: 0, BlockType: canonical.BlockToolUse, ToolID: "tu_1", ToolName: "search"},
		{Type: canonical.EventJSONDelta, Index: 0, PartialJSON: `{"q":`},
		{Type: canonical.EventJSONDelta, Index: 0, PartialJSON: `"go"}`},
		{Type: canonical.EventBlockStop, Index: 0},
		{Type: canonical.EventFinish, StopReason: canonical.StopToolUse},
	})
	var name, args string
	var idx = -1
	for _, l := range dataLines(out) {
		if l == "[DONE]" {
			continue
		}
		var c chatResponse
		json.Unmarshal([]byte(l), &c)
		if len(c.Choices) == 0 || c.Choices[0].Delta == nil {
			continue
		}
		for _, tc := range c.Choices[0].Delta.ToolCalls {
			if tc.Function.Name != "" {
				name = tc.Function.Name
				idx = tc.Index
			}
			args += tc.Function.Arguments
		}
	}
	if name != "search" || idx != 0 {
		t.Errorf("tool name/index: %q %d", name, idx)
	}
	if args != `{"q":"go"}` {
		t.Errorf("reassembled args = %q", args)
	}
}

func TestStreamTwoToolCallsDistinctIndexes(t *testing.T) {
	s := NewStreamSerializer(0)
	out := collect(s, []canonical.StreamEvent{
		{Type: canonical.EventStart},
		{Type: canonical.EventBlockStart, Index: 0, BlockType: canonical.BlockToolUse, ToolID: "a", ToolName: "f1"},
		{Type: canonical.EventJSONDelta, Index: 0, PartialJSON: `{}`},
		{Type: canonical.EventBlockStop, Index: 0},
		{Type: canonical.EventBlockStart, Index: 1, BlockType: canonical.BlockToolUse, ToolID: "b", ToolName: "f2"},
		{Type: canonical.EventJSONDelta, Index: 1, PartialJSON: `{}`},
		{Type: canonical.EventBlockStop, Index: 1},
		{Type: canonical.EventFinish, StopReason: canonical.StopToolUse},
	})
	seen := map[int]string{}
	for _, l := range dataLines(out) {
		if l == "[DONE]" {
			continue
		}
		var c chatResponse
		json.Unmarshal([]byte(l), &c)
		if len(c.Choices) == 0 || c.Choices[0].Delta == nil {
			continue
		}
		for _, tc := range c.Choices[0].Delta.ToolCalls {
			if tc.Function.Name != "" {
				seen[tc.Index] = tc.Function.Name
			}
		}
	}
	if seen[0] != "f1" || seen[1] != "f2" {
		t.Errorf("tool ordinals wrong: %v", seen)
	}
}
