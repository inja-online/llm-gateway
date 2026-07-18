package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestEffortToBudgetAll(t *testing.T) {
	cases := []struct {
		in     string
		want   int
		nilOut bool
	}{
		{"minimal", 1024, false},
		{"low", 1024, false},
		{"medium", 8192, false},
		{"high", 16384, false},
		{"xhigh", 16384, false},
		{"max", 16384, false},
		{"", 0, true},
		{"unknown", 0, true},
	}
	for _, tc := range cases {
		got := effortToBudget(tc.in)
		if tc.nilOut {
			if got != nil {
				t.Fatalf("%q: %v", tc.in, *got)
			}
			continue
		}
		if got == nil || *got != tc.want {
			t.Fatalf("%q: got %v want %d", tc.in, got, tc.want)
		}
	}
}

func TestBuildThinkingBranches(t *testing.T) {
	if buildThinking(nil) != nil {
		t.Fatal("nil")
	}
	if buildThinking(&canonical.ThinkingConfig{}) != nil {
		t.Fatal("empty")
	}
	// budget without type → enabled
	b := 500
	tw := buildThinking(&canonical.ThinkingConfig{BudgetTokens: &b})
	if tw == nil || tw.Type != "enabled" || tw.BudgetTokens == nil || *tw.BudgetTokens != 500 {
		t.Fatalf("%+v", tw)
	}
	// effort without type → adaptive
	tw = buildThinking(&canonical.ThinkingConfig{Effort: "medium"})
	if tw == nil || tw.Type != "adaptive" {
		t.Fatalf("%+v", tw)
	}
	// disabled
	tw = buildThinking(&canonical.ThinkingConfig{Type: "disabled"})
	if tw == nil || tw.Type != "disabled" {
		t.Fatalf("%+v", tw)
	}
	// adaptive explicit
	tw = buildThinking(&canonical.ThinkingConfig{Type: "adaptive"})
	if tw == nil || tw.Type != "adaptive" {
		t.Fatalf("%+v", tw)
	}
	// enabled + effort maps budget
	tw = buildThinking(&canonical.ThinkingConfig{Type: "enabled", Effort: "low"})
	if tw == nil || tw.Type != "enabled" || tw.BudgetTokens == nil || *tw.BudgetTokens != 1024 {
		t.Fatalf("%+v", tw)
	}
	// enabled + budget + effort: budget wins
	tw = buildThinking(&canonical.ThinkingConfig{Type: "enabled", BudgetTokens: &b, Effort: "high"})
	if tw == nil || *tw.BudgetTokens != 500 {
		t.Fatalf("%+v", tw)
	}
	// enabled with unknown effort and no budget
	tw = buildThinking(&canonical.ThinkingConfig{Type: "enabled", Effort: "nope"})
	if tw == nil || tw.Type != "enabled" || tw.BudgetTokens != nil {
		t.Fatalf("%+v", tw)
	}
	// unknown type → nil
	if buildThinking(&canonical.ThinkingConfig{Type: "weird"}) != nil {
		t.Fatal("weird")
	}
}

func TestDocumentToSourceBranches(t *testing.T) {
	// base64
	src := documentToSource(&canonical.DocumentSource{
		Kind: "base64", MediaType: "application/pdf", Data: "QQ==",
	})
	if src.Type != "base64" || src.Data != "QQ==" || src.MediaType != "application/pdf" {
		t.Fatalf("%+v", src)
	}
	// url
	src = documentToSource(&canonical.DocumentSource{Kind: "url", Data: "https://x/a.pdf"})
	if src.Type != "url" || src.URL != "https://x/a.pdf" {
		t.Fatalf("%+v", src)
	}
	// file
	src = documentToSource(&canonical.DocumentSource{Kind: "file", Data: "file_abc"})
	if src.Type != "file" || src.FileID != "file_abc" {
		t.Fatalf("%+v", src)
	}
	// default kind
	src = documentToSource(&canonical.DocumentSource{Kind: "file_uri", Data: "gs://b/o"})
	if src.Type != "file_uri" || src.Data != "gs://b/o" {
		t.Fatalf("%+v", src)
	}
}

func TestDocumentFromSourceBranches(t *testing.T) {
	// base64
	doc := documentFromSource(&imageSourceWire{Type: "base64", MediaType: "application/pdf", Data: "QQ=="})
	if doc.Kind != "base64" || doc.Data != "QQ==" {
		t.Fatalf("%+v", doc)
	}
	// url
	doc = documentFromSource(&imageSourceWire{Type: "url", URL: "https://x/a.pdf"})
	if doc.Kind != "url" || doc.Data != "https://x/a.pdf" {
		t.Fatalf("%+v", doc)
	}
	// file with FileID
	doc = documentFromSource(&imageSourceWire{Type: "file", FileID: "fid"})
	if doc.Kind != "file" || doc.Data != "fid" {
		t.Fatalf("%+v", doc)
	}
	// file_id with Data fallback
	doc = documentFromSource(&imageSourceWire{Type: "file_id", Data: "raw"})
	if doc.Kind != "file" || doc.Data != "raw" {
		t.Fatalf("%+v", doc)
	}
	// default: Data wins
	doc = documentFromSource(&imageSourceWire{Type: "custom", Data: "d", URL: "u", FileID: "f"})
	if doc.Kind != "custom" || doc.Data != "d" {
		t.Fatalf("%+v", doc)
	}
	// default: URL when no Data
	doc = documentFromSource(&imageSourceWire{Type: "custom", URL: "u", FileID: "f"})
	if doc.Data != "u" {
		t.Fatalf("%+v", doc)
	}
	// default: FileID last
	doc = documentFromSource(&imageSourceWire{Type: "custom", FileID: "f"})
	if doc.Data != "f" {
		t.Fatalf("%+v", doc)
	}
}

