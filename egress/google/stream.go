package google

import (
	"encoding/json"
	"fmt"

	"github.com/inja-online/llm-gateway/canonical"
)

// StreamParser converts Gemini SSE generateContent chunks into canonical events.
// Thinking (thought parts) and visible text use separate synthetic block indexes
// so EventThinkingDelta never shares an index with EventTextDelta.
type StreamParser struct {
	started    bool
	nextIndex  int
	textIndex  int
	textOpen   bool
	thinkIndex int
	thinkOpen  bool
	toolIndex  map[string]int // name -> block index
	openBlocks []int
	stopReason string
	usage      canonical.Usage
	finished   bool
	hasTool    bool
}

func NewStreamParser() *StreamParser {
	return &StreamParser{toolIndex: map[string]int{}, textIndex: -1, thinkIndex: -1}
}

// Parse consumes one SSE data payload.
func (p *StreamParser) Parse(data []byte) []canonical.StreamEvent {
	var chunk generateResponse
	if json.Unmarshal(data, &chunk) != nil {
		return nil
	}
	var evs []canonical.StreamEvent
	if !p.started {
		p.started = true
		evs = append(evs, canonical.StreamEvent{
			Type:  canonical.EventStart,
			ID:    chunk.id(),
			Model: chunk.model(),
		})
	}
	if u := chunk.usage(); u != nil {
		p.usage = canonical.Usage{
			InputTokens:     u.prompt(),
			OutputTokens:    u.candidates(),
			HasUsage:        true,
			CacheReadTokens: u.cached(),
			ReasoningTokens: u.thoughts(),
		}
	}
	if len(chunk.Candidates) == 0 {
		return evs
	}
	ch := chunk.Candidates[0]
	if ch.Content != nil {
		for i, part := range ch.Content.Parts {
			switch {
			case part.Thought && part.Text != "":
				if !p.thinkOpen {
					p.thinkIndex = p.nextIndex
					p.nextIndex++
					p.thinkOpen = true
					p.openBlocks = append(p.openBlocks, p.thinkIndex)
					evs = append(evs, canonical.StreamEvent{
						Type:      canonical.EventBlockStart,
						Index:     p.thinkIndex,
						BlockType: canonical.BlockThinking,
					})
				}
				evs = append(evs, canonical.StreamEvent{
					Type:  canonical.EventThinkingDelta,
					Index: p.thinkIndex,
					Text:  part.Text,
				})
			case part.Text != "":
				if !p.textOpen {
					p.textIndex = p.nextIndex
					p.nextIndex++
					p.textOpen = true
					p.openBlocks = append(p.openBlocks, p.textIndex)
					evs = append(evs, canonical.StreamEvent{
						Type:      canonical.EventBlockStart,
						Index:     p.textIndex,
						BlockType: canonical.BlockText,
					})
				}
				evs = append(evs, canonical.StreamEvent{
					Type:  canonical.EventTextDelta,
					Index: p.textIndex,
					Text:  part.Text,
				})
			case part.FunctionCall != nil:
				p.hasTool = true
				name := part.FunctionCall.Name
				idx, known := p.toolIndex[name]
				if !known {
					idx = p.nextIndex
					p.nextIndex++
					p.toolIndex[name] = idx
					p.openBlocks = append(p.openBlocks, idx)
					evs = append(evs, canonical.StreamEvent{
						Type:      canonical.EventBlockStart,
						Index:     idx,
						BlockType: canonical.BlockToolUse,
						ToolID:    fmt.Sprintf("call_%s_%d", name, i),
						ToolName:  name,
					})
				}
				if len(part.FunctionCall.Args) > 0 {
					evs = append(evs, canonical.StreamEvent{
						Type:        canonical.EventJSONDelta,
						Index:       idx,
						PartialJSON: string(part.FunctionCall.Args),
					})
				}
			}
		}
	}
	if fr := ch.finish(); fr != "" {
		// Defer mapping until finish so we know about tools.
		p.stopReason = fr
	}
	return evs
}

// Finish closes open blocks and emits EventFinish.
func (p *StreamParser) Finish() []canonical.StreamEvent {
	if p.finished {
		return nil
	}
	p.finished = true
	var evs []canonical.StreamEvent
	if !p.started {
		evs = append(evs, canonical.StreamEvent{Type: canonical.EventStart})
	}
	for i := len(p.openBlocks) - 1; i >= 0; i-- {
		evs = append(evs, canonical.StreamEvent{Type: canonical.EventBlockStop, Index: p.openBlocks[i]})
	}
	stop := finishToStop(p.stopReason, nil)
	if p.hasTool {
		stop = canonical.StopToolUse
	}
	if stop == "" {
		stop = canonical.StopEndTurn
	}
	evs = append(evs, canonical.StreamEvent{
		Type:       canonical.EventFinish,
		StopReason: stop,
		Usage:      p.usage,
	})
	return evs
}
