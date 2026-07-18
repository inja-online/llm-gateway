package openai

import (
	"encoding/json"
	"testing"

	"github.com/mamad/llm-gateway/canonical"
)

func TestBuildBasic(t *testing.T) {
	req := &canonical.Request{
		MaxTokens: 100,
		System:    []canonical.Block{{Type: canonical.BlockText, Text: "sys"}},
		Messages: []canonical.Message{
			{Role: canonical.RoleUser, Content: []canonical.Block{{Type: canonical.BlockText, Text: "hi"}}},
		},
	}
	body, err := BuildRequest(req, "gpt-4o")
	if err != nil {
		t.Fatal(err)
	}
	var out chatRequest
	json.Unmarshal(body, &out)
	if out.Model != "gpt-4o" || out.MaxTokens != 100 {
		t.Errorf("envelope: %+v", out)
	}
	if len(out.Messages) != 2 {
		t.Fatalf("want system+user, got %d: %+v", len(out.Messages), out.Messages)
	}
	if out.Messages[0].Role != "system" || string(out.Messages[0].Content) != `"sys"` {
		t.Errorf("system message: %+v", out.Messages[0])
	}
	if out.Messages[1].Role != "user" || string(out.Messages[1].Content) != `"hi"` {
		t.Errorf("user message: %+v", out.Messages[1])
	}
}

func TestBuildStreamRequestsUsage(t *testing.T) {
	req := &canonical.Request{Stream: true, Messages: []canonical.Message{
		{Role: canonical.RoleUser, Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}}}}}
	body, _ := BuildRequest(req, "m")
	var out chatRequest
	json.Unmarshal(body, &out)
	if out.StreamOpts == nil || !out.StreamOpts.IncludeUsage {
		t.Error("stream_options.include_usage must be set so usage is meterable")
	}
}

// TestBuildToolResultOrdering guards the OpenAI contract: role:tool messages
// must directly follow the assistant turn that made the calls, so tool results
// have to be emitted before any user text from the same canonical turn.
func TestBuildToolResultOrdering(t *testing.T) {
	req := &canonical.Request{
		Messages: []canonical.Message{
			{Role: canonical.RoleAssistant, Content: []canonical.Block{
				{Type: canonical.BlockToolUse, ID: "tu1", Name: "f", Input: json.RawMessage(`{}`)},
			}},
			{Role: canonical.RoleUser, Content: []canonical.Block{
				{Type: canonical.BlockToolResult, ToolUseID: "tu1", Result: "42"},
				{Type: canonical.BlockText, Text: "thanks"},
			}},
		},
	}
	body, _ := BuildRequest(req, "m")
	var out chatRequest
	json.Unmarshal(body, &out)

	if len(out.Messages) != 3 {
		t.Fatalf("want assistant+tool+user, got %d: %+v", len(out.Messages), out.Messages)
	}
	if out.Messages[0].Role != "assistant" || len(out.Messages[0].ToolCalls) != 1 {
		t.Errorf("assistant tool_calls: %+v", out.Messages[0])
	}
	if out.Messages[1].Role != "tool" || out.Messages[1].ToolCallID != "tu1" {
		t.Errorf("tool message must immediately follow the assistant turn, got: %+v", out.Messages[1])
	}
	if out.Messages[2].Role != "user" {
		t.Errorf("user text must come after tool results, got: %+v", out.Messages[2])
	}
}

func TestBuildAssistantToolCall(t *testing.T) {
	req := &canonical.Request{Messages: []canonical.Message{
		{Role: canonical.RoleAssistant, Content: []canonical.Block{
			{Type: canonical.BlockText, Text: "checking"},
			{Type: canonical.BlockToolUse, ID: "tu1", Name: "search", Input: json.RawMessage(`{"q":"go"}`)},
		}},
	}}
	body, _ := BuildRequest(req, "m")
	var out chatRequest
	json.Unmarshal(body, &out)
	m := out.Messages[0]
	if string(m.Content) != `"checking"` {
		t.Errorf("assistant text: %s", m.Content)
	}
	if m.ToolCalls[0].Function.Name != "search" || m.ToolCalls[0].Function.Arguments != `{"q":"go"}` {
		t.Errorf("tool call: %+v", m.ToolCalls[0])
	}
}

func TestBuildImageAsDataURL(t *testing.T) {
	req := &canonical.Request{Messages: []canonical.Message{
		{Role: canonical.RoleUser, Content: []canonical.Block{
			{Type: canonical.BlockText, Text: "look"},
			{Type: canonical.BlockImage, Image: &canonical.ImageSource{Kind: "base64", MediaType: "image/png", Data: "AAAA"}},
		}},
	}}
	body, _ := BuildRequest(req, "m")
	var out chatRequest
	json.Unmarshal(body, &out)
	var parts []contentPart
	if err := json.Unmarshal(out.Messages[0].Content, &parts); err != nil {
		t.Fatalf("multimodal content must be a parts array: %s", out.Messages[0].Content)
	}
	if parts[1].ImageURL.URL != "data:image/png;base64,AAAA" {
		t.Errorf("image url: %s", parts[1].ImageURL.URL)
	}
}

func TestBuildToolChoice(t *testing.T) {
	cases := []struct {
		mode canonical.ToolChoiceMode
		name string
		want string
	}{
		{canonical.ToolAuto, "", `"auto"`},
		{canonical.ToolNone, "", `"none"`},
		{canonical.ToolRequired, "", `"required"`},
		{canonical.ToolSpecific, "f", `{"function":{"name":"f"},"type":"function"}`},
	}
	for _, c := range cases {
		req := &canonical.Request{ToolChoice: &canonical.ToolChoice{Mode: c.mode, Name: c.name}}
		body, _ := BuildRequest(req, "m")
		var out chatRequest
		json.Unmarshal(body, &out)
		if string(out.ToolChoice) != c.want {
			t.Errorf("%v -> %s, want %s", c.mode, out.ToolChoice, c.want)
		}
	}
}
