package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func ptrInt(n int) *int       { return &n }
func ptrI64(n int64) *int64   { return &n }
func ptrF64(n float64) *float64 { return &n }
func ptrBool(b bool) *bool    { return &b }

// --- #28 output_config ---

func TestBuildOutputConfigJSONSchema(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"n":{"type":"integer"}}}`)
	body, err := BuildRequest(&canonical.Request{
		MaxTokens: 10,
		ResponseFormat: &canonical.ResponseFormat{
			Kind:   canonical.ResponseFormatJSONSchema,
			Name:   "num",
			Schema: schema,
		},
		Messages: []canonical.Message{{
			Role: canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		}},
	}, "claude")
	if err != nil {
		t.Fatal(err)
	}
	var out messagesRequest
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out.OutputConfig == nil || out.OutputConfig.Format == nil {
		t.Fatalf("missing output_config: %s", body)
	}
	if out.OutputConfig.Format.Type != "json_schema" {
		t.Fatalf("%+v", out.OutputConfig.Format)
	}
	if string(out.OutputConfig.Format.Schema) != string(schema) {
		t.Fatalf("schema %s", out.OutputConfig.Format.Schema)
	}
	if out.OutputConfig.Format.Name != "num" {
		t.Fatalf("name %q", out.OutputConfig.Format.Name)
	}
}

func TestBuildJSONObjectDroppedWithoutSchema(t *testing.T) {
	// Anthropic only supports json_schema; json_object is dropped (documented policy).
	body, err := BuildRequest(&canonical.Request{
		MaxTokens:      10,
		ResponseFormat: &canonical.ResponseFormat{Kind: canonical.ResponseFormatJSONObject},
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		}},
	}, "m")
	if err != nil {
		t.Fatal(err)
	}
	var out messagesRequest
	json.Unmarshal(body, &out)
	if out.OutputConfig != nil && out.OutputConfig.Format != nil {
		t.Fatalf("json_object should not emit format: %+v", out.OutputConfig)
	}
}

// --- #30 thinking ---

func TestBuildThinkingEnabledBudget(t *testing.T) {
	body, err := BuildRequest(&canonical.Request{
		MaxTokens: 10,
		Thinking: &canonical.ThinkingConfig{
			Mode:         canonical.ThinkingEnabled,
			BudgetTokens: ptrInt(5000),
		},
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		}},
	}, "m")
	if err != nil {
		t.Fatal(err)
	}
	var out messagesRequest
	json.Unmarshal(body, &out)
	if out.Thinking == nil || out.Thinking.Type != "enabled" {
		t.Fatalf("%+v", out.Thinking)
	}
	if out.Thinking.BudgetTokens == nil || *out.Thinking.BudgetTokens != 5000 {
		t.Fatalf("%+v", out.Thinking.BudgetTokens)
	}
}

func TestBuildThinkingAdaptiveDisabledEffort(t *testing.T) {
	// adaptive
	body, _ := BuildRequest(&canonical.Request{
		MaxTokens: 1,
		Thinking:  &canonical.ThinkingConfig{Mode: canonical.ThinkingAdaptive, Effort: "medium"},
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		}},
	}, "m")
	var out messagesRequest
	json.Unmarshal(body, &out)
	if out.Thinking == nil || out.Thinking.Type != "adaptive" {
		t.Fatalf("thinking %+v", out.Thinking)
	}
	if out.OutputConfig == nil || out.OutputConfig.Effort != "medium" {
		t.Fatalf("effort on output_config: %+v", out.OutputConfig)
	}

	// disabled
	body, _ = BuildRequest(&canonical.Request{
		MaxTokens: 1,
		Thinking:  &canonical.ThinkingConfig{Mode: canonical.ThinkingDisabled},
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		}},
	}, "m")
	json.Unmarshal(body, &out)
	if out.Thinking == nil || out.Thinking.Type != "disabled" {
		t.Fatalf("%+v", out.Thinking)
	}

	// nil thinking: omit (no accidental enable)
	body, _ = BuildRequest(&canonical.Request{
		MaxTokens: 1,
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		}},
	}, "m")
	out = messagesRequest{} // reset: omitempty leaves prior Unmarshal values otherwise
	json.Unmarshal(body, &out)
	if out.Thinking != nil {
		t.Fatalf("want omit thinking, got %+v", out.Thinking)
	}
}

func TestBuildThinkingEffortMapsToBudget(t *testing.T) {
	// effort without mode/budget → adaptive (no budget) when effort alone;
	// enabled + effort maps budget.
	body, _ := BuildRequest(&canonical.Request{
		MaxTokens: 1,
		Thinking: &canonical.ThinkingConfig{
			Mode:   canonical.ThinkingEnabled,
			Effort: "high",
		},
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		}},
	}, "m")
	var out messagesRequest
	json.Unmarshal(body, &out)
	if out.Thinking == nil || out.Thinking.Type != "enabled" {
		t.Fatalf("%+v", out.Thinking)
	}
	if out.Thinking.BudgetTokens == nil || *out.Thinking.BudgetTokens != 16384 {
		t.Fatalf("high effort budget: %v", out.Thinking.BudgetTokens)
	}
}

// --- #32 document ---

func TestBuildDocumentBlock(t *testing.T) {
	body, err := BuildRequest(&canonical.Request{
		MaxTokens: 1,
		Messages: []canonical.Message{{
			Role: canonical.RoleUser,
			Content: []canonical.Block{{
				Type: canonical.BlockDocument,
				Document: &canonical.DocumentSource{
					Kind: "base64", MediaType: "application/pdf", Data: "JVBERi0=", Title: "a.pdf",
				},
			}},
		}},
	}, "m")
	if err != nil {
		t.Fatal(err)
	}
	var out messagesRequest
	json.Unmarshal(body, &out)
	if len(out.Messages) != 1 || len(out.Messages[0].Content) != 1 {
		t.Fatalf("%+v", out.Messages)
	}
	b := out.Messages[0].Content[0]
	if b.Type != "document" || b.Source == nil || b.Source.Type != "base64" || b.Source.Data != "JVBERi0=" {
		t.Fatalf("%+v", b)
	}
	if b.Title != "a.pdf" {
		t.Fatalf("title %q", b.Title)
	}
}

// --- #37 top_k ---

func TestBuildTopK(t *testing.T) {
	body, _ := BuildRequest(&canonical.Request{
		MaxTokens: 1,
		TopK:      ptrInt(40),
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		}},
	}, "m")
	var out messagesRequest
	json.Unmarshal(body, &out)
	if out.TopK == nil || *out.TopK != 40 {
		t.Fatalf("%v", out.TopK)
	}
	// unset omits
	body, _ = BuildRequest(&canonical.Request{
		MaxTokens: 1,
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		}},
	}, "m")
	if strings.Contains(string(body), "top_k") {
		t.Fatalf("unset top_k should omit: %s", body)
	}
}

// --- #40 parallel polarity ---

func TestBuildDisableParallelToolUsePolarity(t *testing.T) {
	// parallel=false → disable_parallel_tool_use=true
	body, _ := BuildRequest(&canonical.Request{
		MaxTokens:         1,
		ToolChoice:        &canonical.ToolChoice{Mode: canonical.ToolAuto},
		ParallelToolCalls: ptrBool(false),
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		}},
	}, "m")
	var out messagesRequest
	json.Unmarshal(body, &out)
	var tc struct {
		Type                   string `json:"type"`
		DisableParallelToolUse *bool  `json:"disable_parallel_tool_use"`
	}
	json.Unmarshal(out.ToolChoice, &tc)
	if tc.Type != "auto" || tc.DisableParallelToolUse == nil || !*tc.DisableParallelToolUse {
		t.Fatalf("parallel=false → disable=true, got %+v raw=%s", tc, out.ToolChoice)
	}

	// parallel=true → disable_parallel_tool_use=false
	body, _ = BuildRequest(&canonical.Request{
		MaxTokens:         1,
		ToolChoice:        &canonical.ToolChoice{Mode: canonical.ToolAuto},
		ParallelToolCalls: ptrBool(true),
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		}},
	}, "m")
	json.Unmarshal(body, &out)
	json.Unmarshal(out.ToolChoice, &tc)
	if tc.DisableParallelToolUse == nil || *tc.DisableParallelToolUse {
		t.Fatalf("parallel=true → disable=false, got %+v", tc)
	}

	// unset → omit disable field
	body, _ = BuildRequest(&canonical.Request{
		MaxTokens:  1,
		ToolChoice: &canonical.ToolChoice{Mode: canonical.ToolAuto},
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		}},
	}, "m")
	if strings.Contains(string(body), "disable_parallel_tool_use") {
		t.Fatalf("unset should omit disable field: %s", body)
	}
}

// --- #48 redacted_thinking ---

func TestBuildRedactedThinking(t *testing.T) {
	body, err := BuildRequest(&canonical.Request{
		MaxTokens: 1,
		Messages: []canonical.Message{{
			Role: canonical.RoleAssistant,
			Content: []canonical.Block{
				{Type: canonical.BlockThinking, Text: "step", Signature: "sig"},
				{Type: canonical.BlockThinking, Text: "cipher", Redacted: true},
				{Type: canonical.BlockText, Text: "answer"},
			},
		}},
	}, "m")
	if err != nil {
		t.Fatal(err)
	}
	var out messagesRequest
	json.Unmarshal(body, &out)
	if len(out.Messages) != 1 || len(out.Messages[0].Content) != 3 {
		t.Fatalf("%+v", out.Messages)
	}
	if out.Messages[0].Content[0].Type != "thinking" {
		t.Fatalf("%+v", out.Messages[0].Content[0])
	}
	red := out.Messages[0].Content[1]
	if red.Type != "redacted_thinking" || red.Data != "cipher" {
		t.Fatalf("%+v", red)
	}
}

func TestParseResponseRedactedThinking(t *testing.T) {
	resp, err := ParseResponse([]byte(`{
		"id":"m1","type":"message","role":"assistant","model":"c",
		"content":[
			{"type":"thinking","thinking":"a","signature":"s"},
			{"type":"redacted_thinking","data":"SECRET"},
			{"type":"text","text":"hi"}
		],
		"stop_reason":"end_turn",
		"usage":{"input_tokens":1,"output_tokens":2}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Content) != 3 {
		t.Fatalf("%+v", resp.Content)
	}
	if !resp.Content[1].Redacted || resp.Content[1].Text != "SECRET" {
		t.Fatalf("%+v", resp.Content[1])
	}
}

