package openai

import (
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestParseResponseUsageDetails(t *testing.T) {
	resp, err := ParseResponse([]byte(`{
		"id":"c1","model":"gpt-x",
		"system_fingerprint":"fp_abc",
		"service_tier":"default",
		"choices":[{"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],
		"usage":{
			"prompt_tokens":100,
			"completion_tokens":50,
			"prompt_tokens_details":{"cached_tokens":80},
			"completion_tokens_details":{"reasoning_tokens":20}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Usage.HasUsage || resp.Usage.InputTokens != 100 || resp.Usage.OutputTokens != 50 {
		t.Fatalf("totals: %+v", resp.Usage)
	}
	if resp.Usage.CacheReadTokens != 80 || resp.Usage.ReasoningTokens != 20 {
		t.Fatalf("details: %+v", resp.Usage)
	}
	if resp.SystemFingerprint != "fp_abc" || resp.ServiceTier != "default" {
		t.Fatalf("meta: fingerprint=%q tier=%q", resp.SystemFingerprint, resp.ServiceTier)
	}
	// stop reason sanity
	if resp.StopReason != canonical.StopEndTurn {
		t.Fatalf("stop: %s", resp.StopReason)
	}
}
