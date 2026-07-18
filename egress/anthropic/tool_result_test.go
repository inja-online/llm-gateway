package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestBuildToolResultMultimodal(t *testing.T) {
	req := &canonical.Request{
		Messages: []canonical.Message{{
			Role: canonical.RoleUser,
			Content: []canonical.Block{{
				Type:      canonical.BlockToolResult,
				ToolUseID: "tu1",
				Result:    "see image",
				ResultBlocks: []canonical.Block{
					{Type: canonical.BlockText, Text: "see image"},
					{Type: canonical.BlockImage, Image: &canonical.ImageSource{
						Kind: "base64", MediaType: "image/png", Data: "AAAA",
					}},
				},
			}},
		}},
	}
	body, err := BuildRequest(req, "claude-x")
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	msgs := out["messages"].([]any)
	msg := msgs[0].(map[string]any)
	content := msg["content"].([]any)
	tr := content[0].(map[string]any)
	// content should be an array of blocks, not a plain string
	arr, ok := tr["content"].([]any)
	if !ok {
		t.Fatalf("want array content, got %T %v", tr["content"], tr["content"])
	}
	if len(arr) != 2 {
		t.Fatalf("blocks: %v", arr)
	}
	if arr[0].(map[string]any)["type"] != "text" {
		t.Fatalf("first: %v", arr[0])
	}
	if arr[1].(map[string]any)["type"] != "image" {
		t.Fatalf("second: %v", arr[1])
	}
}

func TestBuildToolResultStringOnly(t *testing.T) {
	req := &canonical.Request{
		Messages: []canonical.Message{{
			Role: canonical.RoleUser,
			Content: []canonical.Block{{
				Type:      canonical.BlockToolResult,
				ToolUseID: "tu1",
				Result:    "plain",
			}},
		}},
	}
	body, _ := BuildRequest(req, "m")
	var out map[string]any
	json.Unmarshal(body, &out)
	tr := out["messages"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)
	if tr["content"] != "plain" {
		t.Fatalf("want string content, got %v", tr["content"])
	}
}
