package canonical

import (
	"encoding/json"
	"testing"
)

func ptr[T any](v T) *T { return &v }

// --- #27 ResponseFormat ---

func TestResponseFormatConstants(t *testing.T) {
	if ResponseFormatText != "text" ||
		ResponseFormatJSONObject != "json_object" ||
		ResponseFormatJSONSchema != "json_schema" {
		t.Fatalf("unexpected ResponseFormat constants: %q %q %q",
			ResponseFormatText, ResponseFormatJSONObject, ResponseFormatJSONSchema)
	}
}

func TestResponseFormatNilMeansUnset(t *testing.T) {
	var req Request
	if req.ResponseFormat != nil {
		t.Fatal("zero Request.ResponseFormat must be nil (unset)")
	}
}

func TestResponseFormatJSONObject(t *testing.T) {
	req := Request{
		Model: "m",
		ResponseFormat: &ResponseFormat{
			Kind: ResponseFormatJSONObject,
		},
	}
	if req.ResponseFormat == nil || req.ResponseFormat.Kind != ResponseFormatJSONObject {
		t.Fatalf("got %+v", req.ResponseFormat)
	}
	if req.ResponseFormat.Schema != nil || req.ResponseFormat.Strict != nil {
		t.Fatal("json_object should not require schema/strict")
	}
}

