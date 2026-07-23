package anthropic

import (
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestBuildRejectsCustomTools(t *testing.T) {
	_, err := BuildRequest(&canonical.Request{
		Messages: []canonical.Message{{Role: canonical.RoleUser, Content: []canonical.Block{{Type: canonical.BlockText, Text: "hi"}}}},
		Tools:    []canonical.Tool{{Kind: canonical.ToolKindCustom, Name: "c"}},
		MaxTokens: 10,
	}, "claude")
	if err == nil || !strings.Contains(err.Error(), "custom") {
		t.Fatalf("%v", err)
	}
}