func TestParseBlockAllTypes(t *testing.T) {
	// text
	b, ok := parseBlock(block{Type: "text", Text: "hi"})
	if !ok || b.Type != canonical.BlockText || b.Text != "hi" {
		t.Fatalf("%+v", b)
	}
	// thinking
	b, ok = parseBlock(block{Type: "thinking", Thinking: "t", Signature: "s"})
	if !ok || b.Type != canonical.BlockThinking || b.Text != "t" || b.Signature != "s" {
		t.Fatalf("%+v", b)
	}
	// redacted
	b, ok = parseBlock(block{Type: "redacted_thinking", Data: "R"})
	if !ok || !b.Redacted || b.Text != "R" {
		t.Fatalf("%+v", b)
	}
	// tool_use
	b, ok = parseBlock(block{Type: "tool_use", ID: "1", Name: "f", Input: json.RawMessage(`{}`)})
	if !ok || b.Type != canonical.BlockToolUse || b.ID != "1" {
		t.Fatalf("%+v", b)
	}
	// document nil source → false
	if _, ok = parseBlock(block{Type: "document"}); ok {
		t.Fatal("document nil source")
	}
	// document base64
	b, ok = parseBlock(block{
		Type:  "document",
		Title: "a.pdf",
		Source: &imageSourceWire{Type: "base64", MediaType: "application/pdf", Data: "QQ=="},
	})
	if !ok || b.Document == nil || b.Document.Title != "a.pdf" || b.Document.Data != "QQ==" {
		t.Fatalf("%+v", b)
	}
	// document url
	b, ok = parseBlock(block{
		Type:   "document",
		Source: &imageSourceWire{Type: "url", URL: "https://x/a.pdf"},
	})
	if !ok || b.Document.Kind != "url" {
		t.Fatalf("%+v", b)
	}
	// document file
	b, ok = parseBlock(block{
		Type:   "document",
		Source: &imageSourceWire{Type: "file", FileID: "file_1"},
	})
	if !ok || b.Document.Kind != "file" || b.Document.Data != "file_1" {
		t.Fatalf("%+v", b)
	}
	// unknown
	if _, ok = parseBlock(block{Type: "image"}); ok {
		t.Fatal("image not handled in egress parseBlock")
	}
}

func TestParseResponseDocumentBlock(t *testing.T) {
	resp, err := ParseResponse([]byte(`{
		"id":"m","model":"c",
		"content":[
			{"type":"document","title":"q.pdf","source":{"type":"base64","media_type":"application/pdf","data":"QQ=="}},
			{"type":"document","source":{"type":"url","url":"https://x/a.pdf"}},
			{"type":"document","source":{"type":"file","file_id":"fid"}},
			{"type":"document"},
			{"type":"unknown"}
		],
		"stop_reason":"end_turn"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	// nil-source and unknown skipped
	if len(resp.Content) != 3 {
		t.Fatalf("%d %+v", len(resp.Content), resp.Content)
	}
	if resp.Content[0].Document.Title != "q.pdf" {
		t.Fatalf("%+v", resp.Content[0])
	}
}

func TestBuildDocumentKinds(t *testing.T) {
	for _, kind := range []string{"base64", "url", "file", "other"} {
		body, err := BuildRequest(&canonical.Request{
			MaxTokens: 1,
			Messages: []canonical.Message{{
				Role: canonical.RoleUser,
				Content: []canonical.Block{{
					Type: canonical.BlockDocument,
					Document: &canonical.DocumentSource{
						Kind: kind, MediaType: "application/pdf", Data: "payload", Title: "t",
					},
				}},
			}},
		}, "m")
		if err != nil {
			t.Fatal(err)
		}
		var out messagesRequest
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		if len(out.Messages) != 1 || len(out.Messages[0].Content) != 1 {
			t.Fatalf("%s: %+v", kind, out.Messages)
		}
		src := out.Messages[0].Content[0].Source
		if src == nil {
			t.Fatalf("%s: nil source", kind)
		}
	}
}

func TestBuildOutputConfigEffortOnly(t *testing.T) {
	// adaptive effort alone → output_config.effort
	oc := buildOutputConfig(nil, &canonical.ThinkingConfig{Type: "adaptive", Effort: "high"})
	if oc == nil || oc.Effort != "high" {
		t.Fatalf("%+v", oc)
	}
	// empty type + effort → adaptive path
	oc = buildOutputConfig(nil, &canonical.ThinkingConfig{Effort: "low"})
	if oc == nil || oc.Effort != "low" {
		t.Fatalf("%+v", oc)
	}
	// enabled thinking effort does not ride output_config
	oc = buildOutputConfig(nil, &canonical.ThinkingConfig{Type: "enabled", Effort: "high"})
	if oc != nil {
		t.Fatalf("%+v", oc)
	}
	// json_schema name only (no schema body)
	oc = buildOutputConfig(&canonical.ResponseFormat{
		Kind: canonical.ResponseFormatJSONSchema, Name: "n",
	}, nil)
	if oc == nil || oc.Format == nil || oc.Format.Name != "n" {
		t.Fatalf("%+v", oc)
	}
	// empty json_schema → nil
	if buildOutputConfig(&canonical.ResponseFormat{Kind: canonical.ResponseFormatJSONSchema}, nil) != nil {
		t.Fatal("empty json_schema")
	}
}

func TestNormalizeStopEdges(t *testing.T) {
	if normalizeStop("") != canonical.StopEndTurn {
		t.Fatal("empty")
	}
	if normalizeStop("refusal") != "refusal" {
		t.Fatal("refusal")
	}
	if normalizeStop("custom_stop") != "custom_stop" {
		t.Fatal("passthrough")
	}
}
