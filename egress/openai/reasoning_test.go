package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestBuildAssistantReasoningAndRedacted(t *testing.T) {
	req := &canonical.Request{
		Messages: []canonical.Message{
			{Role: canonical.RoleAssistant, Content: []canonical.Block{
				{Type: canonical.BlockThinking, Text: "step1", Redacted: false},
				{Type: canonical.BlockThinking, Text: "secret", Redacted: true},
				{Type: canonical.BlockText, Text: "answer"},
				{Type: canonical.BlockToolUse, ID: "c1", Name: "f", Input: nil},
			}},
		},
	}
	body, err := BuildRequest(req, "m")
	if err != nil {
		t.Fatal(err)
	}
	var out chatRequest
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Messages) != 1 {
		t.Fatalf("%d", len(out.Messages))
	}
	msg := out.Messages[0]
	if string(msg.Content) != `"answer"` {
		t.Fatalf("content %s", msg.Content)
	}
	if !strings.Contains(string(msg.Reasoning), "step1") {
		t.Fatalf("reasoning %s", msg.Reasoning)
	}
	if strings.Contains(string(msg.Reasoning), "secret") {
		t.Fatal("redacted thinking must not appear")
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Arguments != "{}" {
		t.Fatalf("%+v", msg.ToolCalls)
	}
}
