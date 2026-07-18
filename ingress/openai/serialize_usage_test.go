package openai

import (
	"encoding/json"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestSerializeUsageDetails(t *testing.T) {
	resp := &canonical.Response{
		ID:                "msg_1",
		Model:             "m",
		Content:           []canonical.Block{{Type: canonical.BlockText, Text: "hi"}},
		StopReason:        canonical.StopEndTurn,
		SystemFingerprint: "fp_x",
		ServiceTier:       "auto",
		Usage: canonical.Usage{
			InputTokens:     100,
			OutputTokens:    50,
			HasUsage:        true,
			CacheReadTokens: 80,
			ReasoningTokens: 12,
		},
	}
	body, err := SerializeResponse(resp, 1)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out["system_fingerprint"] != "fp_x" || out["service_tier"] != "auto" {
		t.Fatalf("meta: %s", body)
	}
	u := out["usage"].(map[string]any)
	ptd := u["prompt_tokens_details"].(map[string]any)
	if ptd["cached_tokens"].(float64) != 80 {
		t.Fatalf("cached: %v", ptd)
	}
	ctd := u["completion_tokens_details"].(map[string]any)
	if ctd["reasoning_tokens"].(float64) != 12 {
		t.Fatalf("reasoning: %v", ctd)
	}
}

func TestSerializeOmitsZeroUsageDetails(t *testing.T) {
	resp := &canonical.Response{
		Model:   "m",
		Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		Usage:   canonical.Usage{InputTokens: 1, OutputTokens: 1, HasUsage: true},
	}
	body, _ := SerializeResponse(resp, 0)
	var out map[string]any
	json.Unmarshal(body, &out)
	u := out["usage"].(map[string]any)
	if _, ok := u["prompt_tokens_details"]; ok {
		t.Fatalf("unexpected details: %s", body)
	}
	if _, ok := u["completion_tokens_details"]; ok {
		t.Fatalf("unexpected details: %s", body)
	}
}
