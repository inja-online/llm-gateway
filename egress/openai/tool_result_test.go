package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestToolResultContentMultimodal(t *testing.T) {
	// Multimodal tool result: text + image → content parts array
	req := &canonical.Request{
		Messages: []canonical.Message{
			{Role: canonical.RoleAssistant, Content: []canonical.Block{
				{Type: canonical.BlockToolUse, ID: "call_1", Name: "see", Input: json.RawMessage(`{}`)},
			}},
			{Role: canonical.RoleUser, Content: []canonical.Block{
				{
					Type:      canonical.BlockToolResult,
					ToolUseID: "call_1",
					Result:    "fallback",
					ResultBlocks: []canonical.Block{
						{Type: canonical.BlockText, Text: "looks good"},
						{Type: canonical.BlockImage, Image: &canonical.ImageSource{
							Kind: "base64", MediaType: "image/png", Data: "abc",
						}},
						{Type: canonical.BlockImage}, // nil image skipped
						{Type: canonical.BlockToolUse}, // ignored type
					},
				},
			}},
		},
	}
	body, err := BuildRequest(req, "gpt-4o")
	if err != nil {
		t.Fatal(err)
	}
	var out chatRequest
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	var toolMsg *chatMessage
	for i := range out.Messages {
		if out.Messages[i].Role == "tool" {
			toolMsg = &out.Messages[i]
			break
		}
	}
	if toolMsg == nil {
		t.Fatalf("no tool message in %s", body)
	}
	raw := string(toolMsg.Content)
	if !strings.Contains(raw, "looks good") || !strings.Contains(raw, "image_url") {
		t.Fatalf("content %s", raw)
	}

	// Empty ResultBlocks falls back to Result string
	rawOnly := toolResultContent(canonical.Block{Result: `{"ok":true}`})
	if string(rawOnly) != `"{\"ok\":true}"` && !strings.Contains(string(rawOnly), "ok") {
		// jsonString wraps as JSON string
		if !json.Valid(rawOnly) {
			t.Fatalf("%s", rawOnly)
		}
	}

	// ResultBlocks with only unknown types falls through to Result
	fallback := toolResultContent(canonical.Block{
		Result:       "plain",
		ResultBlocks: []canonical.Block{{Type: canonical.BlockToolUse}},
	})
	if !strings.Contains(string(fallback), "plain") {
		t.Fatalf("%s", fallback)
	}

	// Text-only ResultBlocks
	textOnly := toolResultContent(canonical.Block{
		ResultBlocks: []canonical.Block{{Type: canonical.BlockText, Text: "t1"}, {Type: canonical.BlockText, Text: "t2"}},
	})
	if !strings.Contains(string(textOnly), "t1") || !strings.Contains(string(textOnly), "t2") {
		t.Fatalf("%s", textOnly)
	}
}
