package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func ptrFloat(v float64) *float64 { return &v }
func ptrInt64(v int64) *int64     { return &v }
func ptrBool(v bool) *bool        { return &v }

// TestOpenAIToCanonicalToOpenAIRoundTrip is the #28 matrix cell for OpenAI self-loop:
// wire → ParseRequest (ingress) is not used here; we build from a fully-populated
// canonical Request that mirrors what ingress would produce, then BuildRequest.
func TestBuildFidelityFields(t *testing.T) {
	req := &canonical.Request{
		MaxTokens:         128,
		MaxTokensField:    canonical.MaxTokensFieldMaxCompletionTokens,
		Temperature:       ptrFloat(0.2),
		FrequencyPenalty:  ptrFloat(0.5),
		PresencePenalty:   ptrFloat(0.1),
		Seed:              ptrInt64(7),
		ParallelToolCalls: ptrBool(false),
		Thinking:          &canonical.ThinkingConfig{Effort: "medium", Type: "enabled"},
		ResponseFormat: &canonical.ResponseFormat{
			Kind: canonical.ResponseFormatJSONObject,
		},
		Messages: []canonical.Message{
			{Role: canonical.RoleUser, Content: []canonical.Block{
				{Type: canonical.BlockText, Text: "hi"},
			}},
		},
	}
	body, err := BuildRequest(req, "gpt-x")
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if _, has := out["max_tokens"]; has {
		t.Fatalf("must not emit max_tokens when source is max_completion_tokens: %s", body)
	}
	if out["max_completion_tokens"] != float64(128) {
		t.Fatalf("max_completion_tokens: %v", out["max_completion_tokens"])
	}
	if out["frequency_penalty"] != 0.5 || out["presence_penalty"] != 0.1 {
		t.Fatalf("penalties: %s", body)
	}
	if out["seed"] != float64(7) {
		t.Fatalf("seed: %v", out["seed"])
	}
	if out["parallel_tool_calls"] != false {
		t.Fatalf("parallel: %v", out["parallel_tool_calls"])
	}
	if out["reasoning_effort"] != "medium" {
		t.Fatalf("effort: %v", out["reasoning_effort"])
	}
	rf, _ := out["response_format"].(map[string]any)
	if rf == nil || rf["type"] != "json_object" {
		t.Fatalf("response_format: %v", out["response_format"])
	}
}

func TestBuildMaxTokensDefaultField(t *testing.T) {
	body, _ := BuildRequest(&canonical.Request{
		MaxTokens: 10,
		Messages:  []canonical.Message{{Role: canonical.RoleUser, Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}}}},
	}, "m")
	var out map[string]any
	json.Unmarshal(body, &out)
	if out["max_tokens"] != float64(10) {
		t.Fatalf("%s", body)
	}
	if _, has := out["max_completion_tokens"]; has {
		t.Fatalf("must not emit max_completion_tokens: %s", body)
	}
}

func TestBuildResponseFormatJSONSchema(t *testing.T) {
	strict := true
	body, err := BuildRequest(&canonical.Request{
		ResponseFormat: &canonical.ResponseFormat{
			Kind:        canonical.ResponseFormatJSONSchema,
			Name:        "person",
			Description: "d",
			Schema:      json.RawMessage(`{"type":"object"}`),
			Strict:      &strict,
		},
		Messages: []canonical.Message{{Role: canonical.RoleUser, Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}}}},
	}, "m")
	if err != nil {
		t.Fatal(err)
	}
	var out chatRequest
	json.Unmarshal(body, &out)
	if out.ResponseFormat == nil || out.ResponseFormat.Type != "json_schema" {
		t.Fatalf("%+v", out.ResponseFormat)
	}
	if out.ResponseFormat.JSONSchema == nil || out.ResponseFormat.JSONSchema.Name != "person" {
		t.Fatalf("%+v", out.ResponseFormat.JSONSchema)
	}
	if out.ResponseFormat.JSONSchema.Strict == nil || !*out.ResponseFormat.JSONSchema.Strict {
		t.Fatal("strict")
	}
}

func TestBuildImageDetail(t *testing.T) {
	body, _ := BuildRequest(&canonical.Request{
		Messages: []canonical.Message{{
			Role: canonical.RoleUser,
			Content: []canonical.Block{{
				Type:  canonical.BlockImage,
				Image: &canonical.ImageSource{Kind: "url", Data: "https://x/a.png", Detail: "low"},
			}},
		}},
	}, "m")
	var out chatRequest
	json.Unmarshal(body, &out)
	var parts []contentPart
	json.Unmarshal(out.Messages[0].Content, &parts)
	if len(parts) != 1 || parts[0].ImageURL == nil || parts[0].ImageURL.Detail != "low" {
		t.Fatalf("%+v", parts)
	}
}

func TestBuildInputAudioAndFile(t *testing.T) {
	body, err := BuildRequest(&canonical.Request{
		Messages: []canonical.Message{{
			Role: canonical.RoleUser,
			Content: []canonical.Block{
				{Type: canonical.BlockText, Text: "look"},
				{Type: canonical.BlockAudio, Audio: &canonical.AudioSource{Kind: "base64", Data: "QUJD", MediaType: "audio/wav"}},
				{Type: canonical.BlockDocument, Document: &canonical.DocumentSource{Kind: "file_id", Data: "file-1", Filename: "a.pdf"}},
				{Type: canonical.BlockDocument, Document: &canonical.DocumentSource{
					Kind: "base64", MediaType: "application/pdf", Data: "AA==", Filename: "b.pdf",
				}},
			},
		}},
	}, "m")
	if err != nil {
		t.Fatal(err)
	}
	var out chatRequest
	json.Unmarshal(body, &out)
	var parts []contentPart
	if err := json.Unmarshal(out.Messages[0].Content, &parts); err != nil {
		t.Fatal(err)
	}
	if len(parts) != 4 {
		t.Fatalf("%d parts: %+v", len(parts), parts)
	}
	if parts[1].Type != "input_audio" || parts[1].InputAudio.Data != "QUJD" || parts[1].InputAudio.Format != "wav" {
		t.Fatalf("audio: %+v", parts[1])
	}
	if parts[2].Type != "file" || parts[2].File.FileID != "file-1" {
		t.Fatalf("file_id: %+v", parts[2])
	}
	if parts[3].File == nil || !strings.Contains(parts[3].File.FileData, "base64,AA==") {
		t.Fatalf("file_data: %+v", parts[3].File)
	}
}

func TestBuildOmitsUnsetOptionalFields(t *testing.T) {
	body, _ := BuildRequest(&canonical.Request{
		Messages: []canonical.Message{{Role: canonical.RoleUser, Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}}}},
	}, "m")
	var out map[string]any
	json.Unmarshal(body, &out)
	for _, k := range []string{
		"frequency_penalty", "presence_penalty", "seed", "parallel_tool_calls",
		"reasoning_effort", "response_format", "max_completion_tokens",
	} {
		if _, ok := out[k]; ok {
			t.Errorf("must omit unset %s: %s", k, body)
		}
	}
}
