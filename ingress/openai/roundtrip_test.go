package openai_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
	oaegress "github.com/inja-online/llm-gateway/egress/openai"
	oaingress "github.com/inja-online/llm-gateway/ingress/openai"
)

// TestOpenAIToCanonicalToOpenAI is the OpenAI self-loop matrix cell (#28).
// Wire → ingress.ParseRequest → egress.BuildRequest → wire fields preserved.
func TestOpenAIToCanonicalToOpenAI(t *testing.T) {
	in := `{
		"model": "gpt-client",
		"max_completion_tokens": 256,
		"temperature": 0.4,
		"frequency_penalty": 0.2,
		"presence_penalty": 0.1,
		"seed": 123,
		"parallel_tool_calls": false,
		"reasoning_effort": "high",
		"response_format": {
			"type": "json_schema",
			"json_schema": {
				"name": "ans",
				"schema": {"type":"object","properties":{"v":{"type":"string"}}},
				"strict": true
			}
		},
		"messages": [
			{"role": "user", "content": [
				{"type": "text", "text": "see"},
				{"type": "image_url", "image_url": {"url": "https://ex.com/a.png", "detail": "high"}},
				{"type": "input_audio", "input_audio": {"data": "QUJD", "format": "wav"}},
				{"type": "file", "file": {"file_id": "file-xyz", "filename": "d.pdf"}}
			]},
			{"role": "assistant", "content": null,
			 "reasoning_content": "step-one",
			 "tool_calls": [{"id":"c1","type":"function","function":{"name":"lookup","arguments":"{\"q\":1}"}}]},
			{"role": "tool", "tool_call_id": "c1", "content": "42"},
			{"role": "assistant", "content": "more",
			 "reasoning_content": "step-two",
			 "tool_calls": [{"id":"c2","type":"function","function":{"name":"lookup","arguments":"{}"}}]},
			{"role": "tool", "tool_call_id": "c2", "content": "43"},
			{"role": "user", "content": "done?"}
		],
		"tools": [{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]
	}`

	req, err := oaingress.ParseRequest([]byte(in))
	if err != nil {
		t.Fatal(err)
	}

	// Canonical assertions
	if req.MaxTokens != 256 || req.MaxTokensField != canonical.MaxTokensFieldMaxCompletionTokens {
		t.Fatalf("max: %d %q", req.MaxTokens, req.MaxTokensField)
	}
	if req.Thinking == nil || req.Thinking.Effort != "high" {
		t.Fatalf("thinking: %+v", req.Thinking)
	}
	if req.ResponseFormat == nil || req.ResponseFormat.Kind != canonical.ResponseFormatJSONSchema {
		t.Fatalf("rf: %+v", req.ResponseFormat)
	}
	if req.FrequencyPenalty == nil || *req.FrequencyPenalty != 0.2 {
		t.Fatalf("freq")
	}
	if req.Seed == nil || *req.Seed != 123 {
		t.Fatalf("seed")
	}
	if req.ParallelToolCalls == nil || *req.ParallelToolCalls {
		t.Fatalf("parallel")
	}

	// Multi-turn tool loop: two assistant turns with thinking
	var thinkingTurns int
	for _, m := range req.Messages {
		if m.Role != canonical.RoleAssistant {
			continue
		}
		for _, b := range m.Content {
			if b.Type == canonical.BlockThinking {
				thinkingTurns++
			}
		}
	}
	if thinkingTurns < 2 {
		t.Fatalf("want ≥2 thinking blocks across tool-loop history, got %d in %+v", thinkingTurns, req.Messages)
	}

	// User multimodal
	user0 := req.Messages[0]
	var hasImg, hasAudio, hasDoc bool
	for _, b := range user0.Content {
		switch b.Type {
		case canonical.BlockImage:
			hasImg = true
			if b.Image.Detail != "high" {
				t.Fatalf("detail: %+v", b.Image)
			}
		case canonical.BlockAudio:
			hasAudio = true
		case canonical.BlockDocument:
			hasDoc = true
		}
	}
	if !hasImg || !hasAudio || !hasDoc {
		t.Fatalf("multimodal: img=%v audio=%v doc=%v content=%+v", hasImg, hasAudio, hasDoc, user0.Content)
	}

	// Egress rebuild
	outBody, err := oaegress.BuildRequest(req, "upstream-model")
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(outBody, &out); err != nil {
		t.Fatal(err)
	}
	if out["model"] != "upstream-model" {
		t.Fatalf("model rewrite: %v", out["model"])
	}
	if _, has := out["max_tokens"]; has {
		t.Fatalf("must emit max_completion_tokens only: %s", outBody)
	}
	if out["max_completion_tokens"] != float64(256) {
		t.Fatalf("max_completion_tokens: %v", out["max_completion_tokens"])
	}
	if out["reasoning_effort"] != "high" {
		t.Fatalf("effort: %v", out["reasoning_effort"])
	}
	if out["frequency_penalty"] != 0.2 || out["presence_penalty"] != 0.1 {
		t.Fatalf("penalties in %s", outBody)
	}
	if out["seed"] != float64(123) {
		t.Fatalf("seed: %v", out["seed"])
	}
	if out["parallel_tool_calls"] != false {
		t.Fatalf("parallel: %v", out["parallel_tool_calls"])
	}
	rf, _ := out["response_format"].(map[string]any)
	if rf == nil || rf["type"] != "json_schema" {
		t.Fatalf("response_format: %v", out["response_format"])
	}
	js, _ := rf["json_schema"].(map[string]any)
	if js == nil || js["name"] != "ans" {
		t.Fatalf("json_schema: %v", rf["json_schema"])
	}

	// reasoning_content preserved on both assistant tool-loop turns
	msgs, _ := out["messages"].([]any)
	var reasoningCount int
	for _, m := range msgs {
		mm, _ := m.(map[string]any)
		if mm["role"] != "assistant" {
			continue
		}
		if rc, ok := mm["reasoning_content"]; ok && rc != nil {
			reasoningCount++
			s, _ := rc.(string)
			if s != "step-one" && s != "step-two" {
				// JSON may decode as string
				if !strings.Contains(string(mustJSON(rc)), "step-") {
					t.Fatalf("unexpected reasoning: %v", rc)
				}
			}
		}
	}
	if reasoningCount < 2 {
		t.Fatalf("want ≥2 assistant reasoning_content fields in tool loop, got %d: %s", reasoningCount, outBody)
	}

	// Multimodal parts rebuilt
	if !strings.Contains(string(outBody), `"detail":"high"`) {
		t.Fatalf("image detail lost: %s", outBody)
	}
	if !strings.Contains(string(outBody), `"input_audio"`) || !strings.Contains(string(outBody), `"QUJD"`) {
		t.Fatalf("audio lost: %s", outBody)
	}
	if !strings.Contains(string(outBody), `"file_id":"file-xyz"`) {
		t.Fatalf("file lost: %s", outBody)
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func TestRedactedThinkingOpenAIPolicy(t *testing.T) {
	// Canonical with redacted thinking → OpenAI egress omits it.
	req := &canonical.Request{
		Messages: []canonical.Message{{
			Role: canonical.RoleAssistant,
			Content: []canonical.Block{
				{Type: canonical.BlockThinking, Text: "secret", Redacted: true},
				{Type: canonical.BlockThinking, Text: "ok", Redacted: false},
				{Type: canonical.BlockText, Text: "hi"},
			},
		}},
	}
	body, err := oaegress.BuildRequest(req, "m")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "secret") {
		t.Fatalf("redacted must be omitted: %s", body)
	}
	if !strings.Contains(string(body), "ok") {
		t.Fatalf("non-redacted must remain: %s", body)
	}
}