func TestStreamParserRedactedThinking(t *testing.T) {
	p := NewStreamParser()
	evs := p.Parse([]byte(`{"type":"content_block_start","index":1,"content_block":{"type":"redacted_thinking","data":"XYZ"}}`))
	if len(evs) != 1 {
		t.Fatalf("%+v", evs)
	}
	ev := evs[0]
	if ev.Type != canonical.EventBlockStart || ev.BlockType != canonical.BlockThinking {
		t.Fatalf("%+v", ev)
	}
	if !ev.Redacted || ev.Text != "XYZ" {
		t.Fatalf("%+v", ev)
	}
}

// --- #38/#39 drop penalties and seed without error ---

func TestBuildDropsPenaltiesAndSeed(t *testing.T) {
	body, err := BuildRequest(&canonical.Request{
		MaxTokens:        1,
		Seed:             ptrI64(42),
		FrequencyPenalty: ptrF64(0.5),
		PresencePenalty:  ptrF64(0.2),
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		}},
	}, "m")
	if err != nil {
		t.Fatal("Anthropic egress must omit seed/penalties without error:", err)
	}
	s := string(body)
	for _, banned := range []string{"seed", "frequency_penalty", "presence_penalty"} {
		if strings.Contains(s, banned) {
			t.Fatalf("want drop %s, body=%s", banned, s)
		}
	}
	// still a valid request
	var out messagesRequest
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out.Model != "m" || out.MaxTokens != 1 {
		t.Fatalf("%+v", out)
	}
}

