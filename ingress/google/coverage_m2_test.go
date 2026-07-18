package google

import (
	"encoding/json"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestThinkingLevelToEffortAll(t *testing.T) {
	cases := map[string]string{
		"MINIMAL":               "minimal",
		"THINKING_LEVEL_MINIMAL": "minimal",
		"LOW":                   "low",
		"THINKING_LEVEL_LOW":    "low",
		"MEDIUM":                "medium",
		"THINKING_LEVEL_MEDIUM": "medium",
		"HIGH":                  "high",
		"THINKING_LEVEL_HIGH":   "high",
		"  high  ":              "high",
		"custom":                "custom",
		"":                      "",
	}
	for in, want := range cases {
		if got := thinkingLevelToEffort(in); got != want {
			t.Fatalf("%q → %q want %q", in, got, want)
		}
	}
}

func TestParseThinkingConfigBranches(t *testing.T) {
	if parseThinkingConfig(nil) != nil {
		t.Fatal("nil")
	}
	// empty wire
	if parseThinkingConfig(&thinkingConfigWire{}) != nil {
		t.Fatal("empty")
	}
	// budget 0 → disabled
	z := 0
	tc := parseThinkingConfig(&thinkingConfigWire{ThinkingBudget: &z})
	if tc == nil || tc.Type != "disabled" {
		t.Fatalf("%+v", tc)
	}
	// budget >0 → enabled
	b := 100
	tc = parseThinkingConfig(&thinkingConfigWire{ThinkingBudget: &b})
	if tc == nil || tc.Type != "enabled" || tc.BudgetTokens == nil || *tc.BudgetTokens != 100 {
		t.Fatalf("%+v", tc)
	}
	// level only → enabled + effort
	tc = parseThinkingConfig(&thinkingConfigWire{ThinkingLevel: "LOW"})
	if tc == nil || tc.Type != "enabled" || tc.Effort != "low" {
		t.Fatalf("%+v", tc)
	}
	// include only (no type)
	yes := true
	tc = parseThinkingConfig(&thinkingConfigWire{IncludeThoughts: &yes})
	if tc == nil || tc.Type != "" || tc.IncludeThoughts == nil || !*tc.IncludeThoughts {
		t.Fatalf("%+v", tc)
	}
	// camel fields
	tc = parseThinkingConfig(&thinkingConfigWire{
		ThinkingBudgetCamel:  &b,
		IncludeThoughtsCamel: &yes,
		ThinkingLevelCamel:   "MEDIUM",
	})
	if tc == nil || tc.Effort != "medium" || *tc.BudgetTokens != 100 {
		t.Fatalf("camel %+v", tc)
	}
}

func TestParseResponseFormatBranches(t *testing.T) {
	if parseResponseFormat(nil) != nil {
		t.Fatal("nil")
	}
	// empty
	if parseResponseFormat(&generationConfig{}) != nil {
		t.Fatal("empty")
	}
	// text/plain
	rf := parseResponseFormat(&generationConfig{ResponseMIMEType: "text/plain"})
	if rf == nil || rf.Kind != canonical.ResponseFormatText {
		t.Fatalf("%+v", rf)
	}
	// text/x.enum → json_object
	rf = parseResponseFormat(&generationConfig{ResponseMIMEType: "text/x.enum"})
	if rf == nil || rf.Kind != canonical.ResponseFormatJSONObject {
		t.Fatalf("%+v", rf)
	}
	// unknown mime → nil
	if parseResponseFormat(&generationConfig{ResponseMIMEType: "image/png"}) != nil {
		t.Fatal("unknown mime")
	}
	// schema null string treated as no schema
	rf = parseResponseFormat(&generationConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema:   json.RawMessage(`null`),
	})
	if rf == nil || rf.Kind != canonical.ResponseFormatJSONObject {
		t.Fatalf("null schema → json_object: %+v", rf)
	}
	// camel mime + response_json_schema snake
	rf = parseResponseFormat(&generationConfig{
		ResponseMIMETypeCamel: "application/json",
		ResponseJSONSchema:    json.RawMessage(`{"type":"string"}`),
	})
	if rf == nil || rf.Kind != canonical.ResponseFormatJSONSchema {
		t.Fatalf("%+v", rf)
	}
}

