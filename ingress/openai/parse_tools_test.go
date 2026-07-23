package openai

import (
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestParseCustomAndServerTools(t *testing.T) {
	body := []byte(`{
		"model":"gpt",
		"messages":[{"role":"user","content":"hi"}],
		"tools":[
			{"type":"function","function":{"name":"f","parameters":{"type":"object"}}},
			{"type":"custom","custom":{"name":"re","description":"d","format":{"type":"regex","definition":"[0-9]+"}}},
			{"type":"file_search","name":"fs"}
		],
		"safety_identifier":"sid",
		"verbosity":"low",
		"user":"u1"
	}`)
	req, err := ParseRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Tools) != 3 {
		t.Fatalf("tools %d", len(req.Tools))
	}
	if req.Tools[1].Kind != canonical.ToolKindCustom || req.Tools[1].Grammar == "" {
		t.Fatalf("%+v", req.Tools[1])
	}
	if req.Tools[2].Kind != canonical.ToolKindServer {
		t.Fatalf("%+v", req.Tools[2])
	}
	if req.SafetyIdentifier != "sid" || req.Verbosity != "low" || req.User != "u1" {
		t.Fatalf("fields %+v", req)
	}
}

func TestParseCustomFormatHelpers(t *testing.T) {
	gt, g := parseCustomFormat(nil)
	if gt != "" || g != "" {
		t.Fatal("empty")
	}
	gt, g = parseCustomFormat([]byte(`{"type":"lark","grammar":{"definition":"S","syntax":"lark"}}`))
	if gt == "" || g != "S" {
		t.Fatalf("%q %q", gt, g)
	}
	if firstNonEmpty("", "a", "b") != "a" {
		t.Fatal("first")
	}
}
