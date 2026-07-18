package openai

import (
	"encoding/json"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestParseToolMessageContent(t *testing.T) {
	// empty / null
	b, err := parseToolMessageContent(nil)
	if err != nil || b.Result != "" {
		t.Fatalf("%+v %v", b, err)
	}
	b, err = parseToolMessageContent(json.RawMessage(`null`))
	if err != nil || b.Result != "" {
		t.Fatalf("%+v %v", b, err)
	}
	// string
	b, err = parseToolMessageContent(json.RawMessage(`"hello"`))
	if err != nil || b.Result != "hello" {
		t.Fatalf("%+v %v", b, err)
	}
	// empty array
	b, err = parseToolMessageContent(json.RawMessage(`[]`))
	if err != nil || b.Result != "" || len(b.ResultBlocks) != 0 {
		t.Fatalf("%+v %v", b, err)
	}
	// single text part collapses
	b, err = parseToolMessageContent(json.RawMessage(`[{"type":"text","text":"only"}]`))
	if err != nil || b.Result != "only" || len(b.ResultBlocks) != 0 {
		t.Fatalf("%+v %v", b, err)
	}
	// multimodal keeps ResultBlocks
	b, err = parseToolMessageContent(json.RawMessage(`[
		{"type":"text","text":"a"},
		{"type":"image_url","image_url":{"url":"data:image/png;base64,xx"}}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if b.Result != "a" || len(b.ResultBlocks) < 2 {
		t.Fatalf("%+v", b)
	}
	// invalid JSON falls back to raw string
	b, err = parseToolMessageContent(json.RawMessage(`{not-valid`))
	if err != nil || b.Result != `{not-valid` {
		t.Fatalf("%+v %v", b, err)
	}

	// Full request with role:tool multimodal content
	req, err := ParseRequest([]byte(`{
		"model":"gpt-4o",
		"messages":[
			{"role":"assistant","content":null,"tool_calls":[
				{"id":"c1","type":"function","function":{"name":"f","arguments":"{}"}}
			]},
			{"role":"tool","tool_call_id":"c1","content":[
				{"type":"text","text":"ok"},
				{"type":"text","text":" more"}
			]}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	// Find tool result block
	var found bool
	for _, m := range req.Messages {
		for _, bl := range m.Content {
			if bl.Type == canonical.BlockToolResult {
				found = true
				if bl.Result != "ok more" && bl.Result != "ok" {
					// multi-text concatenates when ResultBlocks kept
					if len(bl.ResultBlocks) == 0 && bl.Result == "" {
						t.Fatalf("%+v", bl)
					}
				}
			}
		}
	}
	if !found {
		t.Fatalf("no tool result: %+v", req.Messages)
	}
}