// --- round-trip: Anthropic ingress → egress ---

func TestAnthropicFidelityRoundTripKitchen(t *testing.T) {
	raw := []byte(`{
		"model":"claude-test","max_tokens":256,"top_k":20,
		"thinking":{"type":"enabled","budget_tokens":1024},
		"output_config":{"format":{"type":"json_schema","schema":{"type":"object"}}},
		"tool_choice":{"type":"auto","disable_parallel_tool_use":true},
		"messages":[{"role":"user","content":[
			{"type":"text","text":"hi"},
			{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"QQ=="},"title":"q.pdf"}
		]},
		{"role":"assistant","content":[
			{"type":"thinking","thinking":"t","signature":"s"},
			{"type":"redacted_thinking","data":"R"},
			{"type":"text","text":"ok"}
		]}]
	}`)
	// Note: parse is ingress; build is egress — use ingress parse then egress build.
	// Both packages are separate; this test lives in egress and only builds from canonical.
	// Full parse→build is covered by pairing with ingress tests; here we build a rich canonical.
	req := &canonical.Request{
		MaxTokens:         256,
		TopK:              ptrInt(20),
		Thinking:          &canonical.ThinkingConfig{Mode: canonical.ThinkingEnabled, BudgetTokens: ptrInt(1024)},
		ResponseFormat:    &canonical.ResponseFormat{Kind: canonical.ResponseFormatJSONSchema, Schema: json.RawMessage(`{"type":"object"}`)},
		ToolChoice:        &canonical.ToolChoice{Mode: canonical.ToolAuto},
		ParallelToolCalls: ptrBool(false),
		Messages: []canonical.Message{
			{Role: canonical.RoleUser, Content: []canonical.Block{
				{Type: canonical.BlockText, Text: "hi"},
				{Type: canonical.BlockDocument, Document: &canonical.DocumentSource{
					Kind: "base64", MediaType: "application/pdf", Data: "QQ==", Title: "q.pdf",
				}},
			}},
			{Role: canonical.RoleAssistant, Content: []canonical.Block{
				{Type: canonical.BlockThinking, Text: "t", Signature: "s"},
				{Type: canonical.BlockThinking, Text: "R", Redacted: true},
				{Type: canonical.BlockText, Text: "ok"},
			}},
		},
	}
	_ = raw
	body, err := BuildRequest(req, "claude-test")
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{
		`"top_k":20`,
		`"thinking"`,
		`"budget_tokens":1024`,
		`"output_config"`,
		`"json_schema"`,
		`"disable_parallel_tool_use":true`,
		`"document"`,
		`"redacted_thinking"`,
		`"data":"R"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in %s", want, s)
		}
	}
}
