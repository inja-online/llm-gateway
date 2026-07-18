package anthropic

import (
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestStreamParserFullAndFinalStop(t *testing.T) {
	p := NewStreamParser()
	var all []canonical.StreamEvent
	chunks := []string{
		`{"type":"message_start","message":{"id":"m1","model":"c","usage":{"input_tokens":4,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"t1","name":"f"}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{}"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"content_block_start","index":2,"content_block":{"type":"thinking"}}`,
		`{"type":"content_block_delta","index":2,"delta":{"type":"thinking_delta","thinking":"r"}}`,
		`{"type":"content_block_stop","index":2}`,
		`{"type":"message_delta","delta":{"stop_reason":"max_tokens"},"usage":{"output_tokens":9}}`,
		`{"type":"message_stop"}`,
		`{"type":"ping"}`,
		`not-json`,
	}
	for _, c := range chunks {
		all = append(all, p.Parse([]byte(c))...)
	}
	var sawFinish bool
	for _, ev := range all {
		if ev.Type == canonical.EventFinish {
			sawFinish = true
			if ev.StopReason != canonical.StopMaxTokens {
				t.Fatalf("stop %s", ev.StopReason)
			}
			if ev.Usage.OutputTokens != 9 {
				t.Fatalf("out %d", ev.Usage.OutputTokens)
			}
		}
	}
	if !sawFinish {
		t.Fatalf("events %+v", all)
	}
}

func TestFinalStopEmpty(t *testing.T) {
	p := &StreamParser{}
	if p.finalStop() != canonical.StopEndTurn {
		t.Fatal(p.finalStop())
	}
	p.stopReason = "tool_use"
	if p.finalStop() != "tool_use" {
		t.Fatal()
	}
}

func TestBuildToolChoiceDefaultNil(t *testing.T) {
	if buildToolChoice(&canonical.ToolChoice{Mode: "zzz"}, nil) != nil {
		t.Fatal()
	}
}

func TestBuildNilImage(t *testing.T) {
	_, ok := buildBlock(canonical.Block{Type: canonical.BlockImage})
	if ok {
		t.Fatal()
	}
	_, ok = buildBlock(canonical.Block{Type: "nope"})
	if ok {
		t.Fatal()
	}
}
