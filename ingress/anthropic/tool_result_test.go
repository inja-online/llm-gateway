package anthropic

import (
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestParseToolResultMultimodal(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"claude-x","max_tokens":10,
		"messages":[{
			"role":"user",
			"content":[{
				"type":"tool_result",
				"tool_use_id":"tu1",
				"content":[
					{"type":"text","text":"caption"},
					{"type":"image","source":{"type":"base64","media_type":"image/png","data":"QQ=="}}
				]
			}]
		}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	b := req.Messages[0].Content[0]
	if b.Type != canonical.BlockToolResult || b.ToolUseID != "tu1" {
		t.Fatalf("%+v", b)
	}
	if b.Result != "caption" {
		t.Fatalf("result text: %q", b.Result)
	}
	if len(b.ResultBlocks) != 2 {
		t.Fatalf("result blocks: %+v", b.ResultBlocks)
	}
	if b.ResultBlocks[1].Type != canonical.BlockImage || b.ResultBlocks[1].Image.Data != "QQ==" {
		t.Fatalf("image: %+v", b.ResultBlocks[1])
	}
}

func TestParseToolResultString(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"m","max_tokens":1,
		"messages":[{"role":"user","content":[
			{"type":"tool_result","tool_use_id":"t","content":"ok"}
		]}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	b := req.Messages[0].Content[0]
	if b.Result != "ok" || len(b.ResultBlocks) != 0 {
		t.Fatalf("%+v", b)
	}
}
