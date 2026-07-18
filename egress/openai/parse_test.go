package openai

import (
	"testing"

	"github.com/mamad/llm-gateway/canonical"
)

func TestParseResponseText(t *testing.T) {
	resp, err := ParseResponse([]byte(`{
		"id":"cmpl-1","model":"gpt-4o",
		"choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":7,"completion_tokens":2}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != "cmpl-1" || resp.Model != "gpt-4o" || resp.StopReason != canonical.StopEndTurn {
		t.Errorf("envelope: %+v", resp)
	}
	if resp.Content[0].Text != "hello" {
		t.Errorf("content: %+v", resp.Content)
	}
	if !resp.Usage.HasUsage || resp.Usage.InputTokens != 7 || resp.Usage.OutputTokens != 2 {
		t.Errorf("usage: %+v", resp.Usage)
	}
}

func TestParseResponseToolCalls(t *testing.T) {
	resp, _ := ParseResponse([]byte(`{
		"id":"c","model":"m",
		"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[
			{"id":"call_1","type":"function","function":{"name":"f","arguments":"{\"a\":1}"}}
		]},"finish_reason":"tool_calls"}]
	}`))
	if resp.StopReason != canonical.StopToolUse {
		t.Errorf("stop: %s", resp.StopReason)
	}
	b := resp.Content[0]
	if b.Type != canonical.BlockToolUse || b.ID != "call_1" || b.Name != "f" || string(b.Input) != `{"a":1}` {
		t.Errorf("tool_use: %+v", b)
	}
}

func TestFinishReasonMapping(t *testing.T) {
	cases := map[string]string{
		"stop":           canonical.StopEndTurn,
		"length":         canonical.StopMaxTokens,
		"tool_calls":     canonical.StopToolUse,
		"content_filter": canonical.StopRefusal,
		"":               canonical.StopEndTurn,
	}
	for fr, want := range cases {
		if got := finishToStop(fr); got != want {
			t.Errorf("%q -> %s, want %s", fr, got, want)
		}
	}
}

func TestParseResponseNoUsage(t *testing.T) {
	resp, _ := ParseResponse([]byte(`{"id":"c","model":"m","choices":[{"message":{"content":"x"},"finish_reason":"stop"}]}`))
	if resp.Usage.HasUsage {
		t.Error("usage should be absent")
	}
}
