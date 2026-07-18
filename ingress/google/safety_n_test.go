package google

import (
	"encoding/json"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
	googleegress "github.com/inja-online/llm-gateway/egress/google"
)

func TestParseSafetySettings(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"gemini-x",
		"contents":[{"role":"user","parts":[{"text":"hi"}]}],
		"safety_settings":[{"category":"HARM_CATEGORY_HATE_SPEECH","threshold":"BLOCK_NONE"}]
	}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(req.SafetySettings) == 0 {
		t.Fatal("want safety settings")
	}
	// Egress re-emits.
	body, err := googleegress.BuildRequest(req, "gemini-x")
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	json.Unmarshal(body, &out)
	if out["safety_settings"] == nil {
		t.Fatalf("egress missing safety_settings: %s", body)
	}
}

func TestParseSafetySettingsCamel(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"g",
		"contents":[{"parts":[{"text":"x"}]}],
		"safetySettings":[{"category":"HARM_CATEGORY_DANGEROUS_CONTENT","threshold":"BLOCK_MEDIUM_AND_ABOVE"}]
	}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(req.SafetySettings) == 0 {
		t.Fatal("want camelCase safetySettings")
	}
}

func TestParseCandidateCountPolicy(t *testing.T) {
	_, err := ParseRequest([]byte(`{
		"model":"g",
		"contents":[{"parts":[{"text":"x"}]}],
		"generation_config":{"candidate_count":2}
	}`), "")
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("want ValidationError, got %v", err)
	}
	req, err := ParseRequest([]byte(`{
		"model":"g",
		"contents":[{"parts":[{"text":"x"}]}],
		"generation_config":{"candidate_count":1}
	}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if req.N != 1 {
		t.Fatalf("n: %d", req.N)
	}
}

func TestOpenAIHasNoSafetySettings(t *testing.T) {
	// Canonical without SafetySettings must not invent them on Google egress.
	body, err := googleegress.BuildRequest(&canonical.Request{
		Messages: []canonical.Message{{Role: canonical.RoleUser, Content: []canonical.Block{
			{Type: canonical.BlockText, Text: "hi"},
		}}},
	}, "m")
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	json.Unmarshal(body, &out)
	if _, ok := out["safety_settings"]; ok {
		t.Fatalf("unexpected safety_settings: %s", body)
	}
}
