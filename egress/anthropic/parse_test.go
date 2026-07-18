package anthropic

import (
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestParseResponseText(t *testing.T) {
	resp, err := ParseResponse([]byte(`{
		"id":"msg_1","type":"message","role":"assistant","model":"claude-x",
		"content":[{"type":"text","text":"hello"}],
		"stop_reason":"end_turn",
		"usage":{"input_tokens":10,"output_tokens":5}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != "msg_1" || resp.Model != "claude-x" || resp.StopReason != canonical.StopEndTurn {
		t.Errorf("envelope: %+v", resp)
	}
	if len(resp.Content) != 1 || resp.Content[0].Text != "hello" {
		t.Errorf("content: %+v", resp.Content)
	}
	if !resp.Usage.HasUsage || resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 {
		t.Errorf("usage: %+v", resp.Usage)
	}
}

func TestParseResponseToolUse(t *testing.T) {
	resp, _ := ParseResponse([]byte(`{
		"id":"m","model":"x",
		"content":[
			{"type":"text","text":"let me check"},
			{"type":"tool_use","id":"tu1","name":"get_weather","input":{"city":"Paris"}}
		],
		"stop_reason":"tool_use",
		"usage":{"input_tokens":1,"output_tokens":1}
	}`))
	if resp.StopReason != canonical.StopToolUse {
		t.Errorf("stop: %s", resp.StopReason)
	}
	if len(resp.Content) != 2 {
		t.Fatalf("want 2 blocks: %+v", resp.Content)
	}
	tu := resp.Content[1]
	if tu.Type != canonical.BlockToolUse || tu.ID != "tu1" || tu.Name != "get_weather" {
		t.Errorf("tool_use: %+v", tu)
	}
	if string(tu.Input) != `{"city":"Paris"}` {
		t.Errorf("input: %s", tu.Input)
	}
}

func TestParseResponseThinking(t *testing.T) {
	resp, _ := ParseResponse([]byte(`{
		"id":"m","model":"x",
		"content":[{"type":"thinking","thinking":"hmm","signature":"sig"}],
		"stop_reason":"end_turn"
	}`))
	b := resp.Content[0]
	if b.Type != canonical.BlockThinking || b.Text != "hmm" || b.Signature != "sig" {
		t.Errorf("thinking: %+v", b)
	}
}

func TestParseResponseNoUsage(t *testing.T) {
	resp, _ := ParseResponse([]byte(`{"id":"m","model":"x","content":[{"type":"text","text":"x"}],"stop_reason":"end_turn"}`))
	if resp.Usage.HasUsage {
		t.Errorf("usage should be absent")
	}
}
