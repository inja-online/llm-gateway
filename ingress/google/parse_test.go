package google

import (
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestParseRequestBasic(t *testing.T) {
	body := []byte(`{
		"contents": [{"role":"user","parts":[{"text":"hi"}]}],
		"generation_config": {"max_output_tokens": 100, "temperature": 0.5}
	}`)
	req, err := ParseRequest(body, "gemini-2.0-flash")
	if err != nil {
		t.Fatal(err)
	}
	if req.Model != "gemini-2.0-flash" {
		t.Fatalf("model = %q", req.Model)
	}
	if len(req.Messages) != 1 || req.Messages[0].Content[0].Text != "hi" {
		t.Fatalf("%+v", req.Messages)
	}
	if req.MaxTokens != 100 || req.Temperature == nil || *req.Temperature != 0.5 {
		t.Fatalf("gen config: max=%d temp=%v", req.MaxTokens, req.Temperature)
	}
}

func TestParseRequestSystemAndTools(t *testing.T) {
	body := []byte(`{
		"model": "google/gemini-2.0-flash",
		"system_instruction": {"parts":[{"text":"be brief"}]},
		"contents": [{"role":"user","parts":[{"text":"weather?"}]}],
		"tools": [{"function_declarations":[{"name":"get_weather","parameters":{"type":"object"}}]}],
		"tool_config": {"function_calling_config": {"mode":"AUTO"}}
	}`)
	req, err := ParseRequest(body, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(req.System) != 1 || req.System[0].Text != "be brief" {
		t.Fatalf("system: %+v", req.System)
	}
	if len(req.Tools) != 1 || req.Tools[0].Name != "get_weather" {
		t.Fatalf("tools: %+v", req.Tools)
	}
	if req.ToolChoice == nil || req.ToolChoice.Mode != canonical.ToolAuto {
		t.Fatalf("tool choice: %+v", req.ToolChoice)
	}
}

func TestSerializeResponse(t *testing.T) {
	out, err := SerializeResponse(&canonical.Response{
		ID:         "r1",
		Model:      "gemini-2.0-flash",
		Content:    []canonical.Block{{Type: canonical.BlockText, Text: "hello"}},
		StopReason: canonical.StopEndTurn,
		Usage:      canonical.Usage{InputTokens: 3, OutputTokens: 1, HasUsage: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) < 20 {
		t.Fatalf("%s", out)
	}
}
