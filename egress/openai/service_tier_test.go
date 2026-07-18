package openai

import (
	"encoding/json"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestBuildServiceTier(t *testing.T) {
	body, err := BuildRequest(&canonical.Request{
		ServiceTier: "auto",
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "hi"}},
		}},
	}, "gpt-x")
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	json.Unmarshal(body, &out)
	if out["service_tier"] != "auto" {
		t.Fatalf("%s", body)
	}
}

func TestBuildOmitsEmptyServiceTier(t *testing.T) {
	body, _ := BuildRequest(&canonical.Request{
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "hi"}},
		}},
	}, "m")
	var out map[string]any
	json.Unmarshal(body, &out)
	if _, ok := out["service_tier"]; ok {
		t.Fatalf("%s", body)
	}
}
