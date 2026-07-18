package canonical

import (
	"encoding/json"
	"testing"
)

func TestResponseFormatZeroAndJSONSchema(t *testing.T) {
	var nilRF *ResponseFormat
	if nilRF != nil {
		t.Fatal("nil means unset")
	}
	rf := &ResponseFormat{
		Kind:   ResponseFormatJSONSchema,
		Name:   "contact",
		Schema: json.RawMessage(`{"type":"object"}`),
	}
	if rf.Kind != ResponseFormatJSONSchema || rf.Name != "contact" {
		t.Fatalf("%+v", rf)
	}
}

func TestThinkingConfigModes(t *testing.T) {
	// Nil / zero: no accidental enable
	var tc *ThinkingConfig
	if tc != nil {
		t.Fatal()
	}
	disabled := &ThinkingConfig{Mode: ThinkingDisabled}
	if disabled.Mode != ThinkingDisabled || disabled.BudgetTokens != nil {
		t.Fatalf("%+v", disabled)
	}
	budget := 4096
	enabled := &ThinkingConfig{Mode: ThinkingEnabled, BudgetTokens: &budget}
	if *enabled.BudgetTokens != 4096 {
		t.Fatal()
	}
	adaptive := &ThinkingConfig{Mode: ThinkingAdaptive, Effort: "high"}
	if adaptive.Effort != "high" {
		t.Fatal()
	}
}

func TestBlockDocumentAndRedacted(t *testing.T) {
	doc := Block{
		Type: BlockDocument,
		Document: &DocumentSource{
			Kind: "base64", MediaType: "application/pdf", Data: "JVBERi0=", Title: "a.pdf",
		},
	}
	if doc.Document.Title != "a.pdf" {
		t.Fatal()
	}
	red := Block{Type: BlockThinking, Text: "cipher", Redacted: true}
	if !red.Redacted || red.Text != "cipher" {
		t.Fatal()
	}
}

func TestRequestSamplingExtras(t *testing.T) {
	topK := 40
	seed := int64(7)
	fp, pp := 0.5, 0.1
	parallel := false
	req := Request{
		TopK:              &topK,
		Seed:              &seed,
		FrequencyPenalty:  &fp,
		PresencePenalty:   &pp,
		ParallelToolCalls: &parallel,
	}
	if *req.TopK != 40 || *req.Seed != 7 || *req.ParallelToolCalls {
		t.Fatalf("%+v", req)
	}
}
