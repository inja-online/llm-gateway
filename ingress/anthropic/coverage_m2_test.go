package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestDocumentFromSourceAllBranches(t *testing.T) {
	doc := documentFromSource(&imageSourceWire{Type: "base64", MediaType: "application/pdf", Data: "QQ=="})
	if doc.Kind != "base64" || doc.Data != "QQ==" || doc.MediaType != "application/pdf" {
		t.Fatalf("%+v", doc)
	}
	doc = documentFromSource(&imageSourceWire{Type: "url", URL: "https://x/a.pdf"})
	if doc.Kind != "url" || doc.Data != "https://x/a.pdf" {
		t.Fatalf("%+v", doc)
	}
	doc = documentFromSource(&imageSourceWire{Type: "file", FileID: "fid"})
	if doc.Kind != "file" || doc.Data != "fid" {
		t.Fatalf("%+v", doc)
	}
	doc = documentFromSource(&imageSourceWire{Type: "file_id", Data: "fallback"})
	if doc.Kind != "file" || doc.Data != "fallback" {
		t.Fatalf("%+v", doc)
	}
	// default: Data > URL > FileID
	doc = documentFromSource(&imageSourceWire{Type: "custom", Data: "d", URL: "u", FileID: "f"})
	if doc.Kind != "custom" || doc.Data != "d" {
		t.Fatalf("%+v", doc)
	}
	doc = documentFromSource(&imageSourceWire{Type: "custom", URL: "u", FileID: "f"})
	if doc.Data != "u" {
		t.Fatalf("%+v", doc)
	}
	doc = documentFromSource(&imageSourceWire{Type: "custom", FileID: "f"})
	if doc.Data != "f" {
		t.Fatalf("%+v", doc)
	}
}

func TestParseThinkingBranches(t *testing.T) {
	if parseThinking(nil) != nil {
		t.Fatal("nil")
	}
	tc := parseThinking(&thinkingWire{Type: "enabled", BudgetTokens: intPtr(10)})
	if tc == nil || tc.Type != "enabled" || tc.BudgetTokens == nil || *tc.BudgetTokens != 10 {
		t.Fatalf("%+v", tc)
	}
	tc = parseThinking(&thinkingWire{Type: "disabled"})
	if tc == nil || tc.Type != "disabled" {
		t.Fatalf("%+v", tc)
	}
	tc = parseThinking(&thinkingWire{Type: "adaptive"})
	if tc == nil || tc.Type != "adaptive" {
		t.Fatalf("%+v", tc)
	}
	// unknown type with budget → enabled
	tc = parseThinking(&thinkingWire{Type: "weird", BudgetTokens: intPtr(1)})
	if tc == nil || tc.Type != "enabled" {
		t.Fatalf("%+v", tc)
	}
	// unknown type without budget → empty type preserved path
	tc = parseThinking(&thinkingWire{Type: "weird"})
	if tc == nil || tc.Type != "" {
		t.Fatalf("%+v", tc)
	}
}

func TestParseOutputConfigBranches(t *testing.T) {
	if parseOutputConfig(nil) != nil {
		t.Fatal("nil")
	}
	if parseOutputConfig(&outputConfigWire{}) != nil {
		t.Fatal("empty")
	}
	rf := parseOutputConfig(&outputConfigWire{Format: &outputFormatWire{
		Type: "json_schema", Name: "n", Schema: json.RawMessage(`{"type":"object"}`),
	}})
	if rf == nil || rf.Kind != canonical.ResponseFormatJSONSchema || rf.Name != "n" {
		t.Fatalf("%+v", rf)
	}
	rf = parseOutputConfig(&outputConfigWire{Format: &outputFormatWire{Type: "json_object"}})
	if rf == nil || rf.Kind != canonical.ResponseFormatJSONObject {
		t.Fatalf("%+v", rf)
	}
	if parseOutputConfig(&outputConfigWire{Format: &outputFormatWire{Type: "xml"}}) != nil {
		t.Fatal("unknown")
	}
}

