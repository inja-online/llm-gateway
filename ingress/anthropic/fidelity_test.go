package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

// --- #28 output_config / ResponseFormat ---

func TestParseOutputConfigJSONSchema(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"m","max_tokens":100,
		"messages":[{"role":"user","content":"x"}],
		"output_config":{
			"format":{
				"type":"json_schema",
				"name":"contact",
				"schema":{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}
			}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.ResponseFormat == nil || req.ResponseFormat.Kind != canonical.ResponseFormatJSONSchema {
		t.Fatalf("%+v", req.ResponseFormat)
	}
	if req.ResponseFormat.Name != "contact" {
		t.Fatalf("name %q", req.ResponseFormat.Name)
	}
	if !strings.Contains(string(req.ResponseFormat.Schema), `"name"`) {
		t.Fatalf("schema %s", req.ResponseFormat.Schema)
	}
}

// --- #30 thinking config ---

func TestParseThinkingEnabledBudget(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"m","max_tokens":100,
		"thinking":{"type":"enabled","budget_tokens":8000},
		"messages":[{"role":"user","content":"x"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Thinking == nil || req.Thinking.Type != "enabled" {
		t.Fatalf("%+v", req.Thinking)
	}
	if req.Thinking.BudgetTokens == nil || *req.Thinking.BudgetTokens != 8000 {
		t.Fatalf("budget %+v", req.Thinking.BudgetTokens)
	}
}

func TestParseThinkingAdaptiveAndDisabled(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		mode string
	}{
		{`{"type":"adaptive"}`, "adaptive"},
		{`{"type":"disabled"}`, "disabled"},
	} {
		req, err := ParseRequest([]byte(`{
			"model":"m","max_tokens":1,
			"thinking":` + tc.raw + `,
			"messages":[]
		}`))
		if err != nil {
			t.Fatalf("%s: %v", tc.raw, err)
		}
		if req.Thinking == nil || req.Thinking.Type != tc.mode {
			t.Fatalf("%s -> %+v", tc.raw, req.Thinking)
		}
	}
}

func TestParseThinkingEffortOnOutputConfig(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"m","max_tokens":1,
		"thinking":{"type":"adaptive"},
		"output_config":{"effort":"high"},
		"messages":[]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Thinking == nil || req.Thinking.Effort != "high" {
		t.Fatalf("%+v", req.Thinking)
	}
}

func TestParseNoThinkingDoesNotDefault(t *testing.T) {
	req, err := ParseRequest([]byte(`{"model":"m","max_tokens":1,"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Thinking != nil {
		t.Fatalf("want nil thinking, got %+v", req.Thinking)
	}
}

// --- #32 document blocks ---

func TestParseDocumentBase64AndURL(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"m","max_tokens":1,
		"messages":[{"role":"user","content":[
			{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"JVBERi0="},"title":"a.pdf"},
			{"type":"document","source":{"type":"url","url":"https://x/a.pdf"},"title":"remote"}
		]}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Messages[0].Content) != 2 {
		t.Fatalf("%+v", req.Messages[0].Content)
	}
	d0 := req.Messages[0].Content[0]
	if d0.Type != canonical.BlockDocument || d0.Document == nil {
		t.Fatalf("%+v", d0)
	}
	if d0.Document.Kind != "base64" || d0.Document.Data != "JVBERi0=" || d0.Document.Title != "a.pdf" {
		t.Fatalf("%+v", d0.Document)
	}
	d1 := req.Messages[0].Content[1]
	if d1.Document.Kind != "url" || d1.Document.Data != "https://x/a.pdf" {
		t.Fatalf("%+v", d1.Document)
	}
}

// --- #37 top_k ---

func TestParseTopK(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"m","max_tokens":1,"top_k":40,
		"messages":[]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.TopK == nil || *req.TopK != 40 {
		t.Fatalf("%v", req.TopK)
	}
}

// --- #40 disable_parallel_tool_use polarity ---

func TestParseDisableParallelToolUse(t *testing.T) {
	// disable=true → ParallelToolCalls=false
	req, err := ParseRequest([]byte(`{
		"model":"m","max_tokens":1,
		"tool_choice":{"type":"auto","disable_parallel_tool_use":true},
		"messages":[]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.ParallelToolCalls == nil || *req.ParallelToolCalls {
		t.Fatalf("want parallel=false, got %v", req.ParallelToolCalls)
	}

	// disable=false → ParallelToolCalls=true
	req2, err := ParseRequest([]byte(`{
		"model":"m","max_tokens":1,
		"tool_choice":{"type":"auto","disable_parallel_tool_use":false},
		"messages":[]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if req2.ParallelToolCalls == nil || !*req2.ParallelToolCalls {
		t.Fatalf("want parallel=true, got %v", req2.ParallelToolCalls)
	}

	// unset → nil
	req3, err := ParseRequest([]byte(`{
		"model":"m","max_tokens":1,
		"tool_choice":{"type":"auto"},
		"messages":[]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if req3.ParallelToolCalls != nil {
		t.Fatalf("want unset nil, got %v", req3.ParallelToolCalls)
	}
}

// --- #48 redacted_thinking preserve ---

func TestParseRedactedThinking(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"m","max_tokens":1,
		"messages":[{"role":"assistant","content":[
			{"type":"thinking","thinking":"visible","signature":"sig"},
			{"type":"redacted_thinking","data":"ENCRYPTED"},
			{"type":"text","text":"hi"}
		]}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	blocks := req.Messages[0].Content
	if len(blocks) != 3 {
		t.Fatalf("%+v", blocks)
	}
	if blocks[0].Type != canonical.BlockThinking || blocks[0].Redacted || blocks[0].Text != "visible" {
		t.Fatalf("thinking %+v", blocks[0])
	}
	if !blocks[1].Redacted || blocks[1].Text != "ENCRYPTED" {
		t.Fatalf("redacted %+v", blocks[1])
	}
}

func TestSerializeRedactedThinking(t *testing.T) {
	body, err := SerializeResponse(&canonical.Response{
		Model: "m",
		Content: []canonical.Block{
			{Type: canonical.BlockThinking, Text: "cipher", Redacted: true},
			{Type: canonical.BlockText, Text: "ok"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var out messagesResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Content) != 2 {
		t.Fatalf("%+v", out.Content)
	}
	if out.Content[0].Type != "redacted_thinking" || out.Content[0].Data != "cipher" {
		t.Fatalf("%+v", out.Content[0])
	}
}

func TestStreamSerializerRedactedThinking(t *testing.T) {
	s := NewStreamSerializer()
	out := string(s.Event(canonical.StreamEvent{
		Type:      canonical.EventBlockStart,
		BlockType: canonical.BlockThinking,
		Redacted:  true,
		Text:      "opaque",
		Index:     1,
	}))
	if !strings.Contains(out, `"type":"redacted_thinking"`) {
		t.Fatalf("%s", out)
	}
	if !strings.Contains(out, `"data":"opaque"`) {
		t.Fatalf("%s", out)
	}
}
