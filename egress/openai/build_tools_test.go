package openai

import (
	"encoding/json"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestBuildCustomToolAndFidelityFields(t *testing.T) {
	req := &canonical.Request{
		Messages: []canonical.Message{{Role: canonical.RoleUser, Content: []canonical.Block{{Type: canonical.BlockText, Text: "hi"}}}},
		Tools: []canonical.Tool{
			{Kind: canonical.ToolKindFunction, Name: "f", Schema: json.RawMessage(`{"type":"object"}`)},
			{Kind: canonical.ToolKindCustom, Name: "c", Grammar: "x+", GrammarType: "regex"},
		},
		SafetyIdentifier: "s",
		Verbosity:        "high",
		User:             "u",
	}
	body, err := BuildRequest(req, "m")
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	json.Unmarshal(body, &out)
	tools := out["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("%v", tools)
	}
	if out["safety_identifier"] != "s" || out["verbosity"] != "high" || out["user"] != "u" {
		t.Fatalf("%v", out)
	}
}

func TestBuildServerAndComputerTools(t *testing.T) {
	req := &canonical.Request{
		Messages: []canonical.Message{{Role: canonical.RoleUser, Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}}}},
		Tools: []canonical.Tool{
			{Kind: canonical.ToolKindServer, Name: "file_search"},
			{Kind: canonical.ToolKindComputer, Name: "computer"},
		},
		Stream: true,
		StreamOptions: json.RawMessage(`{"include_usage":false}`),
	}
	body, err := BuildRequest(req, "m")
	if err != nil {
		t.Fatal(err)
	}
	var out chatRequest
	json.Unmarshal(body, &out)
	if out.StreamOpts == nil || !out.StreamOpts.IncludeUsage {
		t.Fatal("include_usage forced")
	}
	if len(out.Tools) != 2 {
		t.Fatal(out.Tools)
	}
}
