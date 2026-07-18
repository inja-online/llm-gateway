package google

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
	googleegress "github.com/inja-online/llm-gateway/egress/google"
)

// --- #28 responseMimeType / responseSchema ---

func TestParseResponseMIMEJSONObject(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"g",
		"contents":[{"parts":[{"text":"x"}]}],
		"generation_config":{"response_mime_type":"application/json"}
	}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if req.ResponseFormat == nil || req.ResponseFormat.Kind != canonical.ResponseFormatJSONObject {
		t.Fatalf("%+v", req.ResponseFormat)
	}
}

func TestParseResponseSchemaJSONSchema(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"g",
		"contents":[{"parts":[{"text":"x"}]}],
		"generation_config":{
			"response_mime_type":"application/json",
			"response_schema":{"type":"object","properties":{"n":{"type":"integer"}}}
		}
	}`), "")
	if err != nil {
		t.Fatal(err)
	}
	rf := req.ResponseFormat
	if rf == nil || rf.Kind != canonical.ResponseFormatJSONSchema {
		t.Fatalf("%+v", rf)
	}
	if !strings.Contains(string(rf.Schema), `"n"`) {
		t.Fatalf("schema %s", rf.Schema)
	}
}

func TestParseResponseJsonSchemaCamel(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"g",
		"contents":[{"parts":[{"text":"x"}]}],
		"generationConfig":{
			"responseMimeType":"application/json",
			"responseJsonSchema":{"type":"object","properties":{"ok":{"type":"boolean"}}}
		}
	}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if req.ResponseFormat == nil || req.ResponseFormat.Kind != canonical.ResponseFormatJSONSchema {
		t.Fatalf("%+v", req.ResponseFormat)
	}
	if !strings.Contains(string(req.ResponseFormat.Schema), `"ok"`) {
		t.Fatalf("%s", req.ResponseFormat.Schema)
	}
}

// --- #30 thinkingConfig ---

func TestParseThinkingConfigBudgetAndInclude(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"g",
		"contents":[{"parts":[{"text":"x"}]}],
		"generation_config":{
			"thinking_config":{"thinking_budget":4096,"include_thoughts":true}
		}
	}`), "")
	if err != nil {
		t.Fatal(err)
	}
	tc := req.Thinking
	if tc == nil || tc.Type != "enabled" {
		t.Fatalf("%+v", tc)
	}
	if tc.BudgetTokens == nil || *tc.BudgetTokens != 4096 {
		t.Fatalf("budget %+v", tc.BudgetTokens)
	}
	if tc.IncludeThoughts == nil || !*tc.IncludeThoughts {
		t.Fatalf("include %+v", tc.IncludeThoughts)
	}
}

