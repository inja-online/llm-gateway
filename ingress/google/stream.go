package google

import (
	"encoding/json"

	"github.com/inja-online/llm-gateway/canonical"
)

// StreamSerializer turns canonical stream events into Gemini SSE chunks
// (each data: line is a partial GenerateContentResponse).
type StreamSerializer struct {
	id    string
	model string
	// textAccum / tool state for emitting cumulative-looking chunks.
	// Gemini streams are typically incremental text parts; we emit deltas.
}

func NewStreamSerializer() *StreamSerializer { return &StreamSerializer{} }

// Event returns SSE bytes for one canonical event, or nil.
func (s *StreamSerializer) Event(ev canonical.StreamEvent) []byte {
	switch ev.Type {
	case canonical.EventStart:
		s.id = responseID(ev.ID)
		s.model = ev.Model
		return nil

	case canonical.EventTextDelta:
		return s.chunk(part{Text: ev.Text}, "", nil)

	case canonical.EventThinkingDelta:
		return s.chunk(part{Text: ev.Text, Thought: true}, "", nil)

	case canonical.EventBlockStart:
		if ev.BlockType == canonical.BlockToolUse {
			return s.chunk(part{FunctionCall: &functionCall{
				Name: ev.ToolName,
				Args: json.RawMessage(`{}`),
			}}, "", nil)
		}
		return nil

	case canonical.EventJSONDelta:
		// Emit function call args as a complete-looking partial; best-effort.
		return s.chunk(part{FunctionCall: &functionCall{
			Args: json.RawMessage(ev.PartialJSON),
		}}, "", nil)

	case canonical.EventFinish:
		var u *usageMetadata
		if ev.Usage.HasUsage {
			u = &usageMetadata{
				PromptTokenCount:     ev.Usage.InputTokens,
				CandidatesTokenCount: ev.Usage.OutputTokens,
				TotalTokenCount:      ev.Usage.InputTokens + ev.Usage.OutputTokens,
			}
		}
		return s.chunk(part{}, stopToFinish(ev.StopReason), u)
	}
	return nil
}

func (s *StreamSerializer) chunk(p part, finish string, u *usageMetadata) []byte {
	cand := candidate{Index: 0, FinishReason: finish}
	if p.Text != "" || p.Thought || p.FunctionCall != nil {
		cand.Content = &content{Role: "model", Parts: []part{p}}
	}
	resp := generateResponse{
		ResponseID:    s.id,
		ModelVersion:  s.model,
		Candidates:    []candidate{cand},
		UsageMetadata: u,
	}
	body, _ := json.Marshal(resp)
	out := make([]byte, 0, len(body)+8)
	out = append(out, "data: "...)
	out = append(out, body...)
	out = append(out, '\n', '\n')
	return out
}
