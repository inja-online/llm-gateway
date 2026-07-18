package openai

import (
	"encoding/json"

	"github.com/inja-online/llm-gateway/canonical"
)

// StreamSerializer turns canonical stream events into OpenAI SSE chunk bytes.
// It is stateful: OpenAI tool_calls carry a per-call ordinal index distinct
// from canonical block indexes, so we track the mapping.
type StreamSerializer struct {
	id       string
	created  int64
	model    string
	roleSent bool

	// blockIndexToToolOrdinal maps a canonical tool_use block index to its
	// OpenAI tool_calls ordinal.
	blockIndexToToolOrdinal map[int]int
	nextToolOrdinal         int
}

func NewStreamSerializer(created int64) *StreamSerializer {
	return &StreamSerializer{
		created:                 created,
		blockIndexToToolOrdinal: map[int]int{},
	}
}

// Event returns the SSE bytes for one canonical event, or nil if the event
// produces no client-visible chunk. done reports whether the stream is over
// (the caller should then write the [DONE] sentinel via Done).
func (s *StreamSerializer) Event(ev canonical.StreamEvent) []byte {
	switch ev.Type {
	case canonical.EventStart:
		s.id = ev.ID
		s.model = ev.Model
		return s.chunk(chatOutMsg{Role: canonical.RoleAssistant}, nil, nil)

	case canonical.EventBlockStart:
		if ev.BlockType == canonical.BlockToolUse {
			ord := s.nextToolOrdinal
			s.nextToolOrdinal++
			s.blockIndexToToolOrdinal[ev.Index] = ord
			return s.chunk(chatOutMsg{ToolCalls: []outToolCall{{
				Index:    ord,
				ID:       ev.ToolID,
				Type:     "function",
				Function: outFunctionDelta{Name: ev.ToolName},
			}}}, nil, nil)
		}
		return nil

	case canonical.EventTextDelta:
		return s.chunk(chatOutMsg{Content: &ev.Text}, nil, nil)

	case canonical.EventThinkingDelta:
		raw, _ := json.Marshal(ev.Text)
		return s.chunk(chatOutMsg{Reasoning: raw}, nil, nil)

	case canonical.EventJSONDelta:
		ord, ok := s.blockIndexToToolOrdinal[ev.Index]
		if !ok {
			return nil
		}
		return s.chunk(chatOutMsg{ToolCalls: []outToolCall{{
			Index:    ord,
			Function: outFunctionDelta{Arguments: ev.PartialJSON},
		}}}, nil, nil)

	case canonical.EventBlockStop:
		return nil

	case canonical.EventFinish:
		fin := stopReasonToFinish(ev.StopReason)
		var u *usage
		if ev.Usage.HasUsage {
			u = &usage{
				PromptTokens:     ev.Usage.InputTokens,
				CompletionTokens: ev.Usage.OutputTokens,
				TotalTokens:      ev.Usage.InputTokens + ev.Usage.OutputTokens,
			}
		}
		return s.chunk(chatOutMsg{}, &fin, u)
	}
	return nil
}

// Done returns the SSE termination sentinel.
func (s *StreamSerializer) Done() []byte {
	return []byte("data: [DONE]\n\n")
}

// chunk builds one SSE "data:" line for a chat.completion.chunk.
func (s *StreamSerializer) chunk(delta chatOutMsg, finish *string, u *usage) []byte {
	ch := chatChoice{Index: 0, Delta: &delta, FinishReason: finish}
	c := chatResponse{
		ID:      responseID(s.id),
		Object:  "chat.completion.chunk",
		Created: s.created,
		Model:   s.model,
		Choices: []chatChoice{ch},
		Usage:   u,
	}
	// A usage-only final chunk in OpenAI carries an empty choices array.
	if u != nil && finish == nil {
		c.Choices = []chatChoice{}
	}
	body, _ := json.Marshal(c)
	out := make([]byte, 0, len(body)+8)
	out = append(out, "data: "...)
	out = append(out, body...)
	out = append(out, '\n', '\n')
	return out
}
