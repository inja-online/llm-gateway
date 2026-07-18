package anthropic

import "testing"

func TestParseResponseCacheUsage(t *testing.T) {
	resp, err := ParseResponse([]byte(`{
		"id":"msg_1","type":"message","role":"assistant","model":"claude-x",
		"content":[{"type":"text","text":"ok"}],
		"stop_reason":"end_turn",
		"usage":{
			"input_tokens":100,
			"output_tokens":10,
			"cache_creation_input_tokens":15,
			"cache_read_input_tokens":70
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Usage.HasUsage {
		t.Fatal("want usage")
	}
	if resp.Usage.InputTokens != 100 || resp.Usage.OutputTokens != 10 {
		t.Fatalf("totals: %+v", resp.Usage)
	}
	if resp.Usage.CacheReadTokens != 70 || resp.Usage.CacheWriteTokens != 15 {
		t.Fatalf("cache: %+v", resp.Usage)
	}
}
