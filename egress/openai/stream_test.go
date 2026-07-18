package openai

import (
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func feed(p *StreamParser, payloads []string) []canonical.StreamEvent {
	var evs []canonical.StreamEvent
	for _, pl := range payloads {
		evs = append(evs, p.Parse([]byte(pl))...)
	}
	return evs
}

func TestStreamParseText(t *testing.T) {
	p := NewStreamParser()
	evs := feed(p, []string{
		`{"id":"c1","model":"gpt-4o","choices":[{"delta":{"role":"assistant"},"finish_reason":null}]}`,
		`{"id":"c1","choices":[{"delta":{"content":"Hel"},"finish_reason":null}]}`,
		`{"id":"c1","choices":[{"delta":{"content":"lo"},"finish_reason":null}]}`,
		`{"id":"c1","choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`{"id":"c1","choices":[],"usage":{"prompt_tokens":4,"completion_tokens":2}}`,
		`[DONE]`,
	})

	if evs[0].Type != canonical.EventStart || evs[0].ID != "c1" || evs[0].Model != "gpt-4o" {
		t.Errorf("start: %+v", evs[0])
	}
	var text string
	var blockStarts, blockStops int
	var fin *canonical.StreamEvent
	for i := range evs {
		switch evs[i].Type {
		case canonical.EventBlockStart:
			blockStarts++
		case canonical.EventTextDelta:
			text += evs[i].Text
		case canonical.EventBlockStop:
			blockStops++
		case canonical.EventFinish:
			fin = &evs[i]
		}
	}
	if text != "Hello" {
		t.Errorf("text = %q", text)
	}
	// The flat OpenAI stream must be given synthetic block boundaries.
	if blockStarts != 1 || blockStops != 1 {
		t.Errorf("block boundaries: %d starts, %d stops", blockStarts, blockStops)
	}
	if fin == nil {
		t.Fatal("no finish event")
	}
	if fin.StopReason != canonical.StopEndTurn {
		t.Errorf("stop = %s", fin.StopReason)
	}
	if !fin.Usage.HasUsage || fin.Usage.InputTokens != 4 || fin.Usage.OutputTokens != 2 {
		t.Errorf("usage = %+v", fin.Usage)
	}
}

func TestStreamParseToolCalls(t *testing.T) {
	p := NewStreamParser()
	evs := feed(p, []string{
		`{"id":"c","model":"m","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"search","arguments":""}}]}}]}`,
		`{"id":"c","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"q\":"}}]}}]}`,
		`{"id":"c","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"go\"}"}}]}}]}`,
		`{"id":"c","choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`[DONE]`,
	})
	var start *canonical.StreamEvent
	var args string
	var fin *canonical.StreamEvent
	for i := range evs {
		switch evs[i].Type {
		case canonical.EventBlockStart:
			start = &evs[i]
		case canonical.EventJSONDelta:
			args += evs[i].PartialJSON
		case canonical.EventFinish:
			fin = &evs[i]
		}
	}
	if start == nil || start.BlockType != canonical.BlockToolUse || start.ToolID != "call_1" || start.ToolName != "search" {
		t.Errorf("block start: %+v", start)
	}
	if args != `{"q":"go"}` {
		t.Errorf("args = %q", args)
	}
	if fin.StopReason != canonical.StopToolUse {
		t.Errorf("stop = %s", fin.StopReason)
	}
}

func TestStreamParseTwoToolCallsGetDistinctBlocks(t *testing.T) {
	p := NewStreamParser()
	evs := feed(p, []string{
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"a","function":{"name":"f1"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"b","function":{"name":"f2"}}]}}]}`,
		`[DONE]`,
	})
	seen := map[int]string{}
	for _, e := range evs {
		if e.Type == canonical.EventBlockStart {
			seen[e.Index] = e.ToolName
		}
	}
	if len(seen) != 2 || seen[0] != "f1" || seen[1] != "f2" {
		t.Errorf("distinct blocks not created: %v", seen)
	}
}

// TestStreamParseFinishWithoutDone covers upstreams that cut the connection
// without sending the [DONE] sentinel.
func TestStreamParseFinishWithoutDone(t *testing.T) {
	p := NewStreamParser()
	feed(p, []string{
		`{"id":"c","model":"m","choices":[{"delta":{"content":"partial"}}]}`,
	})
	evs := p.Finish()
	var stops, finishes int
	for _, e := range evs {
		switch e.Type {
		case canonical.EventBlockStop:
			stops++
		case canonical.EventFinish:
			finishes++
		}
	}
	if stops != 1 || finishes != 1 {
		t.Errorf("Finish must close the open block and terminate: %d stops, %d finishes", stops, finishes)
	}
	// Calling twice must not double-emit.
	if extra := p.Finish(); extra != nil {
		t.Errorf("second Finish should be a no-op, got %+v", extra)
	}
}

func TestStreamParseUsageOnlyChunkNoChoices(t *testing.T) {
	p := NewStreamParser()
	feed(p, []string{`{"id":"c","choices":[],"usage":{"prompt_tokens":9,"completion_tokens":3}}`})
	evs := p.Finish()
	var fin *canonical.StreamEvent
	for i := range evs {
		if evs[i].Type == canonical.EventFinish {
			fin = &evs[i]
		}
	}
	if fin == nil || fin.Usage.InputTokens != 9 || fin.Usage.OutputTokens != 3 {
		t.Errorf("usage from choice-less chunk lost: %+v", fin)
	}
}
