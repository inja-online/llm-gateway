package google

import (
	"encoding/json"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestBuildContentRich(t *testing.T) {
	req := &canonical.Request{
		Messages: []canonical.Message{
			{Role: canonical.RoleAssistant, Content: []canonical.Block{
				{Type: canonical.BlockThinking, Text: "plan"},
				{Type: canonical.BlockToolUse, ID: "c1", Name: "fn", Input: json.RawMessage(`{"a":1}`)},
			}},
			{Role: canonical.RoleUser, Content: []canonical.Block{
				{Type: canonical.BlockToolResult, ToolUseID: "c1", Result: `{"ok":true}`},
				{Type: canonical.BlockImage, Image: &canonical.ImageSource{Kind: "base64", MediaType: "image/png", Data: "xx"}},
				{Type: canonical.BlockImage, Image: &canonical.ImageSource{Kind: "url", Data: "https://ex/i.png"}},
				{Type: canonical.BlockText, Text: "more"},
			}},
		},
		ToolChoice: &canonical.ToolChoice{Mode: canonical.ToolNone},
	}
	body, err := BuildRequest(req, "m")
	if err != nil {
		t.Fatal(err)
	}
	var wire generateRequest
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.ToolConfig.FunctionCallingConfig.Mode != "NONE" {
		t.Fatal(wire.ToolConfig.FunctionCallingConfig.Mode)
	}
	// required / specific
	for _, tc := range []*canonical.ToolChoice{
		{Mode: canonical.ToolRequired},
		{Mode: canonical.ToolSpecific, Name: "fn"},
		{Mode: canonical.ToolAuto},
	} {
		req.ToolChoice = tc
		if _, err := BuildRequest(req, "m"); err != nil {
			t.Fatal(err)
		}
	}
	// empty tool schema
	req.Tools = []canonical.Tool{{Name: "x"}}
	if _, err := BuildRequest(req, "m"); err != nil {
		t.Fatal(err)
	}
	// empty content
	req.Messages = []canonical.Message{{Role: canonical.RoleUser, Content: nil}}
	if _, err := BuildRequest(req, "m"); err != nil {
		t.Fatal(err)
	}
	// non-json tool result
	req.Messages = []canonical.Message{{Role: canonical.RoleUser, Content: []canonical.Block{
		{Type: canonical.BlockToolResult, ToolUseID: "c1", Result: "not-json"},
	}}}
	if _, err := BuildRequest(req, "m"); err != nil {
		t.Fatal(err)
	}
}

func TestParseResponseVariants(t *testing.T) {
	// snake_case usage + finish + function call
	body := []byte(`{
		"response_id":"r2",
		"model_version":"m2",
		"candidates":[{
			"content":{"parts":[
				{"text":"t","thought":true},
				{"function_call":{"name":"fn","args":{}}}
			]},
			"finish_reason":"MAX_TOKENS"
		}],
		"usage_metadata":{"prompt_token_count":7,"candidates_token_count":3}
	}`)
	resp, err := ParseResponse(body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != "r2" || resp.Model != "m2" {
		t.Fatalf("%+v", resp)
	}
	if resp.StopReason != canonical.StopToolUse {
		t.Fatal(resp.StopReason)
	}
	if resp.Usage.InputTokens != 7 {
		t.Fatal(resp.Usage)
	}
	// safety finish
	body2 := []byte(`{"candidates":[{"content":{"parts":[{"text":"x"}]},"finishReason":"SAFETY"}]}`)
	resp2, _ := ParseResponse(body2)
	if resp2.StopReason != canonical.StopRefusal {
		t.Fatal(resp2.StopReason)
	}
	// invalid json
	if _, err := ParseResponse([]byte(`{`)); err == nil {
		t.Fatal("expected error")
	}
	// empty candidates
	if _, err := ParseResponse([]byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	// stop mapping
	for fr, want := range map[string]string{
		"STOP": canonical.StopEndTurn, "max_tokens": canonical.StopMaxTokens,
		"RECITATION": canonical.StopRefusal, "weird": canonical.StopEndTurn,
	} {
		if got := finishToStop(fr, nil); got != want {
			t.Fatalf("%s -> %s want %s", fr, got, want)
		}
	}
}

func TestStreamParserToolsAndThought(t *testing.T) {
	p := NewStreamParser()
	evs := p.Parse([]byte(`{"candidates":[{"content":{"parts":[{"text":"think","thought":true}]}}]}`))
	if len(evs) < 2 {
		t.Fatalf("%+v", evs)
	}
	_ = p.Parse([]byte(`{"candidates":[{"content":{"parts":[{"function_call":{"name":"fn","args":{"a":1}}}]},"finishReason":"STOP"}]}`))
	fin := p.Finish()
	// second finish is empty
	if p.Finish() != nil {
		t.Fatal("double finish")
	}
	var sawTool bool
	for _, e := range fin {
		if e.Type == canonical.EventFinish && e.StopReason == canonical.StopToolUse {
			sawTool = true
		}
	}
	if !sawTool {
		t.Fatalf("%+v", fin)
	}
	// empty stream finish
	p2 := NewStreamParser()
	if len(p2.Finish()) < 1 {
		t.Fatal("empty finish")
	}
	// bad json ignored
	p3 := NewStreamParser()
	if p3.Parse([]byte(`nope`)) != nil {
		t.Fatal()
	}
	// usage only
	p3.Parse([]byte(`{"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":0}}`))
}

// TestStreamParserThoughtThenText uses distinct block indexes so thinking
// deltas never share an index with visible text.
func TestStreamParserThoughtThenText(t *testing.T) {
	p := NewStreamParser()
	var all []canonical.StreamEvent
	all = append(all, p.Parse([]byte(`{"candidates":[{"content":{"parts":[{"text":"plan","thought":true}]}}]}`))...)
	all = append(all, p.Parse([]byte(`{"candidates":[{"content":{"parts":[{"text":"hello"}]},"finishReason":"STOP"}]}`))...)
	all = append(all, p.Finish()...)

	var thinkIdx, textIdx = -1, -1
	var thinking, text string
	for _, e := range all {
		switch e.Type {
		case canonical.EventBlockStart:
			if e.BlockType == canonical.BlockThinking {
				thinkIdx = e.Index
			}
			if e.BlockType == canonical.BlockText {
				textIdx = e.Index
			}
		case canonical.EventThinkingDelta:
			thinking += e.Text
			if thinkIdx >= 0 && e.Index != thinkIdx {
				t.Fatalf("thinking index mismatch %d vs %d", e.Index, thinkIdx)
			}
		case canonical.EventTextDelta:
			text += e.Text
			if textIdx >= 0 && e.Index != textIdx {
				t.Fatalf("text index mismatch %d vs %d", e.Index, textIdx)
			}
		}
	}
	if thinking != "plan" || text != "hello" {
		t.Fatalf("thinking=%q text=%q", thinking, text)
	}
	if thinkIdx < 0 || textIdx < 0 || thinkIdx == textIdx {
		t.Fatalf("indexes think=%d text=%d", thinkIdx, textIdx)
	}
}

func TestWireHelpers(t *testing.T) {
	u := &usageMetadata{PromptTokensSnake: 1, CandidatesTokensSnake: 2}
	if u.prompt() != 1 || u.candidates() != 2 {
		t.Fatal()
	}
	r := generateResponse{UsageMetadataSnake: u, ModelVersionSnake: "m", ResponseIDSnake: "i"}
	if r.usage() != u || r.model() != "m" || r.id() != "i" {
		t.Fatal()
	}
	c := candidate{FinishSnake: "STOP"}
	if c.finish() != "STOP" {
		t.Fatal()
	}
}
