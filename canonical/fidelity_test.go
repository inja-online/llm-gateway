package canonical

import (
	"encoding/json"
	"testing"
)

func TestResponseFormatZeroVsSet(t *testing.T) {
	var req Request
	if req.ResponseFormat != nil {
		t.Fatal("zero Request must have nil ResponseFormat")
	}
	strict := true
	req.ResponseFormat = &ResponseFormat{
		Kind:   ResponseFormatJSONSchema,
		Name:   "out",
		Schema: json.RawMessage(`{"type":"object"}`),
		Strict: &strict,
	}
	if req.ResponseFormat.Kind != ResponseFormatJSONSchema || req.ResponseFormat.Name != "out" {
		t.Fatalf("%+v", req.ResponseFormat)
	}
	if req.ResponseFormat.Strict == nil || !*req.ResponseFormat.Strict {
		t.Fatal("strict")
	}
}

func TestThinkingConfigZeroVsExplicit(t *testing.T) {
	var req Request
	if req.Thinking != nil {
		t.Fatal("nil thinking means client did not request controls")
	}
	budget := 1024
	inc := true
	req.Thinking = &ThinkingConfig{
		Mode:            ThinkingEnabled,
		Effort:          "high",
		BudgetTokens:    &budget,
		IncludeThoughts: &inc,
	}
	if req.Thinking.Effort != "high" || *req.Thinking.BudgetTokens != 1024 {
		t.Fatalf("%+v", req.Thinking)
	}
	req.Thinking = &ThinkingConfig{Mode: ThinkingDisabled}
	if req.Thinking.Mode != ThinkingDisabled {
		t.Fatal("explicit disable must be representable")
	}
}

func TestSamplingPenaltyAndSeedPointers(t *testing.T) {
	var req Request
	if req.FrequencyPenalty != nil || req.PresencePenalty != nil || req.Seed != nil {
		t.Fatal("unset must be nil")
	}
	fp, pp := 0.5, -0.2
	seed := int64(42)
	req.FrequencyPenalty = &fp
	req.PresencePenalty = &pp
	req.Seed = &seed
	if *req.FrequencyPenalty != 0.5 || *req.PresencePenalty != -0.2 || *req.Seed != 42 {
		t.Fatalf("%+v", req)
	}
}

func TestParallelToolCallsUnset(t *testing.T) {
	var req Request
	if req.ParallelToolCalls != nil {
		t.Fatal("unset ParallelToolCalls must be nil (omit on wire)")
	}
	v := false
	req.ParallelToolCalls = &v
	if *req.ParallelToolCalls {
		t.Fatal("false must be distinct from unset")
	}
}

func TestBlockAudioDocumentAndImageDetail(t *testing.T) {
	img := ImageSource{Kind: "url", Data: "https://x/a.png", Detail: "high"}
	if img.Detail != "high" {
		t.Fatal(img.Detail)
	}
	b := Block{Type: BlockAudio, Audio: &AudioSource{Kind: "base64", Format: "wav", Data: "AA=="}}
	if b.Audio.Format != "wav" {
		t.Fatal(b.Audio)
	}
	d := Block{Type: BlockDocument, Document: &DocumentSource{Kind: "file_id", Data: "file-1"}}
	if d.Document.Kind != "file_id" {
		t.Fatal(d.Document)
	}
}

func TestMaxTokensFieldConstants(t *testing.T) {
	if MaxTokensSourceTokens != "max_tokens" || MaxTokensSourceCompletionTokens != "max_completion_tokens" {
		t.Fatal("wire field names must match OpenAI")
	}
}
