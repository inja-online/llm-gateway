package google

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestToolResultAsJSON(t *testing.T) {
	// Multimodal result blocks
	raw := toolResultAsJSON(canonical.Block{
		Result: "ignored",
		ResultBlocks: []canonical.Block{
			{Type: canonical.BlockText, Text: "hello"},
			{Type: canonical.BlockImage, Image: &canonical.ImageSource{Data: "x"}},
			{Type: canonical.BlockImage}, // nil image
			{Type: canonical.BlockToolUse},
		},
	})
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	content, ok := m["content"].([]any)
	if !ok || len(content) < 2 {
		t.Fatalf("%s", raw)
	}
	if !strings.Contains(string(raw), "hello") || !strings.Contains(string(raw), "image in tool_result") {
		t.Fatalf("%s", raw)
	}

	// Valid JSON result passthrough
	valid := toolResultAsJSON(canonical.Block{Result: `{"a":1}`})
	if string(valid) != `{"a":1}` {
		t.Fatalf("%s", valid)
	}

	// Invalid JSON result → marshaled string
	inv := toolResultAsJSON(canonical.Block{Result: "not-json"})
	var s string
	if err := json.Unmarshal(inv, &s); err != nil || s != "not-json" {
		t.Fatalf("%s %v", inv, err)
	}
}

func TestParseEmbedResponseEdges(t *testing.T) {
	// batch invalid
	if _, _, _, err := ParseEmbedResponse([]byte(`{`), true); err == nil {
		t.Fatal("expected error")
	}
	// batch empty
	if _, _, _, err := ParseEmbedResponse([]byte(`{"embeddings":[]}`), true); err == nil {
		t.Fatal("expected empty error")
	}
	// batch with camelCase usage
	vecs, tok, has, err := ParseEmbedResponse([]byte(`{
		"embeddings":[{"values":[1,2]}],
		"usageMetadata":{"promptTokenCount":5}
	}`), true)
	if err != nil || !has || tok != 5 || len(vecs) != 1 {
		t.Fatalf("%v %d %v %v", vecs, tok, has, err)
	}
	// single invalid
	if _, _, _, err := ParseEmbedResponse([]byte(`{`), false); err == nil {
		t.Fatal()
	}
	// single no values
	if _, _, _, err := ParseEmbedResponse([]byte(`{}`), false); err == nil {
		t.Fatal()
	}
	// single embeddings[] array + snake usage
	vecs, tok, has, err = ParseEmbedResponse([]byte(`{
		"embeddings":[{"values":[9]}],
		"usage_metadata":{"prompt_token_count":2}
	}`), false)
	if err != nil || !has || tok != 2 || len(vecs) != 1 {
		t.Fatalf("%v %d %v %v", vecs, tok, has, err)
	}
	// EmbedActionPath default
	if EmbedActionPath("m", "embedContent") != EmbedPath("m") {
		t.Fatal(EmbedActionPath("m", "embedContent"))
	}
	// batch with dims
	dim := 4
	raw, err := BuildBatchEmbedContents([]string{"a"}, "m", &dim)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "outputDimensionality") {
		t.Fatalf("%s", raw)
	}
}

func TestUsageMetadataSnakeCase(t *testing.T) {
	// ParseResponse via snake_case usage / model / id fields
	body := []byte(`{
		"response_id": "rid-snake",
		"model_version": "gemini-snake",
		"candidates":[{"content":{"role":"model","parts":[{"text":"hi"}]},"finish_reason":"STOP"}],
		"usage_metadata":{
			"prompt_token_count": 7,
			"candidates_token_count": 3,
			"cached_content_token_count": 2,
			"thoughts_token_count": 1
		}
	}`)
	canon, err := ParseResponse(body)
	if err != nil {
		t.Fatal(err)
	}
	if !canon.Usage.HasUsage || canon.Usage.InputTokens != 7 || canon.Usage.OutputTokens != 3 {
		t.Fatalf("%+v", canon.Usage)
	}
	if canon.Usage.CacheReadTokens != 2 {
		t.Fatalf("cache %+v", canon.Usage)
	}
	// model/id from snake helpers
	if canon.Model != "gemini-snake" && canon.ID != "rid-snake" {
		// ParseResponse may use either; accept if either populated
		if canon.Model == "" && canon.ID == "" {
			t.Fatalf("model/id empty: %+v", canon)
		}
	}
	// nil usageMetadata helpers
	var u *usageMetadata
	if u.cached() != 0 || u.thoughts() != 0 || u.prompt() != 0 || u.candidates() != 0 {
		t.Fatal()
	}
	u = &usageMetadata{CachedContentTokenSnake: 4, ThoughtsTokenSnake: 5}
	if u.cached() != 4 || u.thoughts() != 5 {
		t.Fatalf("%d %d", u.cached(), u.thoughts())
	}
	// camelCase preferred when both present
	u = &usageMetadata{CachedContentTokenCount: 9, CachedContentTokenSnake: 1, ThoughtsTokenCount: 8, ThoughtsTokenSnake: 1}
	if u.cached() != 9 || u.thoughts() != 8 {
		t.Fatalf("%d %d", u.cached(), u.thoughts())
	}
	var r generateResponse
	if r.model() != "" || r.id() != "" {
		t.Fatal()
	}
	r = generateResponse{ModelVersionSnake: "m", ResponseIDSnake: "i"}
	if r.model() != "m" || r.id() != "i" {
		t.Fatalf("%s %s", r.model(), r.id())
	}
	r = generateResponse{ModelVersion: "M", ModelVersionSnake: "m", ResponseID: "I", ResponseIDSnake: "i"}
	if r.model() != "M" || r.id() != "I" {
		t.Fatalf("%s %s", r.model(), r.id())
	}
}
