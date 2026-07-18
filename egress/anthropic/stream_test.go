package anthropic

import (
	"testing"

	"github.com/mamad/llm-gateway/canonical"
)

// feed runs raw Anthropic SSE data payloads through the parser and returns all
// canonical events.
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
		`{"type":"message_start","message":{"id":"msg_1","model":"claude-x","usage":{"input_tokens":8,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hel"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"lo"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":4}}`,
		`{"type":"message_stop"}`,
	})

	if evs[0].Type != canonical.EventStart || evs[0].ID != "msg_1" || evs[0].Model != "claude-x" {
		t.Errorf("start: %+v", evs[0])
	}
	var text string
	var fin *canonical.StreamEvent
	for i := range evs {
		switch evs[i].Type {
		case canonical.EventTextDelta:
			text += evs[i].Text
		case canonical.EventFinish:
			fin = &evs[i]
		}
	}
	if text != "Hello" {
		t.Errorf("text = %q", text)
	}
	if fin == nil {
		t.Fatal("no finish event")
	}
	if fin.StopReason != canonical.StopEndTurn {
		t.Errorf("stop = %s", fin.StopReason)
	}
	if !fin.Usage.HasUsage || fin.Usage.InputTokens != 8 || fin.Usage.OutputTokens != 4 {
		t.Errorf("usage = %+v", fin.Usage)
	}
}

func TestStreamParseToolUse(t *testing.T) {
	p := NewStreamParser()
	evs := feed(p, []string{
		`{"type":"message_start","message":{"id":"m","model":"x","usage":{"input_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tu1","name":"search"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"q\":"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"go\"}"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":5}}`,
		`{"type":"message_stop"}`,
	})
	var start *canonical.StreamEvent
	var json string
	var fin *canonical.StreamEvent
	for i := range evs {
		switch evs[i].Type {
		case canonical.EventBlockStart:
			start = &evs[i]
		case canonical.EventJSONDelta:
			json += evs[i].PartialJSON
		case canonical.EventFinish:
			fin = &evs[i]
		}
	}
	if start == nil || start.BlockType != canonical.BlockToolUse || start.ToolID != "tu1" || start.ToolName != "search" {
		t.Errorf("block start: %+v", start)
	}
	if json != `{"q":"go"}` {
		t.Errorf("reassembled json = %q", json)
	}
	if fin.StopReason != canonical.StopToolUse {
		t.Errorf("stop = %s", fin.StopReason)
	}
}

func TestStreamParseIgnoresPing(t *testing.T) {
	p := NewStreamParser()
	evs := p.Parse([]byte(`{"type":"ping"}`))
	if len(evs) != 0 {
		t.Errorf("ping should produce no events: %+v", evs)
	}
}

func TestStreamParseThinkingDelta(t *testing.T) {
	p := NewStreamParser()
	evs := feed(p, []string{
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"reasoning"}}`,
	})
	var got string
	for _, e := range evs {
		if e.Type == canonical.EventThinkingDelta {
			got += e.Text
		}
	}
	if got != "reasoning" {
		t.Errorf("thinking delta = %q", got)
	}
}
