package google

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func ptrFloat(v float64) *float64 { return &v }
func ptrInt(v int) *int           { return &v }
func ptrInt64(v int64) *int64     { return &v }
func ptrBool(v bool) *bool        { return &v }

// --- #28 ResponseFormat ---

func TestBuildResponseFormatJSONObject(t *testing.T) {
	body, err := BuildRequest(&canonical.Request{
		ResponseFormat: &canonical.ResponseFormat{Kind: canonical.ResponseFormatJSONObject},
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		}},
	}, "m")
	if err != nil {
		t.Fatal(err)
	}
	var out generateRequest
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out.GenerationConfig == nil || out.GenerationConfig.ResponseMIMEType != "application/json" {
		t.Fatalf("%+v", out.GenerationConfig)
	}
	if len(out.GenerationConfig.ResponseSchema) > 0 {
		t.Fatalf("json_object must not emit schema: %s", out.GenerationConfig.ResponseSchema)
	}
}

func TestBuildResponseFormatJSONSchema(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"n":{"type":"integer"}}}`)
	body, err := BuildRequest(&canonical.Request{
		ResponseFormat: &canonical.ResponseFormat{
			Kind:   canonical.ResponseFormatJSONSchema,
			Name:   "num",
			Schema: schema,
		},
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		}},
	}, "m")
	if err != nil {
		t.Fatal(err)
	}
	var out generateRequest
	json.Unmarshal(body, &out)
	gc := out.GenerationConfig
	if gc == nil || gc.ResponseMIMEType != "application/json" {
		t.Fatalf("%+v", gc)
	}
	if string(gc.ResponseSchema) != string(schema) {
		t.Fatalf("schema %s", gc.ResponseSchema)
	}
}

// --- #30 ThinkingConfig ---

func TestBuildThinkingBudgetAndInclude(t *testing.T) {
	body, err := BuildRequest(&canonical.Request{
		Thinking: &canonical.ThinkingConfig{
			Type:            "enabled",
			BudgetTokens:    ptrInt(5000),
			IncludeThoughts: ptrBool(true),
		},
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		}},
	}, "m")
	if err != nil {
		t.Fatal(err)
	}
	var out generateRequest
	json.Unmarshal(body, &out)
	tc := out.GenerationConfig.ThinkingConfig
	if tc == nil || tc.ThinkingBudget == nil || *tc.ThinkingBudget != 5000 {
		t.Fatalf("%+v", tc)
	}
	if tc.IncludeThoughts == nil || !*tc.IncludeThoughts {
		t.Fatalf("include %+v", tc.IncludeThoughts)
	}
}

func TestBuildThinkingDisabledAndEffort(t *testing.T) {
	// disabled → budget 0
	body, _ := BuildRequest(&canonical.Request{
		Thinking: &canonical.ThinkingConfig{Type: "disabled"},
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		}},
	}, "m")
	var out generateRequest
	json.Unmarshal(body, &out)
	if out.GenerationConfig.ThinkingConfig == nil ||
		out.GenerationConfig.ThinkingConfig.ThinkingBudget == nil ||
		*out.GenerationConfig.ThinkingConfig.ThinkingBudget != 0 {
		t.Fatalf("%+v", out.GenerationConfig.ThinkingConfig)
	}

	// effort high without budget → mapped budget
	body, _ = BuildRequest(&canonical.Request{
		Thinking: &canonical.ThinkingConfig{Effort: "high"},
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		}},
	}, "m")
	out = generateRequest{}
	json.Unmarshal(body, &out)
	if out.GenerationConfig.ThinkingConfig == nil ||
		out.GenerationConfig.ThinkingConfig.ThinkingBudget == nil ||
		*out.GenerationConfig.ThinkingConfig.ThinkingBudget != 16384 {
		t.Fatalf("high effort: %+v", out.GenerationConfig.ThinkingConfig)
	}

	// nil thinking: omit
	body, _ = BuildRequest(&canonical.Request{
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		}},
	}, "m")
	if strings.Contains(string(body), "thinking_config") {
		t.Fatalf("must omit thinking: %s", body)
	}
}

// --- #37 top_k / #39 seed ---

func TestBuildTopKAndSeed(t *testing.T) {
	body, err := BuildRequest(&canonical.Request{
		TopK: ptrInt(40),
		Seed: ptrInt64(7),
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		}},
	}, "m")
	if err != nil {
		t.Fatal(err)
	}
	var out generateRequest
	json.Unmarshal(body, &out)
	if out.GenerationConfig.TopK == nil || *out.GenerationConfig.TopK != 40 {
		t.Fatalf("topK %+v", out.GenerationConfig.TopK)
	}
	if out.GenerationConfig.Seed == nil || *out.GenerationConfig.Seed != 7 {
		t.Fatalf("seed %+v", out.GenerationConfig.Seed)
	}
	// unset omits
	body, _ = BuildRequest(&canonical.Request{
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		}},
	}, "m")
	raw := string(body)
	if strings.Contains(raw, "top_k") || strings.Contains(raw, `"seed"`) {
		t.Fatalf("unset top_k/seed must omit: %s", raw)
	}
}

// --- #32 BlockDocument ---

func TestBuildDocumentBlockPDF(t *testing.T) {
	body, err := BuildRequest(&canonical.Request{
		Messages: []canonical.Message{{
			Role: canonical.RoleUser,
			Content: []canonical.Block{{
				Type: canonical.BlockDocument,
				Document: &canonical.DocumentSource{
					Kind: "base64", MediaType: "application/pdf", Data: "JVBERi0=",
				},
			}},
		}},
	}, "m")
	if err != nil {
		t.Fatal(err)
	}
	var out generateRequest
	json.Unmarshal(body, &out)
	if len(out.Contents) != 1 || len(out.Contents[0].Parts) != 1 {
		t.Fatalf("%+v", out.Contents)
	}
	p := out.Contents[0].Parts[0]
	if p.InlineData == nil || p.InlineData.MIMEType != "application/pdf" || p.InlineData.Data != "JVBERi0=" {
		t.Fatalf("%+v", p)
	}
}

func TestBuildDocumentFileURI(t *testing.T) {
	body, err := BuildRequest(&canonical.Request{
		Messages: []canonical.Message{{
			Role: canonical.RoleUser,
			Content: []canonical.Block{{
				Type: canonical.BlockDocument,
				Document: &canonical.DocumentSource{
					Kind: "file_uri", MediaType: "application/pdf",
					Data: "https://generativelanguage.googleapis.com/v1beta/files/abc",
				},
			}},
		}},
	}, "m")
	if err != nil {
		t.Fatal(err)
	}
	var out generateRequest
	json.Unmarshal(body, &out)
	fd := out.Contents[0].Parts[0].FileData
	if fd == nil || fd.MIMEType != "application/pdf" || !strings.Contains(fd.FileURI, "files/abc") {
		t.Fatalf("%+v", fd)
	}
}

// OpenAI-shaped canonical → Google egress for format + thinking (cross-dialect cell).
func TestOpenAIStyleCanonicalToGoogleFormatThinking(t *testing.T) {
	// Mirrors what OpenAI ingress would produce for response_format + reasoning_effort.
	req := &canonical.Request{
		Temperature: ptrFloat(0.2),
		Seed:        ptrInt64(42),
		Thinking:    &canonical.ThinkingConfig{Effort: "medium", Type: "enabled"},
		ResponseFormat: &canonical.ResponseFormat{
			Kind:   canonical.ResponseFormatJSONSchema,
			Name:   "answer",
			Schema: json.RawMessage(`{"type":"object","properties":{"a":{"type":"string"}}}`),
		},
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "hi"}},
		}},
	}
	body, err := BuildRequest(req, "gemini-2.0-flash")
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	gc, _ := out["generation_config"].(map[string]any)
	if gc == nil {
		t.Fatalf("%s", body)
	}
	if gc["response_mime_type"] != "application/json" {
		t.Fatalf("mime %v", gc["response_mime_type"])
	}
	if gc["response_schema"] == nil {
		t.Fatal("schema")
	}
	if gc["seed"] != float64(42) {
		t.Fatalf("seed %v", gc["seed"])
	}
	tc, _ := gc["thinking_config"].(map[string]any)
	// medium effort → 8192 budget
	if tc == nil || tc["thinking_budget"] != float64(8192) {
		t.Fatalf("thinking_config %v (want budget 8192 from medium effort)", tc)
	}
}

func TestBuildFidelityKitchenSink(t *testing.T) {
	req := &canonical.Request{
		MaxTokens:   128,
		Temperature: ptrFloat(0.1),
		TopP:        ptrFloat(0.95),
		TopK:        ptrInt(20),
		Seed:        ptrInt64(1),
		Thinking: &canonical.ThinkingConfig{
			Type:            "enabled",
			BudgetTokens:    ptrInt(1024),
			IncludeThoughts: ptrBool(false),
		},
		ResponseFormat: &canonical.ResponseFormat{
			Kind: canonical.ResponseFormatJSONObject,
		},
		Messages: []canonical.Message{{
			Role: canonical.RoleUser,
			Content: []canonical.Block{
				{Type: canonical.BlockText, Text: "go"},
				{Type: canonical.BlockDocument, Document: &canonical.DocumentSource{
					Kind: "base64", MediaType: "application/pdf", Data: "QQ==",
				}},
			},
		}},
	}
	body, err := BuildRequest(req, "m")
	if err != nil {
		t.Fatal(err)
	}
	raw := string(body)
	for _, want := range []string{
		`"top_k":20`,
		`"seed":1`,
		`"response_mime_type":"application/json"`,
		`"thinking_budget":1024`,
		`"include_thoughts":false`,
		`"mime_type":"application/pdf"`,
		`"data":"QQ=="`,
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("missing %s in %s", want, raw)
		}
	}
}