func TestWireNilHelpers(t *testing.T) {
	// nil receivers
	var b *blob
	if b.mime() != "" {
		t.Fatal("blob nil")
	}
	var f *fileData
	if f.mime() != "" || f.uri() != "" {
		t.Fatal("fileData nil")
	}
	var gc *generationConfig
	if gc.topP() != nil || gc.maxOutputTokens() != 0 || gc.stopSequences() != nil ||
		gc.topK() != nil || gc.responseMIMEType() != "" || gc.responseSchema() != nil ||
		gc.thinking() != nil {
		t.Fatal("gc nil helpers")
	}
	var tw *thinkingConfigWire
	if tw.includeThoughts() != nil || tw.thinkingBudget() != nil || tw.thinkingLevel() != "" {
		t.Fatal("tw nil")
	}

	// camel preference when snake empty
	f = &fileData{MIMETypeCamel: "image/webp", FileURICamel: "gs://b/o"}
	if f.mime() != "image/webp" || f.uri() != "gs://b/o" {
		t.Fatalf("%+v", f)
	}
	b2 := &blob{MIMETypeCamel: "image/gif"}
	if b2.mime() != "image/gif" {
		t.Fatal(b2.mime())
	}

	// stopSequences camel
	gc2 := &generationConfig{StopSequencesCamel: []string{"END"}}
	if got := gc2.stopSequences(); len(got) != 1 || got[0] != "END" {
		t.Fatalf("%v", got)
	}
	// snake wins
	gc2.StopSequences = []string{"S"}
	if got := gc2.stopSequences(); got[0] != "S" {
		t.Fatalf("%v", got)
	}

	// responseSchema priority chain
	gc3 := &generationConfig{ResponseJSONSchemaCamel: json.RawMessage(`{"a":1}`)}
	if string(gc3.responseSchema()) != `{"a":1}` {
		t.Fatalf("%s", gc3.responseSchema())
	}
	gc3.ResponseJSONSchema = json.RawMessage(`{"b":2}`)
	if string(gc3.responseSchema()) != `{"b":2}` {
		t.Fatalf("%s", gc3.responseSchema())
	}
	gc3.ResponseSchemaCamel = json.RawMessage(`{"c":3}`)
	if string(gc3.responseSchema()) != `{"c":3}` {
		t.Fatalf("%s", gc3.responseSchema())
	}
	gc3.ResponseSchema = json.RawMessage(`{"d":4}`)
	if string(gc3.responseSchema()) != `{"d":4}` {
		t.Fatalf("%s", gc3.responseSchema())
	}

	// topP/topK/max/mime camel
	tp := 0.7
	tk := 5
	gc4 := &generationConfig{
		TopPCamel:              &tp,
		TopKCamel:              &tk,
		MaxOutputTokensCamel:   50,
		ResponseMIMETypeCamel:  "text/plain",
		ThinkingConfigCamel:    &thinkingConfigWire{ThinkingLevelCamel: "HIGH"},
	}
	if gc4.topP() == nil || *gc4.topP() != 0.7 {
		t.Fatal("topP")
	}
	if gc4.topK() == nil || *gc4.topK() != 5 {
		t.Fatal("topK")
	}
	if gc4.maxOutputTokens() != 50 {
		t.Fatal("max")
	}
	if gc4.responseMIMEType() != "text/plain" {
		t.Fatal("mime")
	}
	if gc4.thinking() == nil || gc4.thinking().thinkingLevel() != "HIGH" {
		t.Fatal("thinking camel")
	}
}

func TestParseThinkingAndFormatFromBody(t *testing.T) {
	// all thinking levels via body
	for _, level := range []string{"MINIMAL", "LOW", "MEDIUM", "HIGH", "THINKING_LEVEL_LOW"} {
		body := []byte(`{
			"model":"g",
			"contents":[{"parts":[{"text":"x"}]}],
			"generation_config":{"thinking_config":{"thinking_level":"` + level + `"}}
		}`)
		req, err := ParseRequest(body, "")
		if err != nil {
			t.Fatalf("%s: %v", level, err)
		}
		if req.Thinking == nil || req.Thinking.Effort == "" {
			t.Fatalf("%s: %+v", level, req.Thinking)
		}
	}
	// stopSequences camel + text/plain format
	req, err := ParseRequest([]byte(`{
		"model":"g",
		"contents":[{"parts":[{"text":"x"}]}],
		"generationConfig":{"stopSequences":["STOP"],"responseMimeType":"text/plain"}
	}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(req.StopSequences) != 1 || req.StopSequences[0] != "STOP" {
		t.Fatalf("%v", req.StopSequences)
	}
	if req.ResponseFormat == nil || req.ResponseFormat.Kind != canonical.ResponseFormatText {
		t.Fatalf("%+v", req.ResponseFormat)
	}
	// camel file_data mime/uri
	req, err = ParseRequest([]byte(`{
		"model":"g",
		"contents":[{"parts":[{"fileData":{"mimeType":"application/pdf","fileUri":"https://x/a.pdf"}}]}]
	}`), "")
	if err != nil {
		t.Fatal(err)
	}
	doc := req.Messages[0].Content[0]
	if doc.Type != canonical.BlockDocument || doc.Document == nil || doc.Document.Data != "https://x/a.pdf" {
		t.Fatalf("%+v", doc)
	}
	// camel inline mime
	req, err = ParseRequest([]byte(`{
		"model":"g",
		"contents":[{"parts":[{"inlineData":{"mimeType":"image/png","data":"xx"}}]}]
	}`), "")
	if err != nil {
		t.Fatal(err)
	}
	img := req.Messages[0].Content[0]
	if img.Type != canonical.BlockImage || img.Image == nil || img.Image.MediaType != "image/png" {
		t.Fatalf("%+v", img)
	}
}
