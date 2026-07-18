package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestValidationErrorString(t *testing.T) {
	if (&ValidationError{Msg: "x"}).Error() != "x" {
		t.Fatal()
	}
}

func TestParseInvalidJSON(t *testing.T) {
	_, err := ParseRequest([]byte(`{`))
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("%v", err)
	}
}

func TestParseBadSystem(t *testing.T) {
	_, err := ParseRequest([]byte(`{"model":"m","max_tokens":1,"system":123,"messages":[]}`))
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("%v", err)
	}
}

func TestParseEmptySystemString(t *testing.T) {
	req, err := ParseRequest([]byte(`{"model":"m","max_tokens":1,"system":"","messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(req.System) != 0 {
		t.Fatalf("%+v", req.System)
	}
}

func TestParseContentBlocksAndTools(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"m","max_tokens":10,
		"tools":[{"name":"f","description":"d","input_schema":{"type":"object"}}],
		"tool_choice":{"type":"tool","name":"f"},
		"messages":[{"role":"user","content":[
			{"type":"text","text":"hi"},
			{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AA"}},
			{"type":"tool_result","tool_use_id":"t1","content":"ok","is_error":false}
		]}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Tools) != 1 || req.ToolChoice == nil || req.ToolChoice.Name != "f" {
		t.Fatalf("tools/choice %+v %+v", req.Tools, req.ToolChoice)
	}
	if len(req.Messages[0].Content) < 2 {
		t.Fatalf("%+v", req.Messages[0].Content)
	}
}

func TestParseToolChoiceModeVariants(t *testing.T) {
	for _, raw := range []string{
		`{"type":"auto"}`,
		`{"type":"none"}`,
		`{"type":"any"}`,
	} {
		req, err := ParseRequest([]byte(`{"model":"m","max_tokens":1,"tool_choice":` + raw + `,"messages":[]}`))
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if req.ToolChoice == nil {
			t.Fatalf("%s nil", raw)
		}
	}
	_, err := ParseRequest([]byte(`{"model":"m","max_tokens":1,"tool_choice":{"type":"nope"},"messages":[]}`))
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("%v", err)
	}
}

func TestParseBadContent(t *testing.T) {
	_, err := ParseRequest([]byte(`{"model":"m","max_tokens":1,"messages":[{"role":"user","content":1}]}`))
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("%v", err)
	}
}

func TestSerializeThinkingAndEmptyToolInput(t *testing.T) {
	body, err := SerializeResponse(&canonical.Response{
		Model: "m",
		Content: []canonical.Block{
			{Type: canonical.BlockThinking, Text: "think", Signature: "sig"},
			{Type: canonical.BlockToolUse, ID: "t", Name: "f"},
			{Type: canonical.BlockImage}, // skipped
		},
		StopReason: "",
	})
	if err != nil {
		t.Fatal(err)
	}
	var out messagesResponse
	json.Unmarshal(body, &out)
	if out.StopReason != canonical.StopEndTurn {
		t.Fatalf("stop %s", out.StopReason)
	}
	if out.ID != "msg_gateway" {
		t.Fatalf("id %s", out.ID)
	}
	if len(out.Content) != 2 {
		t.Fatalf("%+v", out.Content)
	}
	if out.Content[0].Type != "thinking" || out.Content[1].Type != "tool_use" {
		t.Fatalf("%+v", out.Content)
	}
	if string(out.Content[1].Input) != `{}` {
		t.Fatalf("input %s", out.Content[1].Input)
	}
}

func TestStreamSerializerThinkingAndEmptyID(t *testing.T) {
	s := NewStreamSerializer()
	out := string(s.Event(canonical.StreamEvent{Type: canonical.EventStart}))
	if !strings.Contains(out, "msg_gateway") {
		t.Fatalf("%s", out)
	}
	out = string(s.Event(canonical.StreamEvent{
		Type: canonical.EventBlockStart, BlockType: canonical.BlockThinking, Index: 0,
	}))
	if !strings.Contains(out, `"type":"thinking"`) {
		t.Fatalf("%s", out)
	}
	out = string(s.Event(canonical.StreamEvent{
		Type: canonical.EventThinkingDelta, Index: 0, Text: "r",
	}))
	if !strings.Contains(out, "thinking_delta") {
		t.Fatalf("%s", out)
	}
	// unknown event type
	if s.Event(canonical.StreamEvent{Type: 99}) != nil {
		t.Fatal("want nil")
	}
}
