package openai

import (
	"bytes"
	"encoding/json"

	"github.com/mamad/llm-gateway/canonical"
)

// StreamParser converts OpenAI SSE chunk payloads into canonical stream
// events. OpenAI's stream is flat (deltas on one choice) while canonical is
// block-structured, so the parser synthesizes block boundaries: it opens a
// text block on the first content delta and one tool_use block per distinct
// tool_calls ordinal, closing them all at the end.
type StreamParser struct {
	started   bool
	nextIndex int

	textIndex    int
	textOpen     bool
	toolIndexes  map[int]int // OpenAI tool ordinal -> canonical block index
	openBlocks   []int       // canonical indexes still open, in open order
	stopReason   string
	usage        canonical.Usage
	finishedSeen bool
}

func NewStreamParser() *StreamParser {
	return &StreamParser{toolIndexes: map[int]int{}, textIndex: -1}
}

// Parse consumes one SSE data payload and returns canonical events. The
// "[DONE]" sentinel yields the terminal EventFinish.
func (p *StreamParser) Parse(data []byte) []canonical.StreamEvent {
	if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		return p.finish()
	}
	var chunk chatResponse
	if json.Unmarshal(data, &chunk) != nil {
		return nil
	}

	var evs []canonical.StreamEvent
	if !p.started {
		p.started = true
		evs = append(evs, canonical.StreamEvent{
			Type:  canonical.EventStart,
			ID:    chunk.ID,
			Model: chunk.Model,
		})
	}
	// A usage-only chunk carries no choices.
	if chunk.Usage != nil {
		p.usage = canonical.Usage{
			InputTokens:  chunk.Usage.PromptTokens,
			OutputTokens: chunk.Usage.CompletionTokens,
			HasUsage:     true,
		}
	}
	if len(chunk.Choices) == 0 {
		return evs
	}
	ch := chunk.Choices[0]
	if ch.Delta != nil {
		if ch.Delta.Content != nil && *ch.Delta.Content != "" {
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
				Text:  *ch.Delta.Content,
			})
		}
		for _, tc := range ch.Delta.ToolCalls {
			idx, known := p.toolIndexes[tc.Index]
			if !known {
				idx = p.nextIndex
				p.nextIndex++
				p.toolIndexes[tc.Index] = idx
				p.openBlocks = append(p.openBlocks, idx)
				evs = append(evs, canonical.StreamEvent{
					Type:      canonical.EventBlockStart,
					Index:     idx,
					BlockType: canonical.BlockToolUse,
					ToolID:    tc.ID,
					ToolName:  tc.Function.Name,
				})
			}
			if tc.Function.Arguments != "" {
				evs = append(evs, canonical.StreamEvent{
					Type:        canonical.EventJSONDelta,
					Index:       idx,
					PartialJSON: tc.Function.Arguments,
				})
			}
		}
	}
	if ch.FinishReason != nil && *ch.FinishReason != "" {
		p.stopReason = finishToStop(*ch.FinishReason)
	}
	return evs
}

// finish closes open blocks and emits the terminal event. Safe to call more
// than once; only the first call produces events.
func (p *StreamParser) finish() []canonical.StreamEvent {
	if p.finishedSeen {
		return nil
	}
	p.finishedSeen = true
	var evs []canonical.StreamEvent
	if !p.started {
		// Stream ended without any chunk; still emit a well-formed sequence.
		evs = append(evs, canonical.StreamEvent{Type: canonical.EventStart})
	}
	for i := len(p.openBlocks) - 1; i >= 0; i-- {
		evs = append(evs, canonical.StreamEvent{Type: canonical.EventBlockStop, Index: p.openBlocks[i]})
	}
	stop := p.stopReason
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

// Finish flushes terminal events when the upstream ends without a [DONE]
// sentinel (a cut connection, or a provider that omits it).
func (p *StreamParser) Finish() []canonical.StreamEvent { return p.finish() }
