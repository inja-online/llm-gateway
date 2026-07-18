package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/mamad/llm-gateway/canonical"
)

func TestBuildBasic(t *testing.T) {
	req := &canonical.Request{
		System:   []canonical.Block{{Type: canonical.BlockText, Text: "sys"}},
		Messages: []canonical.Message{{Role: canonical.RoleUser, Content: []canonical.Block{{Type: canonical.BlockText, Text: "hi"}}}},
	}
	body, err := BuildRequest(req, "claude-sonnet-5")
	if err != nil {
		t.Fatal(err)
	}
	var out messagesRequest
	json.Unmarshal(body, &out)
	if out.Model != "claude-sonnet-5" {
		t.Errorf("model: %s", out.Model)
	}
	if out.MaxTokens != defaultMaxTokens {
		t.Errorf("max_tokens default not applied: %d", out.MaxTokens)
	}
	if len(out.System) != 1 || out.System[0].Text != "sys" {
		t.Errorf("system: %+v", out.System)
	}
	if len(out.Messages) != 1 || out.Messages[0].Content[0].Text != "hi" {
		t.Errorf("messages: %+v", out.Messages)
	}
}

func TestBuildToolUseAndResult(t *testing.T) {
	req := &canonical.Request{
		MaxTokens: 50,
		Messages: []canonical.Message{
			{Role: canonical.RoleAssistant, Content: []canonical.Block{
				{Type: canonical.BlockToolUse, ID: "tu1", Name: "f", Input: json.RawMessage(`{"a":1}`)},
			}},
			{Role: canonical.RoleUser, Content: []canonical.Block{
				{Type: canonical.BlockToolResult, ToolUseID: "tu1", Result: "42"},
			}},
		},
	}
	body, _ := BuildRequest(req, "m")
	var out messagesRequest
	json.Unmarshal(body, &out)

	tu := out.Messages[0].Content[0]
	if tu.Type != "tool_use" || tu.ID != "tu1" || tu.Name != "f" || string(tu.Input) != `{"a":1}` {
		t.Errorf("tool_use: %+v", tu)
	}
	tr := out.Messages[1].Content[0]
	if tr.Type != "tool_result" || tr.ToolUseID != "tu1" {
		t.Errorf("tool_result: %+v", tr)
	}
	var result string
	json.Unmarshal(tr.Content, &result)
	if result != "42" {
		t.Errorf("tool_result content = %q", result)
	}
}

func TestBuildToolChoice(t *testing.T) {
	cases := []struct {
		tc   *canonical.ToolChoice
		want string
	}{
		{&canonical.ToolChoice{Mode: canonical.ToolAuto}, `{"type":"auto"}`},
		{&canonical.ToolChoice{Mode: canonical.ToolNone}, `{"type":"none"}`},
		{&canonical.ToolChoice{Mode: canonical.ToolRequired}, `{"type":"any"}`},
		{&canonical.ToolChoice{Mode: canonical.ToolSpecific, Name: "f"}, `{"name":"f","type":"tool"}`},
	}
	for _, c := range cases {
		req := &canonical.Request{ToolChoice: c.tc, Messages: []canonical.Message{
			{Role: canonical.RoleUser, Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}}}}}
		body, _ := BuildRequest(req, "m")
		var out struct {
			ToolChoice json.RawMessage `json:"tool_choice"`
		}
		json.Unmarshal(body, &out)
		if string(out.ToolChoice) != c.want {
			t.Errorf("%v -> %s, want %s", c.tc.Mode, out.ToolChoice, c.want)
		}
	}
}

func TestBuildDropsEmptyTurns(t *testing.T) {
	req := &canonical.Request{Messages: []canonical.Message{
		{Role: canonical.RoleUser, Content: []canonical.Block{{Type: canonical.BlockText, Text: ""}}}, // empty
		{Role: canonical.RoleUser, Content: []canonical.Block{{Type: canonical.BlockText, Text: "real"}}},
	}}
	body, _ := BuildRequest(req, "m")
	var out messagesRequest
	json.Unmarshal(body, &out)
	if len(out.Messages) != 1 {
		t.Errorf("empty turn not dropped: %+v", out.Messages)
	}
}

func TestBuildImageBase64(t *testing.T) {
	req := &canonical.Request{Messages: []canonical.Message{{Role: canonical.RoleUser, Content: []canonical.Block{
		{Type: canonical.BlockImage, Image: &canonical.ImageSource{Kind: "base64", MediaType: "image/png", Data: "AAAA"}},
	}}}}
	body, _ := BuildRequest(req, "m")
	var out messagesRequest
	json.Unmarshal(body, &out)
	src := out.Messages[0].Content[0].Source
	if src == nil || src.Type != "base64" || src.MediaType != "image/png" || src.Data != "AAAA" {
		t.Errorf("image source: %+v", src)
	}
}

func TestBuildToolSchemaDefault(t *testing.T) {
	req := &canonical.Request{
		Tools:    []canonical.Tool{{Name: "f"}}, // no schema
		Messages: []canonical.Message{{Role: canonical.RoleUser, Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}}}},
	}
	body, _ := BuildRequest(req, "m")
	var out messagesRequest
	json.Unmarshal(body, &out)
	if string(out.Tools[0].InputSchema) != `{"type":"object"}` {
		t.Errorf("default schema not applied: %s", out.Tools[0].InputSchema)
	}
}
