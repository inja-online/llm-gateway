package openai

import (
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestStreamParserToolAndFinish(t *testing.T) {
	p := NewStreamParser()
	var all []canonical.StreamEvent
	for _, line := range []string{
		`{"id":"c1","model":"m","choices":[{"delta":{"role":"assistant"}}]}`,
		`{"id":"c1","choices":[{"delta":{"content":"hi"}}]}`,
		`{"id":"c1","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"f","arguments":"{\"a\""}}]}}]}`,
		`{"id":"c1","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":1}"}}]}}]}`,
		`{"id":"c1","choices":[{"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":2,"completion_tokens":3}}`,
		`[DONE]`,
	} {
		all = append(all, p.Parse([]byte(line))...)
	}
	all = append(all, p.Finish()...)
	var sawText, sawTool, sawFinish bool
	for _, ev := range all {
		switch ev.Type {
		case canonical.EventTextDelta:
			sawText = true
		case canonical.EventBlockStart:
			if ev.BlockType == canonical.BlockToolUse {
				sawTool = true
			}
		case canonical.EventFinish:
			sawFinish = true
			if ev.Usage.InputTokens != 2 || ev.Usage.OutputTokens != 3 {
				t.Fatalf("usage %+v", ev.Usage)
			}
			if ev.StopReason != canonical.StopToolUse {
				t.Fatalf("stop %s", ev.StopReason)
			}
		}
	}
	if !sawText || !sawTool || !sawFinish {
		t.Fatalf("text=%v tool=%v finish=%v events=%+v", sawText, sawTool, sawFinish, all)
	}
}

func TestFinishToStopFunctionCall(t *testing.T) {
	if finishToStop("function_call") != canonical.StopToolUse {
		t.Fatal()
	}
	if finishToStop("weird") != canonical.StopEndTurn {
		t.Fatal()
	}
}

func TestParseResponseEmptyToolArgs(t *testing.T) {
	resp, err := ParseResponse([]byte(`{
		"id":"c","model":"m",
		"choices":[{"message":{"tool_calls":[
			{"id":"1","type":"function","function":{"name":"f","arguments":""}}
		]},"finish_reason":"tool_calls"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Content[0].Input) != "{}" {
		t.Fatal(string(resp.Content[0].Input))
	}
}

func TestBuildToolChoiceNilMode(t *testing.T) {
	// unknown mode returns nil from buildToolChoice
	raw := buildToolChoice(&canonical.ToolChoice{Mode: "nope"})
	if raw != nil {
		t.Fatalf("%s", raw)
	}
}
