package google

import (
	"encoding/json"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestParseMissingContents(t *testing.T) {
	if _, err := ParseRequest([]byte(`{"model":"m"}`), ""); err == nil {
		t.Fatal("expected error")
	}
	if _, err := ParseRequest([]byte(`not-json`), "m"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := ParseRequest([]byte(`{"contents":[]}`), ""); err == nil {
		t.Fatal("expected model error")
	}
}

func TestParseToolModesAndParts(t *testing.T) {
	body := []byte(`{
		"contents":[
			{"role":"model","parts":[
				{"text":"thinking","thought":true},
				{"function_call":{"name":"fn","args":{"a":1}}},
				{"inline_data":{"mime_type":"image/png","data":"abc"}}
			]},
			{"role":"user","parts":[
				{"function_response":{"name":"fn","response":{"ok":true}}}
			]}
		],
		"tools":[{"function_declarations":[{"name":"fn","description":"d","parameters":{"type":"object"}}]}],
		"tool_config":{"function_calling_config":{"mode":"ANY","allowed_function_names":["fn"]}}
	}`)
	req, err := ParseRequest(body, "m")
	if err != nil {
		t.Fatal(err)
	}
	if req.ToolChoice == nil || req.ToolChoice.Mode != canonical.ToolSpecific || req.ToolChoice.Name != "fn" {
		t.Fatalf("%+v", req.ToolChoice)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("%d msgs", len(req.Messages))
	}
	// NONE mode
	body2 := []byte(`{"contents":[{"parts":[{"text":"x"}]}],"tool_config":{"function_calling_config":{"mode":"NONE"}}}`)
	req2, err := ParseRequest(body2, "m")
	if err != nil {
		t.Fatal(err)
	}
	if req2.ToolChoice.Mode != canonical.ToolNone {
		t.Fatal(req2.ToolChoice.Mode)
	}
	// ANY without allowed names
	body3 := []byte(`{"contents":[{"parts":[{"text":"x"}]}],"tool_config":{"function_calling_config":{"mode":"ANY"}}}`)
	req3, err := ParseRequest(body3, "m")
	if err != nil {
		t.Fatal(err)
	}
	if req3.ToolChoice.Mode != canonical.ToolRequired {
		t.Fatal(req3.ToolChoice.Mode)
	}
	// bad mode
	if _, err := ParseRequest([]byte(`{"contents":[{"parts":[{"text":"x"}]}],"tool_config":{"function_calling_config":{"mode":"NOPE"}}}`), "m"); err == nil {
		t.Fatal("expected bad mode")
	}
	// empty function name
	if _, err := ParseRequest([]byte(`{"contents":[{"parts":[{"text":"x"}]}],"tools":[{"function_declarations":[{"name":""}]}]}`), "m"); err == nil {
		t.Fatal("expected name error")
	}
}

func TestSerializeVariants(t *testing.T) {
	out, err := SerializeResponse(&canonical.Response{
		Content: []canonical.Block{
			{Type: canonical.BlockThinking, Text: "t"},
			{Type: canonical.BlockToolUse, Name: "fn", Input: json.RawMessage(`{"x":1}`)},
		},
		StopReason: canonical.StopMaxTokens,
	})
	if err != nil {
		t.Fatal(err)
	}
	var env generateResponse
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatal(err)
	}
	if env.Candidates[0].FinishReason != "MAX_TOKENS" {
		t.Fatal(env.Candidates[0].FinishReason)
	}
	for _, sr := range []string{canonical.StopToolUse, canonical.StopRefusal, "other"} {
		b, _ := SerializeResponse(&canonical.Response{Content: []canonical.Block{}, StopReason: sr})
		if len(b) == 0 {
			t.Fatal(sr)
		}
	}
	if responseID("") != "resp_gateway" {
		t.Fatal(responseID(""))
	}
}

func TestStreamSerializer(t *testing.T) {
	s := NewStreamSerializer()
	if s.Event(canonical.StreamEvent{Type: canonical.EventStart, ID: "id", Model: "m"}) != nil {
		// start is silent
	}
	if out := s.Event(canonical.StreamEvent{Type: canonical.EventTextDelta, Text: "hi"}); out == nil {
		t.Fatal("text")
	}
	if out := s.Event(canonical.StreamEvent{Type: canonical.EventThinkingDelta, Text: "th"}); out == nil {
		t.Fatal("think")
	}
	if out := s.Event(canonical.StreamEvent{Type: canonical.EventBlockStart, BlockType: canonical.BlockToolUse, ToolName: "fn"}); out == nil {
		t.Fatal("tool start")
	}
	if out := s.Event(canonical.StreamEvent{Type: canonical.EventJSONDelta, PartialJSON: `{"a":1}`}); out == nil {
		t.Fatal("json")
	}
	if out := s.Event(canonical.StreamEvent{Type: canonical.EventBlockStart, BlockType: canonical.BlockText}); out != nil {
		t.Fatal("text block start silent")
	}
	if out := s.Event(canonical.StreamEvent{
		Type: canonical.EventFinish, StopReason: canonical.StopEndTurn,
		Usage: canonical.Usage{InputTokens: 1, OutputTokens: 2, HasUsage: true},
	}); out == nil {
		t.Fatal("finish")
	}
	_ = (&ValidationError{Msg: "x"}).Error()
}
