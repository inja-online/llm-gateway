package anthropic

import (
	"encoding/json"

	"github.com/inja-online/llm-gateway/canonical"
)

// StreamSerializer turns canonical stream events into Anthropic SSE named
// events (event: + data: line pairs), the format Claude Code and the Anthropic
// SDK expect.
//
// Limitation: Anthropic carries input_tokens in message_start, but when the
// upstream is OpenAI-wire, token usage only arrives at the end of the stream.
// In that case message_start reports input_tokens: 0 and the final message_delta
// reports the real output_tokens. Same-dialect (Anthropic upstream) traffic
// uses passthrough and is unaffected.
type StreamSerializer struct {
	id         string
	model      string
	startInput int // input tokens to stamp at message_start, if known
}

func NewStreamSerializer() *StreamSerializer { return &StreamSerializer{} }

// Event returns the SSE bytes for one canonical event, or nil if none.
func (s *StreamSerializer) Event(ev canonical.StreamEvent) []byte {
	switch ev.Type {
	case canonical.EventStart:
		s.id = messageID(ev.ID)
		s.model = ev.Model
		return sseEvent("message_start", map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id":            s.id,
				"type":          "message",
				"role":          canonical.RoleAssistant,
				"model":         s.model,
				"content":       []any{},
				"stop_reason":   nil,
				"stop_sequence": nil,
				"usage":         map[string]any{"input_tokens": s.startInput, "output_tokens": 0},
			},
		})

	case canonical.EventBlockStart:
		cb := map[string]any{}
		switch ev.BlockType {
		case canonical.BlockToolUse:
			cb["type"] = "tool_use"
			cb["id"] = ev.ToolID
			cb["name"] = ev.ToolName
			cb["input"] = map[string]any{}
		case canonical.BlockThinking:
			cb["type"] = "thinking"
			cb["thinking"] = ""
		default:
			cb["type"] = "text"
			cb["text"] = ""
		}
		return sseEvent("content_block_start", map[string]any{
			"type":          "content_block_start",
			"index":         ev.Index,
			"content_block": cb,
		})

	case canonical.EventTextDelta:
		return sseEvent("content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": ev.Index,
			"delta": map[string]any{"type": "text_delta", "text": ev.Text},
		})

	case canonical.EventThinkingDelta:
		return sseEvent("content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": ev.Index,
			"delta": map[string]any{"type": "thinking_delta", "thinking": ev.Text},
		})

	case canonical.EventJSONDelta:
		return sseEvent("content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": ev.Index,
			"delta": map[string]any{"type": "input_json_delta", "partial_json": ev.PartialJSON},
		})

	case canonical.EventBlockStop:
		return sseEvent("content_block_stop", map[string]any{
			"type":  "content_block_stop",
			"index": ev.Index,
		})

	case canonical.EventFinish:
		delta := sseEvent("message_delta", map[string]any{
			"type":  "message_delta",
			"delta": map[string]any{"stop_reason": stopReason(ev.StopReason), "stop_sequence": nil},
			"usage": map[string]any{"output_tokens": ev.Usage.OutputTokens},
		})
		stop := sseEvent("message_stop", map[string]any{"type": "message_stop"})
		return append(delta, stop...)
	}
	return nil
}

// sseEvent renders a named SSE event: "event: NAME\ndata: JSON\n\n".
func sseEvent(name string, payload any) []byte {
	body, _ := json.Marshal(payload)
	out := make([]byte, 0, len(name)+len(body)+16)
	out = append(out, "event: "...)
	out = append(out, name...)
	out = append(out, '\n')
	out = append(out, "data: "...)
	out = append(out, body...)
	out = append(out, '\n', '\n')
	return out
}

func messageID(id string) string {
	if id == "" {
		return "msg_gateway"
	}
	return id
}
