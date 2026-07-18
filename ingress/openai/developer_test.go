package openai

import (
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestParseDeveloperAndEmptyContent(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"gpt-4o",
		"messages":[
			{"role":"developer","content":"sys-dev"},
			{"role":"assistant","content":null,"tool_calls":[
				{"id":"c1","type":"function","function":{"name":"f","arguments":""}}
			]},
			{"role":"user","content":""}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(req.System) != 1 || req.System[0].Text != "sys-dev" {
		t.Fatalf("system %+v", req.System)
	}
	// empty user content string → no blocks
	// assistant null content + tool_calls with empty args → {}
	var sawTool bool
	for _, m := range req.Messages {
		if m.Role == canonical.RoleAssistant {
			for _, b := range m.Content {
				if b.Type == canonical.BlockToolUse {
					sawTool = true
					if string(b.Input) != "{}" {
						t.Fatalf("args %s", b.Input)
					}
				}
			}
		}
	}
	if !sawTool {
		t.Fatal("expected tool_use")
	}
}

func TestParseContentBlocksEmptyString(t *testing.T) {
	blocks, err := parseContentBlocks([]byte(`""`))
	if err != nil || len(blocks) != 0 {
		t.Fatalf("%v %v", blocks, err)
	}
}