func TestParseDocumentBlocksInRequest(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"c","max_tokens":1,
		"messages":[{"role":"user","content":[
			{"type":"document","title":"a.pdf","source":{"type":"base64","media_type":"application/pdf","data":"QQ=="}},
			{"type":"document","source":{"type":"url","url":"https://x/a.pdf"}},
			{"type":"document","source":{"type":"file","file_id":"fid"}},
			{"type":"document","source":{"type":"file_id","data":"raw"}},
			{"type":"document","source":{"type":"custom","url":"u"}},
			{"type":"document","source":{"type":"custom","file_id":"onlyf"}}
		]}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	blocks := req.Messages[0].Content
	if len(blocks) != 6 {
		t.Fatalf("%d", len(blocks))
	}
	if blocks[0].Document.Title != "a.pdf" || blocks[0].Document.Kind != "base64" {
		t.Fatalf("%+v", blocks[0].Document)
	}
	if blocks[1].Document.Kind != "url" || blocks[1].Document.Data != "https://x/a.pdf" {
		t.Fatalf("%+v", blocks[1].Document)
	}
	if blocks[2].Document.Kind != "file" || blocks[2].Document.Data != "fid" {
		t.Fatalf("%+v", blocks[2].Document)
	}
	if blocks[3].Document.Kind != "file" || blocks[3].Document.Data != "raw" {
		t.Fatalf("%+v", blocks[3].Document)
	}
	if blocks[4].Document.Data != "u" {
		t.Fatalf("%+v", blocks[4].Document)
	}
	if blocks[5].Document.Data != "onlyf" {
		t.Fatalf("%+v", blocks[5].Document)
	}
}

func TestParseThinkingAndOutputConfigBody(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"c","max_tokens":1,
		"thinking":{"type":"enabled","budget_tokens":2048},
		"output_config":{"format":{"type":"json_schema","name":"n","schema":{"type":"object"}}},
		"messages":[{"role":"user","content":"hi"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Thinking == nil || req.Thinking.Type != "enabled" || req.Thinking.BudgetTokens == nil || *req.Thinking.BudgetTokens != 2048 {
		t.Fatalf("%+v", req.Thinking)
	}
	if req.ResponseFormat == nil || req.ResponseFormat.Kind != canonical.ResponseFormatJSONSchema || req.ResponseFormat.Name != "n" {
		t.Fatalf("%+v", req.ResponseFormat)
	}

	// adaptive / disabled / json_object / unknown thinking type with budget
	req, err = ParseRequest([]byte(`{
		"model":"c","max_tokens":1,
		"thinking":{"type":"adaptive"},
		"output_config":{"format":{"type":"json_object"}},
		"messages":[{"role":"user","content":"hi"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Thinking.Type != "adaptive" {
		t.Fatalf("%+v", req.Thinking)
	}
	if req.ResponseFormat.Kind != canonical.ResponseFormatJSONObject {
		t.Fatalf("%+v", req.ResponseFormat)
	}

	req, err = ParseRequest([]byte(`{
		"model":"c","max_tokens":1,
		"thinking":{"type":"disabled"},
		"messages":[{"role":"user","content":"hi"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Thinking.Type != "disabled" {
		t.Fatalf("%+v", req.Thinking)
	}

	// budget without known type
	req, err = ParseRequest([]byte(`{
		"model":"c","max_tokens":1,
		"thinking":{"type":"future","budget_tokens":9},
		"output_config":{"format":{"type":"xml"}},
		"messages":[{"role":"user","content":"hi"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Thinking.Type != "enabled" || *req.Thinking.BudgetTokens != 9 {
		t.Fatalf("%+v", req.Thinking)
	}
	if req.ResponseFormat != nil {
		t.Fatalf("unknown format should drop: %+v", req.ResponseFormat)
	}
}

func intPtr(n int) *int { return &n }
