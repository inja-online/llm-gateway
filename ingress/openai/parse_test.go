package openai

import (
	"encoding/json"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestParseBasic(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model": "gpt-4o",
		"max_tokens": 100,
		"temperature": 0.5,
		"messages": [
			{"role": "system", "content": "be brief"},
			{"role": "user", "content": "hello"}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Model != "gpt-4o" || req.MaxTokens != 100 {
		t.Errorf("model/max: %+v", req)
	}
	if req.Temperature == nil || *req.Temperature != 0.5 {
		t.Errorf("temperature not parsed")
	}
	if len(req.System) != 1 || req.System[0].Text != "be brief" {
		t.Errorf("system: %+v", req.System)
	}
	if len(req.Messages) != 1 || req.Messages[0].Role != "user" ||
		req.Messages[0].Content[0].Text != "hello" {
		t.Errorf("messages: %+v", req.Messages)
	}
}

func TestParseMissingModel(t *testing.T) {
	_, err := ParseRequest([]byte(`{"messages":[]}`))
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("want ValidationError, got %v", err)
	}
}

func TestParseInvalidJSON(t *testing.T) {
	_, err := ParseRequest([]byte(`{bad`))
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("want ValidationError, got %v", err)
	}
}

func TestParseMaxCompletionTokens(t *testing.T) {
	req, err := ParseRequest([]byte(`{"model":"m","max_completion_tokens":42,"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.MaxTokens != 42 {
		t.Errorf("max_completion_tokens not mapped: %d", req.MaxTokens)
	}
	if req.MaxTokensField != canonical.MaxTokensFieldMaxCompletionTokens {
		t.Errorf("MaxTokensField: %q", req.MaxTokensField)
	}
}

func TestParseStopStringAndArray(t *testing.T) {
	req, _ := ParseRequest([]byte(`{"model":"m","stop":"END","messages":[]}`))
	if len(req.StopSequences) != 1 || req.StopSequences[0] != "END" {
		t.Errorf("stop string: %+v", req.StopSequences)
	}
	req, _ = ParseRequest([]byte(`{"model":"m","stop":["A","B"],"messages":[]}`))
	if len(req.StopSequences) != 2 {
		t.Errorf("stop array: %+v", req.StopSequences)
	}
}

func TestParseMultimodalContent(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"m","messages":[{"role":"user","content":[
			{"type":"text","text":"look"},
			{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}}
		]}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	blocks := req.Messages[0].Content
	if len(blocks) != 2 {
		t.Fatalf("want 2 blocks, got %d", len(blocks))
	}
	if blocks[0].Type != canonical.BlockText || blocks[0].Text != "look" {
		t.Errorf("text block: %+v", blocks[0])
	}
	img := blocks[1]
	if img.Type != canonical.BlockImage || img.Image.Kind != "base64" ||
		img.Image.MediaType != "image/png" || img.Image.Data != "AAAA" {
		t.Errorf("image block: %+v", img.Image)
	}
}

func TestParseRemoteImageURL(t *testing.T) {
	req, _ := ParseRequest([]byte(`{"model":"m","messages":[{"role":"user","content":[
		{"type":"image_url","image_url":{"url":"https://ex.com/x.png"}}]}]}`))
	img := req.Messages[0].Content[0].Image
	if img.Kind != "url" || img.Data != "https://ex.com/x.png" {
		t.Errorf("remote image: %+v", img)
	}
}

func TestParseAssistantToolCalls(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"m","messages":[
			{"role":"user","content":"weather?"},
			{"role":"assistant","content":null,"tool_calls":[
				{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Paris\"}"}}
			]},
			{"role":"tool","tool_call_id":"call_1","content":"18C"}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	// user, assistant(tool_use), user(tool_result)
	if len(req.Messages) != 3 {
		t.Fatalf("want 3 turns, got %d: %+v", len(req.Messages), req.Messages)
	}
	tu := req.Messages[1].Content[0]
	if tu.Type != canonical.BlockToolUse || tu.ID != "call_1" || tu.Name != "get_weather" {
		t.Errorf("tool_use: %+v", tu)
	}
	var args map[string]string
	json.Unmarshal(tu.Input, &args)
	if args["city"] != "Paris" {
		t.Errorf("tool args: %s", tu.Input)
	}
	tr := req.Messages[2].Content[0]
	if req.Messages[2].Role != canonical.RoleUser || tr.Type != canonical.BlockToolResult ||
		tr.ToolUseID != "call_1" || tr.Result != "18C" {
		t.Errorf("tool_result: role=%s %+v", req.Messages[2].Role, tr)
	}
}

func TestParseConsecutiveToolResultsGroup(t *testing.T) {
	// Two tool messages in a row must collapse into one user turn.
	req, _ := ParseRequest([]byte(`{
		"model":"m","messages":[
			{"role":"tool","tool_call_id":"a","content":"1"},
			{"role":"tool","tool_call_id":"b","content":"2"}
		]
	}`))
	if len(req.Messages) != 1 {
		t.Fatalf("want 1 grouped turn, got %d", len(req.Messages))
	}
	if len(req.Messages[0].Content) != 2 {
		t.Errorf("want 2 tool_result blocks, got %d", len(req.Messages[0].Content))
	}
}

func TestParseToolChoice(t *testing.T) {
	cases := []struct {
		raw  string
		mode canonical.ToolChoiceMode
		name string
	}{
		{`"auto"`, canonical.ToolAuto, ""},
		{`"none"`, canonical.ToolNone, ""},
		{`"required"`, canonical.ToolRequired, ""},
		{`{"type":"function","function":{"name":"f"}}`, canonical.ToolSpecific, "f"},
	}
	for _, c := range cases {
		req, err := ParseRequest([]byte(`{"model":"m","tool_choice":` + c.raw + `,"messages":[]}`))
		if err != nil {
			t.Fatalf("%s: %v", c.raw, err)
		}
		if req.ToolChoice.Mode != c.mode || req.ToolChoice.Name != c.name {
			t.Errorf("%s -> %+v", c.raw, req.ToolChoice)
		}
	}
}

func TestParseToolsDefinition(t *testing.T) {
	req, _ := ParseRequest([]byte(`{"model":"m","tools":[
		{"type":"function","function":{"name":"f","description":"d","parameters":{"type":"object"}}}
	],"messages":[]}`))
	if len(req.Tools) != 1 || req.Tools[0].Name != "f" || req.Tools[0].Description != "d" {
		t.Errorf("tools: %+v", req.Tools)
	}
}
