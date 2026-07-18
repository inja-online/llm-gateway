package anthropic

import (
	"testing"

	"github.com/mamad/llm-gateway/canonical"
)

func TestParseBasic(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"claude-sonnet-5","max_tokens":1024,
		"system":"be brief",
		"messages":[{"role":"user","content":"hello"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Model != "claude-sonnet-5" || req.MaxTokens != 1024 {
		t.Errorf("envelope: %+v", req)
	}
	if len(req.System) != 1 || req.System[0].Text != "be brief" {
		t.Errorf("system: %+v", req.System)
	}
	if req.Messages[0].Content[0].Text != "hello" {
		t.Errorf("messages: %+v", req.Messages)
	}
}

func TestParseSystemAsBlocks(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"m","max_tokens":1,
		"system":[{"type":"text","text":"a"},{"type":"text","text":"b"}],
		"messages":[]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(req.System) != 2 || req.System[1].Text != "b" {
		t.Errorf("system blocks: %+v", req.System)
	}
}

func TestParseMissingMaxTokens(t *testing.T) {
	_, err := ParseRequest([]byte(`{"model":"m","messages":[]}`))
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("max_tokens is required by the Anthropic API; want ValidationError, got %v", err)
	}
}

func TestParseMissingModel(t *testing.T) {
	_, err := ParseRequest([]byte(`{"max_tokens":1,"messages":[]}`))
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("want ValidationError, got %v", err)
	}
}

func TestParseToolUseAndResult(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"m","max_tokens":1,
		"messages":[
			{"role":"assistant","content":[
				{"type":"tool_use","id":"tu1","name":"f","input":{"a":1}}
			]},
			{"role":"user","content":[
				{"type":"tool_result","tool_use_id":"tu1","content":"42"}
			]}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	tu := req.Messages[0].Content[0]
	if tu.Type != canonical.BlockToolUse || tu.ID != "tu1" || string(tu.Input) != `{"a":1}` {
		t.Errorf("tool_use: %+v", tu)
	}
	tr := req.Messages[1].Content[0]
	if tr.Type != canonical.BlockToolResult || tr.ToolUseID != "tu1" || tr.Result != "42" {
		t.Errorf("tool_result: %+v", tr)
	}
}

func TestParseToolResultBlockArray(t *testing.T) {
	// tool_result content may be an array of text blocks rather than a string.
	req, _ := ParseRequest([]byte(`{
		"model":"m","max_tokens":1,
		"messages":[{"role":"user","content":[
			{"type":"tool_result","tool_use_id":"t","content":[{"type":"text","text":"ok"}]}
		]}]
	}`))
	if got := req.Messages[0].Content[0].Result; got != "ok" {
		t.Errorf("flattened tool_result = %q", got)
	}
}

func TestParseImage(t *testing.T) {
	req, _ := ParseRequest([]byte(`{
		"model":"m","max_tokens":1,
		"messages":[{"role":"user","content":[
			{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}}
		]}]
	}`))
	img := req.Messages[0].Content[0].Image
	if img == nil || img.Kind != "base64" || img.MediaType != "image/png" || img.Data != "AAAA" {
		t.Errorf("image: %+v", img)
	}
}

func TestParseToolChoiceModes(t *testing.T) {
	cases := map[string]canonical.ToolChoiceMode{
		`{"type":"auto"}`:            canonical.ToolAuto,
		`{"type":"none"}`:            canonical.ToolNone,
		`{"type":"any"}`:             canonical.ToolRequired,
		`{"type":"tool","name":"f"}`: canonical.ToolSpecific,
	}
	for raw, want := range cases {
		req, err := ParseRequest([]byte(`{"model":"m","max_tokens":1,"tool_choice":` + raw + `,"messages":[]}`))
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if req.ToolChoice.Mode != want {
			t.Errorf("%s -> %v, want %v", raw, req.ToolChoice.Mode, want)
		}
	}
}

func TestParseTools(t *testing.T) {
	req, _ := ParseRequest([]byte(`{
		"model":"m","max_tokens":1,
		"tools":[{"name":"f","description":"d","input_schema":{"type":"object"}}],
		"messages":[]
	}`))
	if len(req.Tools) != 1 || req.Tools[0].Name != "f" || string(req.Tools[0].Schema) != `{"type":"object"}` {
		t.Errorf("tools: %+v", req.Tools)
	}
}