func TestResponseFormatJSONSchema(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}}}`)
	req := Request{
		ResponseFormat: &ResponseFormat{
			Kind:        ResponseFormatJSONSchema,
			Name:        "answer",
			Description: "structured answer",
			Schema:      schema,
			Strict:      ptr(true),
		},
	}
	rf := req.ResponseFormat
	if rf.Kind != ResponseFormatJSONSchema || rf.Name != "answer" || rf.Description != "structured answer" {
		t.Fatalf("got %+v", rf)
	}
	if string(rf.Schema) != string(schema) {
		t.Fatalf("schema: %s", rf.Schema)
	}
	if rf.Strict == nil || !*rf.Strict {
		t.Fatal("want Strict=true")
	}
}

func TestResponseFormatJSONRoundTrip(t *testing.T) {
	// Canonical types are not themselves wire-JSON tagged, but fields must be
	// stable when held in-process and when schema blobs are preserved.
	in := &ResponseFormat{
		Kind:   ResponseFormatJSONSchema,
		Name:   "n",
		Schema: json.RawMessage(`{"type":"object"}`),
		Strict: ptr(false),
	}
	// Simulate storing schema through a marshal/unmarshal of the schema blob only.
	var schema json.RawMessage
	if err := json.Unmarshal(in.Schema, &schema); err != nil {
		t.Fatal(err)
	}
	out := &ResponseFormat{
		Kind:   in.Kind,
		Name:   in.Name,
		Schema: schema,
		Strict: in.Strict,
	}
	if out.Kind != in.Kind || out.Name != in.Name || string(out.Schema) != string(in.Schema) {
		t.Fatalf("schema fidelity lost: %+v vs %+v", out, in)
	}
	if out.Strict == nil || *out.Strict != false {
		t.Fatal("strict false must be distinguishable from nil")
	}
}

// --- #29 ThinkingConfig ---

func TestThinkingConfigNilMeansUnset(t *testing.T) {
	var req Request
	if req.Thinking != nil {
		t.Fatal("zero Request.Thinking must be nil")
	}
}

func TestThinkingConfigExplicitDisable(t *testing.T) {
	req := Request{Thinking: &ThinkingConfig{Type: "disabled"}}
	if req.Thinking == nil || req.Thinking.Type != "disabled" {
		t.Fatalf("got %+v", req.Thinking)
	}
	if req.Thinking.BudgetTokens != nil || req.Thinking.Effort != "" {
		t.Fatal("explicit disable should not invent budget/effort")
	}
}

func TestThinkingConfigBudgetAndEffort(t *testing.T) {
	req := Request{
		Thinking: &ThinkingConfig{
			Type:            "enabled",
			Effort:          "high",
			BudgetTokens:    ptr(8000),
			IncludeThoughts: ptr(true),
		},
	}
	tc := req.Thinking
	if tc.Type != "enabled" || tc.Effort != "high" {
		t.Fatalf("got %+v", tc)
	}
	if tc.BudgetTokens == nil || *tc.BudgetTokens != 8000 {
		t.Fatalf("budget: %v", tc.BudgetTokens)
	}
	if tc.IncludeThoughts == nil || !*tc.IncludeThoughts {
		t.Fatal("want IncludeThoughts=true")
	}
}

func TestThinkingConfigZeroBudgetDistinctFromNil(t *testing.T) {
	req := Request{Thinking: &ThinkingConfig{BudgetTokens: ptr(0)}}
	if req.Thinking.BudgetTokens == nil || *req.Thinking.BudgetTokens != 0 {
		t.Fatal("BudgetTokens=0 must be distinguishable from unset")
	}
}

func TestThinkingConfigAdaptive(t *testing.T) {
	req := Request{Thinking: &ThinkingConfig{Type: "adaptive", Effort: "medium"}}
	if req.Thinking.Type != "adaptive" || req.Thinking.Effort != "medium" {
		t.Fatalf("got %+v", req.Thinking)
	}
}

// --- #32 BlockDocument ---

func TestBlockDocumentConstant(t *testing.T) {
	if BlockDocument != "document" {
		t.Fatalf("BlockDocument=%q", BlockDocument)
	}
}

func TestBlockDocumentBase64(t *testing.T) {
	b := Block{
		Type: BlockDocument,
		Document: &DocumentSource{
			Kind:      "base64",
			MediaType: "application/pdf",
			Data:      "JVBERi0x",
			Filename:  "doc.pdf",
			Title:     "Spec",
		},
	}
	if b.Type != BlockDocument || b.Document == nil {
		t.Fatal()
	}
	if b.Document.Kind != "base64" || b.Document.MediaType != "application/pdf" {
		t.Fatalf("%+v", b.Document)
	}
	if b.Document.Filename != "doc.pdf" || b.Document.Title != "Spec" {
		t.Fatalf("%+v", b.Document)
	}
}

func TestBlockDocumentURLAndFileURI(t *testing.T) {
	urlDoc := DocumentSource{Kind: "url", MediaType: "application/pdf", Data: "https://example.com/a.pdf"}
	uriDoc := DocumentSource{Kind: "file_uri", MediaType: "application/pdf", Data: "files/abc"}
	if urlDoc.Kind != "url" || uriDoc.Kind != "file_uri" {
		t.Fatal()
	}
	msg := Message{
		Role: RoleUser,
		Content: []Block{
			{Type: BlockDocument, Document: &urlDoc},
			{Type: BlockDocument, Document: &uriDoc},
		},
	}
	if len(msg.Content) != 2 {
		t.Fatal()
	}
}

// --- #33 BlockAudio ---

func TestBlockAudioConstant(t *testing.T) {
	if BlockAudio != "audio" {
		t.Fatalf("BlockAudio=%q", BlockAudio)
	}
}

func TestBlockAudioInputOnlyConstruct(t *testing.T) {
	// Chat input audio: construct without panicking; transcript optional.
	b := Block{
		Type: BlockAudio,
		Audio: &AudioSource{
			Kind:       "base64",
			MediaType:  "audio/wav",
			Data:       "UklGRg==",
			Transcript: "hello",
		},
	}
	if b.Audio == nil || b.Audio.Kind != "base64" || b.Audio.Transcript != "hello" {
		t.Fatalf("%+v", b.Audio)
	}
	urlAudio := AudioSource{Kind: "url", MediaType: "audio/mpeg", Data: "https://example.com/a.mp3"}
	if urlAudio.Kind != "url" {
		t.Fatal()
	}
	req := Request{
		Messages: []Message{{
			Role:    RoleUser,
			Content: []Block{b, {Type: BlockAudio, Audio: &urlAudio}},
		}},
	}
	if len(req.Messages[0].Content) != 2 {
		t.Fatal()
	}
}

// --- #37 TopK ---

func TestTopKNilUnset(t *testing.T) {
	var req Request
	if req.TopK != nil {
		t.Fatal()
	}
}

func TestTopKSet(t *testing.T) {
	req := Request{TopK: ptr(40)}
	if req.TopK == nil || *req.TopK != 40 {
		t.Fatalf("%v", req.TopK)
	}
}

// --- #38 FrequencyPenalty / PresencePenalty ---

func TestPenaltiesNilUnset(t *testing.T) {
	var req Request
	if req.FrequencyPenalty != nil || req.PresencePenalty != nil {
		t.Fatal()
	}
}

func TestPenaltiesSet(t *testing.T) {
	req := Request{
		FrequencyPenalty: ptr(0.5),
		PresencePenalty:  ptr(-0.25),
	}
	if req.FrequencyPenalty == nil || *req.FrequencyPenalty != 0.5 {
		t.Fatal()
	}
	if req.PresencePenalty == nil || *req.PresencePenalty != -0.25 {
		t.Fatal()
	}
}

func TestPenaltiesZeroDistinctFromNil(t *testing.T) {
	req := Request{FrequencyPenalty: ptr(0.0), PresencePenalty: ptr(0.0)}
	if req.FrequencyPenalty == nil || req.PresencePenalty == nil {
		t.Fatal("zero penalties must be set pointers")
	}
}

// --- #39 Seed ---

func TestSeedNilUnset(t *testing.T) {
	var req Request
	if req.Seed != nil {
		t.Fatal()
	}
}

func TestSeedSet(t *testing.T) {
	req := Request{Seed: ptr(int64(42))}
	if req.Seed == nil || *req.Seed != 42 {
		t.Fatal()
	}
}

func TestSeedZeroDistinctFromNil(t *testing.T) {
	req := Request{Seed: ptr(int64(0))}
	if req.Seed == nil || *req.Seed != 0 {
		t.Fatal()
	}
}

// --- #40 ParallelToolCalls ---

func TestParallelToolCallsNilUnset(t *testing.T) {
	var req Request
	if req.ParallelToolCalls != nil {
		t.Fatal()
	}
}

func TestParallelToolCallsTrueFalse(t *testing.T) {
	on := Request{ParallelToolCalls: ptr(true)}
	off := Request{ParallelToolCalls: ptr(false)}
	if on.ParallelToolCalls == nil || !*on.ParallelToolCalls {
		t.Fatal("want true")
	}
	if off.ParallelToolCalls == nil || *off.ParallelToolCalls {
		t.Fatal("want false (distinct from nil)")
	}
}

// --- #44 ImageSource.Detail ---

func TestImageSourceDetailEmptyUnset(t *testing.T) {
	img := ImageSource{Kind: "url", Data: "https://example.com/i.png"}
	if img.Detail != "" {
		t.Fatal("empty Detail must mean unset; do not default")
	}
}

func TestImageSourceDetailValues(t *testing.T) {
	for _, d := range []string{"auto", "low", "high"} {
		img := ImageSource{Kind: "url", Data: "u", Detail: d}
		if img.Detail != d {
			t.Fatalf("want %q got %q", d, img.Detail)
		}
	}
	// Pass-through unknown detail strings is allowed (no validation here).
	img := ImageSource{Detail: "original"}
	if img.Detail != "original" {
		t.Fatal()
	}
}

// --- #50 MaxTokensField ---

func TestMaxTokensFieldConstants(t *testing.T) {
	if MaxTokensFieldMaxTokens != "max_tokens" ||
		MaxTokensFieldMaxCompletionTokens != "max_completion_tokens" {
		t.Fatal()
	}
}

func TestMaxTokensFieldEmptyDefault(t *testing.T) {
	var req Request
	if req.MaxTokensField != "" {
		t.Fatal("empty MaxTokensField means unset / not applicable")
	}
	req.MaxTokens = 1024
	if req.MaxTokens != 1024 || req.MaxTokensField != "" {
		t.Fatal("numeric MaxTokens independent of field source")
	}
}

func TestMaxTokensFieldSources(t *testing.T) {
	req := Request{MaxTokens: 2048, MaxTokensField: MaxTokensFieldMaxTokens}
	if req.MaxTokensField != "max_tokens" {
		t.Fatal()
	}
	req2 := Request{MaxTokens: 4096, MaxTokensField: MaxTokensFieldMaxCompletionTokens}
	if req2.MaxTokensField != "max_completion_tokens" || req2.MaxTokens != 4096 {
		t.Fatal()
	}
}

// --- #48 Redacted thinking (field already exists; document + construct) ---

func TestBlockThinkingRedacted(t *testing.T) {
	b := Block{
		Type:     BlockThinking,
		Text:     "opaque-redacted-payload",
		Redacted: true,
	}
	if b.Type != BlockThinking || !b.Redacted {
		t.Fatal()
	}
	plain := Block{Type: BlockThinking, Text: "visible", Signature: "sig", Redacted: false}
	if plain.Redacted {
		t.Fatal()
	}
}

// --- Kitchen sink: all new Request fields together ---

func TestRequestKitchenSinkNewFields(t *testing.T) {
	schema := json.RawMessage(`{"type":"object"}`)
	req := Request{
		Model:             "gpt-test",
		MaxTokens:         1000,
		MaxTokensField:    MaxTokensFieldMaxCompletionTokens,
		TopK:              ptr(32),
		FrequencyPenalty:  ptr(0.1),
		PresencePenalty:   ptr(0.2),
		Seed:              ptr(int64(7)),
		ParallelToolCalls: ptr(false),
		ResponseFormat: &ResponseFormat{
			Kind:   ResponseFormatJSONSchema,
			Name:   "out",
			Schema: schema,
			Strict: ptr(true),
		},
		Thinking: &ThinkingConfig{
			Type:            "enabled",
			Effort:          "low",
			BudgetTokens:    ptr(512),
			IncludeThoughts: ptr(false),
		},
		Messages: []Message{{
			Role: RoleUser,
			Content: []Block{
				{Type: BlockText, Text: "hi"},
				{Type: BlockImage, Image: &ImageSource{Kind: "url", Data: "https://x", Detail: "high"}},
				{Type: BlockDocument, Document: &DocumentSource{Kind: "base64", MediaType: "application/pdf", Data: "pdf"}},
				{Type: BlockAudio, Audio: &AudioSource{Kind: "base64", MediaType: "audio/wav", Data: "au"}},
				{Type: BlockThinking, Text: "secret", Redacted: true},
			},
		}},
	}
	if req.MaxTokensField != MaxTokensFieldMaxCompletionTokens {
		t.Fatal()
	}
	if req.ResponseFormat == nil || req.Thinking == nil {
		t.Fatal()
	}
	if len(req.Messages[0].Content) != 5 {
		t.Fatalf("content len %d", len(req.Messages[0].Content))
	}
	img := req.Messages[0].Content[1]
	if img.Image == nil || img.Image.Detail != "high" {
		t.Fatal()
	}
	if !req.Messages[0].Content[4].Redacted {
		t.Fatal()
	}
}

func TestBlockTypeSet(t *testing.T) {
	// Ensure new block types are distinct string values.
	seen := map[BlockType]bool{}
	for _, bt := range []BlockType{
		BlockText, BlockImage, BlockToolUse, BlockToolResult,
		BlockThinking, BlockDocument, BlockAudio,
	} {
		if seen[bt] {
			t.Fatalf("duplicate BlockType %q", bt)
		}
		seen[bt] = true
		if bt == "" {
			t.Fatal("empty block type")
		}
	}
}