func TestParseThinkingConfigDisabledAndLevel(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"g",
		"contents":[{"parts":[{"text":"x"}]}],
		"generationConfig":{
			"thinkingConfig":{"thinkingBudget":0}
		}
	}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if req.Thinking == nil || req.Thinking.Type != "disabled" {
		t.Fatalf("%+v", req.Thinking)
	}

	req2, err := ParseRequest([]byte(`{
		"model":"g",
		"contents":[{"parts":[{"text":"x"}]}],
		"generation_config":{
			"thinking_config":{"thinking_level":"HIGH"}
		}
	}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if req2.Thinking == nil || req2.Thinking.Effort != "high" || req2.Thinking.Type != "enabled" {
		t.Fatalf("%+v", req2.Thinking)
	}
}

func TestParseNoThinkingDoesNotDefault(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"g",
		"contents":[{"parts":[{"text":"x"}]}],
		"generation_config":{"temperature":0.5}
	}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if req.Thinking != nil {
		t.Fatalf("want nil thinking, got %+v", req.Thinking)
	}
}

// --- #37 top_k / #39 seed ---

func TestParseTopKAndSeed(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"g",
		"contents":[{"parts":[{"text":"x"}]}],
		"generation_config":{"top_k":40,"seed":7}
	}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if req.TopK == nil || *req.TopK != 40 {
		t.Fatalf("topK %+v", req.TopK)
	}
	if req.Seed == nil || *req.Seed != 7 {
		t.Fatalf("seed %+v", req.Seed)
	}
}

func TestParseTopKCamel(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"g",
		"contents":[{"parts":[{"text":"x"}]}],
		"generationConfig":{"topK":12,"seed":0}
	}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if req.TopK == nil || *req.TopK != 12 {
		t.Fatalf("%v", req.TopK)
	}
	if req.Seed == nil || *req.Seed != 0 {
		t.Fatalf("seed 0 must be distinct from nil: %+v", req.Seed)
	}
}

// --- Google → canonical → Google round-trip ---

func TestGoogleToCanonicalToGoogleFidelity(t *testing.T) {
	in := []byte(`{
		"model":"gemini-2.0-flash",
		"contents":[{"role":"user","parts":[
			{"text":"summarize"},
			{"inline_data":{"mime_type":"application/pdf","data":"JVBERi0="}}
		]}],
		"generation_config":{
			"temperature":0.2,
			"top_p":0.9,
			"top_k":32,
			"seed":99,
			"max_output_tokens":256,
			"response_mime_type":"application/json",
			"response_schema":{"type":"object","properties":{"s":{"type":"string"}}},
			"thinking_config":{"thinking_budget":2048,"include_thoughts":true}
		}
	}`)
	req, err := ParseRequest(in, "")
	if err != nil {
		t.Fatal(err)
	}
	// Spot-check canonical
	if req.TopK == nil || *req.TopK != 32 {
		t.Fatalf("topK %+v", req.TopK)
	}
	if req.Seed == nil || *req.Seed != 99 {
		t.Fatalf("seed %+v", req.Seed)
	}
	if req.ResponseFormat == nil || req.ResponseFormat.Kind != canonical.ResponseFormatJSONSchema {
		t.Fatalf("rf %+v", req.ResponseFormat)
	}
	if req.Thinking == nil || req.Thinking.BudgetTokens == nil || *req.Thinking.BudgetTokens != 2048 {
		t.Fatalf("thinking %+v", req.Thinking)
	}
	if len(req.Messages) != 1 || len(req.Messages[0].Content) != 2 {
		t.Fatalf("messages %+v", req.Messages)
	}
	if req.Messages[0].Content[1].Type != canonical.BlockDocument {
		t.Fatalf("want document, got %+v", req.Messages[0].Content[1])
	}

	out, err := googleegress.BuildRequest(req, "gemini-2.0-flash")
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(out, &wire); err != nil {
		t.Fatal(err)
	}
	gc, _ := wire["generation_config"].(map[string]any)
	if gc == nil {
		t.Fatalf("missing generation_config: %s", out)
	}
	if gc["top_k"] != float64(32) {
		t.Fatalf("top_k: %v", gc["top_k"])
	}
	if gc["seed"] != float64(99) {
		t.Fatalf("seed: %v", gc["seed"])
	}
	if gc["response_mime_type"] != "application/json" {
		t.Fatalf("mime: %v", gc["response_mime_type"])
	}
	if gc["response_schema"] == nil {
		t.Fatalf("missing response_schema: %s", out)
	}
	tc, _ := gc["thinking_config"].(map[string]any)
	if tc == nil || tc["thinking_budget"] != float64(2048) {
		t.Fatalf("thinking_config: %v", tc)
	}
	if tc["include_thoughts"] != true {
		t.Fatalf("include_thoughts: %v", tc)
	}
	// PDF re-emitted as inline_data
	contents := wire["contents"].([]any)
	parts := contents[0].(map[string]any)["parts"].([]any)
	if len(parts) != 2 {
		t.Fatalf("parts: %v", parts)
	}
	id := parts[1].(map[string]any)["inline_data"].(map[string]any)
	if id["mime_type"] != "application/pdf" || id["data"] != "JVBERi0=" {
		t.Fatalf("pdf part: %v", id)
	}
}

func TestGoogleRoundTripOmitsUnsetOptional(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"g",
		"contents":[{"parts":[{"text":"hi"}]}]
	}`), "")
	if err != nil {
		t.Fatal(err)
	}
	out, err := googleegress.BuildRequest(req, "g")
	if err != nil {
		t.Fatal(err)
	}
	raw := string(out)
	for _, k := range []string{
		"top_k", "seed", "response_mime_type", "response_schema", "thinking_config",
	} {
		if strings.Contains(raw, k) {
			t.Errorf("must omit unset %s: %s", k, raw)
		}
	}
}
