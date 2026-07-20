package openai

import (
	"encoding/json"
	"testing"

	openaiegress "github.com/inja-online/llm-gateway/egress/openai"
)

func TestPromptCacheKeyRoundTrip(t *testing.T) {
	raw := []byte(`{
		"model":"gpt-test",
		"prompt_cache_key":"tenant-abc",
		"prompt_cache_retention":"24h",
		"messages":[{"role":"user","content":"hi"}]
	}`)
	req, err := ParseRequest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if req.PromptCacheKey != "tenant-abc" || req.PromptCacheRetention != "24h" {
		t.Fatalf("IR: key=%q retention=%q", req.PromptCacheKey, req.PromptCacheRetention)
	}
	body, err := openaiegress.BuildRequest(req, "gpt-test")
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out["prompt_cache_key"] != "tenant-abc" {
		t.Fatalf("%s", body)
	}
	if out["prompt_cache_retention"] != "24h" {
		t.Fatalf("%s", body)
	}
}
