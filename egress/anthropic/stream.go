package anthropic

import (
	"encoding/json"

	"github.com/inja-online/llm-gateway/canonical"
)

// StreamParser converts Anthropic SSE data payloads into canonical stream
// events. It is stateful: input tokens arrive in message_start, output tokens
// in message_delta, and the final canonical EventFinish combines them.
type StreamParser struct {
	inputTokens  int
	outputTokens int
	cacheRead    int
	cacheWrite   int
	hasUsage     bool
	stopReason   string
}

func NewStreamParser() *StreamParser { return &StreamParser{} }

// wire event envelope
type streamEnvelope struct {
	Type    string `json:"type"`
	Index   int    `json:"index"`
	Message *struct {
		ID    string          `json:"id"`
		Model string          `json:"model"`
		Usage *anthropicUsage `json:"usage"`
	} `json:"message"`
	ContentBlock *struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
		Data string `json:"data"` // redacted_thinking
	} `json:"content_block"`
	Delta *struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
		Thinking    string `json:"thinking"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	Usage *anthropicUsage `json:"usage"`
}

// Parse consumes one SSE data payload and returns zero or more canonical
// events. Unknown event types (e.g. ping) return nil.
func (p *StreamParser) Parse(data []byte) []canonical.StreamEvent {
	var env streamEnvelope
	if json.Unmarshal(data, &env) != nil {
		return nil
	}
	switch env.Type {
	case "message_start":
		if env.Message == nil {
			return nil
		}
		if env.Message.Usage != nil {
			p.inputTokens = env.Message.Usage.InputTokens
			p.cacheRead = env.Message.Usage.CacheReadInputTokens
			p.cacheWrite = env.Message.Usage.CacheCreationInputTokens
			p.hasUsage = true
		}
		return []canonical.StreamEvent{{
			Type:  canonical.EventStart,
			ID:    env.Message.ID,
			Model: env.Message.Model,
		}}

	case "content_block_start":
		if env.ContentBlock == nil {
			return nil
		}
		ev := canonical.StreamEvent{Type: canonical.EventBlockStart, Index: env.Index}
		switch env.ContentBlock.Type {
		case "tool_use":
			ev.BlockType = canonical.BlockToolUse
			ev.ToolID = env.ContentBlock.ID
			ev.ToolName = env.ContentBlock.Name
		case "thinking":
			ev.BlockType = canonical.BlockThinking
		case "redacted_thinking":
			// Full opaque payload arrives on start; no thinking_delta follows.
			ev.BlockType = canonical.BlockThinking
			ev.Redacted = true
			ev.Text = env.ContentBlock.Data
		default:
			ev.BlockType = canonical.BlockText
		}
		return []canonical.StreamEvent{ev}

	case "content_block_delta":
		if env.Delta == nil {
			return nil
		}
		switch env.Delta.Type {
		case "text_delta":
			return []canonical.StreamEvent{{Type: canonical.EventTextDelta, Index: env.Index, Text: env.Delta.Text}}
		case "input_json_delta":
			return []canonical.StreamEvent{{Type: canonical.EventJSONDelta, Index: env.Index, PartialJSON: env.Delta.PartialJSON}}
		case "thinking_delta":
			return []canonical.StreamEvent{{Type: canonical.EventThinkingDelta, Index: env.Index, Text: env.Delta.Thinking}}
		}
		return nil

	case "content_block_stop":
		return []canonical.StreamEvent{{Type: canonical.EventBlockStop, Index: env.Index}}

	case "message_delta":
		if env.Delta != nil && env.Delta.StopReason != "" {
			p.stopReason = normalizeStop(env.Delta.StopReason)
		}
		if env.Usage != nil {
			p.outputTokens = env.Usage.OutputTokens
			// Cache fields may also appear on message_delta in some API versions.
			if env.Usage.CacheReadInputTokens > 0 {
				p.cacheRead = env.Usage.CacheReadInputTokens
			}
			if env.Usage.CacheCreationInputTokens > 0 {
				p.cacheWrite = env.Usage.CacheCreationInputTokens
			}
			p.hasUsage = true
		}
		return nil

	case "message_stop":
		return []canonical.StreamEvent{{
			Type:       canonical.EventFinish,
			StopReason: p.finalStop(),
			Usage: canonical.Usage{
				InputTokens:      p.inputTokens,
				OutputTokens:     p.outputTokens,
				HasUsage:         p.hasUsage,
				CacheReadTokens:  p.cacheRead,
				CacheWriteTokens: p.cacheWrite,
			},
		}}
	}
	return nil
}

func (p *StreamParser) finalStop() string {
	if p.stopReason == "" {
		return canonical.StopEndTurn
	}
	return p.stopReason
}
