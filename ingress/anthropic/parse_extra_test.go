package anthropic

import (
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestParseImageURLAndToolResultArray(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"m","max_tokens":10,
		"messages":[{"role":"user","content":[
			{"type":"image","source":{"type":"url","url":"https://x/a.png"}},
			{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"part1"},{"type":"text","text":"part2"}],"is_error":true},
			{"type":"tool_use","id":"u1","name":"f","input":{"a":1}},
			{"type":"thinking","thinking":"r","signature":"s"},
			{"type":"unknown_skip"}
		]}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var sawImg, sawTR, sawTU, sawThink bool
	for _, b := range req.Messages[0].Content {
		switch b.Type {
		case canonical.BlockImage:
			sawImg = true
			if b.Image.Kind != "url" || b.Image.Data != "https://x/a.png" {
				t.Fatalf("%+v", b.Image)
			}
		case canonical.BlockToolResult:
			sawTR = true
			if b.Result != "part1part2" || !b.IsError {
				t.Fatalf("%+v", b)
			}
		case canonical.BlockToolUse:
			sawTU = true
		case canonical.BlockThinking:
			sawThink = true
		}
	}
	if !sawImg || !sawTR || !sawTU || !sawThink {
		t.Fatalf("flags img=%v tr=%v tu=%v th=%v content=%+v", sawImg, sawTR, sawTU, sawThink, req.Messages[0].Content)
	}
}

func TestToolResultTextFallbacks(t *testing.T) {
	if toolResultText(nil) != "" {
		t.Fatal()
	}
	if toolResultText([]byte(`"hi"`)) != "hi" {
		t.Fatal(toolResultText([]byte(`"hi"`)))
	}
	if toolResultText([]byte(`123`)) != "123" {
		t.Fatal(toolResultText([]byte(`123`)))
	}
}

func TestParseImageNilSource(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"m","max_tokens":1,
		"messages":[{"role":"user","content":[{"type":"image"}]}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Messages[0].Content) != 0 {
		t.Fatalf("%+v", req.Messages[0].Content)
	}
}

func TestParseToolChoiceInvalidJSON(t *testing.T) {
	_, err := ParseRequest([]byte(`{"model":"m","max_tokens":1,"tool_choice":"auto","messages":[]}`))
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("%v", err)
	}
}

func TestParseEmptyStringContent(t *testing.T) {
	req, err := ParseRequest([]byte(`{"model":"m","max_tokens":1,"messages":[{"role":"user","content":""}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Messages[0].Content) != 0 {
		t.Fatal(req.Messages[0].Content)
	}
}
