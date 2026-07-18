package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestBuildDefaultMaxTokensAndTools(t *testing.T) {
	body, err := BuildRequest(&canonical.Request{
		Messages: []canonical.Message{
			{Role: canonical.RoleUser, Content: []canonical.Block{
				{Type: canonical.BlockText, Text: "hi"},
				{Type: canonical.BlockImage, Image: &canonical.ImageSource{Kind: "url", Data: "https://x/a.png"}},
				{Type: canonical.BlockThinking, Text: "t", Signature: "s"},
			}},
			{Role: canonical.RoleAssistant, Content: []canonical.Block{
				{Type: canonical.BlockToolUse, ID: "1", Name: "f"},
			}},
			{Role: canonical.RoleUser, Content: []canonical.Block{
				{Type: canonical.BlockToolResult, ToolUseID: "1", Result: "ok", IsError: true},
			}},
			{Role: canonical.RoleUser, Content: nil}, // empty skipped
		},
		Tools: []canonical.Tool{{Name: "f"}},
		ToolChoice: &canonical.ToolChoice{Mode: canonical.ToolRequired},
	}, "claude")
	if err != nil {
		t.Fatal(err)
	}
	var out messagesRequest
	json.Unmarshal(body, &out)
	if out.MaxTokens != defaultMaxTokens {
		t.Fatalf("max_tokens %d", out.MaxTokens)
	}
	if string(out.ToolChoice) != `{"type":"any"}` {
		t.Fatalf("tool_choice %s", out.ToolChoice)
	}
	if len(out.Tools) != 1 || string(out.Tools[0].InputSchema) != `{"type":"object"}` {
		t.Fatalf("tools %+v", out.Tools)
	}
}

func TestBuildToolChoiceVariants(t *testing.T) {
	cases := []struct {
		mode      canonical.ToolChoiceMode
		wantType  string
		wantName  string
	}{
		{canonical.ToolAuto, "auto", ""},
		{canonical.ToolNone, "none", ""},
		{canonical.ToolSpecific, "tool", "x"},
	}
	for _, c := range cases {
		body, _ := BuildRequest(&canonical.Request{
			MaxTokens:  1,
			ToolChoice: &canonical.ToolChoice{Mode: c.mode, Name: "x"},
			Messages: []canonical.Message{
				{Role: canonical.RoleUser, Content: []canonical.Block{{Type: canonical.BlockText, Text: "a"}}},
			},
		}, "m")
		var out messagesRequest
		json.Unmarshal(body, &out)
		var tc struct {
			Type string `json:"type"`
			Name string `json:"name"`
		}
		json.Unmarshal(out.ToolChoice, &tc)
		if tc.Type != c.wantType || tc.Name != c.wantName {
			t.Errorf("%s: got %+v", c.mode, tc)
		}
	}
}

func TestNormalizeStopUnknown(t *testing.T) {
	if normalizeStop("custom") != "custom" {
		t.Fatal(normalizeStop("custom"))
	}
	if normalizeStop("") != canonical.StopEndTurn {
		t.Fatal()
	}
	if normalizeStop("refusal") != "refusal" {
		t.Fatal()
	}
}

func TestParseBlockRedactedThinkingPreserve(t *testing.T) {
	// #48: redacted_thinking must be preserved (not skipped) for multi-turn Claude.
	cb, ok := parseBlock(block{Type: "redacted_thinking", Data: "opaque-cipher"})
	if !ok {
		t.Fatal("want preserve redacted_thinking")
	}
	if cb.Type != canonical.BlockThinking || !cb.Redacted || cb.Text != "opaque-cipher" {
		t.Fatalf("%+v", cb)
	}
}

func TestParseResponseInvalidJSON(t *testing.T) {
	if _, err := ParseResponse([]byte(`{`)); err == nil {
		t.Fatal()
	}
}

func TestBuildImageBase64Source(t *testing.T) {
	body, _ := BuildRequest(&canonical.Request{
		MaxTokens: 1,
		Messages: []canonical.Message{{
			Role: canonical.RoleUser,
			Content: []canonical.Block{{
				Type:  canonical.BlockImage,
				Image: &canonical.ImageSource{Kind: "base64", MediaType: "image/png", Data: "AA"},
			}},
		}},
	}, "m")
	var out messagesRequest
	json.Unmarshal(body, &out)
	src := out.Messages[0].Content[0].Source
	if src == nil || src.Type != "base64" || src.Data != "AA" {
		t.Fatalf("%+v", src)
	}
}

func TestBuildEmptyTextSkipped(t *testing.T) {
	body, _ := BuildRequest(&canonical.Request{
		MaxTokens: 1,
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: ""}},
		}},
	}, "m")
	var out messagesRequest
	json.Unmarshal(body, &out)
	if len(out.Messages) != 0 {
		t.Fatalf("%+v", out.Messages)
	}
}
