package google

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestBuildAndParseRoundTrip(t *testing.T) {
	temp := 0.2
	req := &canonical.Request{
		Model:       "gemini-2.0-flash",
		System:      []canonical.Block{{Type: canonical.BlockText, Text: "sys"}},
		Messages:    []canonical.Message{{Role: canonical.RoleUser, Content: []canonical.Block{{Type: canonical.BlockText, Text: "hi"}}}},
		MaxTokens:   50,
		Temperature: &temp,
		Tools:       []canonical.Tool{{Name: "fn", Schema: json.RawMessage(`{"type":"object"}`)}},
		ToolChoice:  &canonical.ToolChoice{Mode: canonical.ToolAuto},
	}
	body, err := BuildRequest(req, "gemini-2.0-flash")
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if _, ok := wire["model"]; ok {
		t.Fatal("native google body must not include model")
	}
	if wire["system_instruction"] == nil {
		t.Fatal("missing system_instruction")
	}

	// Simulate upstream response
	respBody := []byte(`{
		"candidates":[{"content":{"role":"model","parts":[{"text":"yo"}]},"finishReason":"STOP"}],
		"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2}
	}`)
	canon, err := ParseResponse(respBody)
	if err != nil {
		t.Fatal(err)
	}
	if len(canon.Content) != 1 || canon.Content[0].Text != "yo" {
		t.Fatalf("%+v", canon.Content)
	}
	if !canon.Usage.HasUsage || canon.Usage.InputTokens != 5 || canon.Usage.OutputTokens != 2 {
		t.Fatalf("%+v", canon.Usage)
	}
}

func TestPath(t *testing.T) {
	if Path("gemini-2.0-flash", false) != "/models/gemini-2.0-flash:generateContent" {
		t.Fatal(Path("gemini-2.0-flash", false))
	}
	if Path("gemini-2.0-flash", true) != "/models/gemini-2.0-flash:streamGenerateContent?alt=sse" {
		t.Fatal(Path("gemini-2.0-flash", true))
	}
	if CountTokensPath("gemini-2.0-flash") != "/models/gemini-2.0-flash:countTokens" {
		t.Fatal(CountTokensPath("gemini-2.0-flash"))
	}
	if ModelsPath() != "/models" {
		t.Fatal(ModelsPath())
	}
	if ModelPath("gemini-2.0-flash") != "/models/gemini-2.0-flash" {
		t.Fatal(ModelPath("gemini-2.0-flash"))
	}
}

func TestStreamParser(t *testing.T) {
	p := NewStreamParser()
	evs := p.Parse([]byte(`{"candidates":[{"content":{"parts":[{"text":"Hel"}]}}],"usageMetadata":{"promptTokenCount":1}}`))
	if len(evs) < 2 {
		t.Fatalf("%+v", evs)
	}
	evs = append(evs, p.Parse([]byte(`{"candidates":[{"content":{"parts":[{"text":"lo"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":2}}`))...)
	evs = append(evs, p.Finish()...)
	var sawFinish bool
	for _, e := range evs {
		if e.Type == canonical.EventFinish {
			sawFinish = true
			if e.Usage.OutputTokens != 2 {
				t.Fatalf("%+v", e.Usage)
			}
		}
	}
	if !sawFinish {
		t.Fatal("no finish")
	}
}

func TestBuildRequestSafetySettings(t *testing.T) {
	req := &canonical.Request{
		Messages: []canonical.Message{{Role: canonical.RoleUser, Content: []canonical.Block{{Type: canonical.BlockText, Text: "hi"}}}},
		SafetySettings: json.RawMessage(`[{"category":"HARM_CATEGORY_HATE_SPEECH","threshold":"BLOCK_NONE"}]`),
	}
	body, err := BuildRequest(req, "m")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "HARM_CATEGORY") {
		t.Fatalf("%s", body)
	}
}
