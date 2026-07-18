package anthropic

import (
	"encoding/json"

	"github.com/inja-online/llm-gateway/canonical"
)

// ParseResponse converts a non-streaming Anthropic Messages response into a
// canonical response.
func ParseResponse(body []byte) (*canonical.Response, error) {
	var in messagesResponse
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, err
	}
	resp := &canonical.Response{
		ID:         in.ID,
		Model:      in.Model,
		StopReason: normalizeStop(in.StopReason),
	}
	for _, b := range in.Content {
		if cb, ok := parseBlock(b); ok {
			resp.Content = append(resp.Content, cb)
		}
	}
	if in.Usage != nil {
		resp.Usage = usageFromWire(in.Usage)
	}
	return resp, nil
}

func usageFromWire(u *anthropicUsage) canonical.Usage {
	return canonical.Usage{
		InputTokens:      u.InputTokens,
		OutputTokens:     u.OutputTokens,
		HasUsage:         true,
		CacheReadTokens:  u.CacheReadInputTokens,
		CacheWriteTokens: u.CacheCreationInputTokens,
	}
}

func normalizeStop(sr string) string {
	switch sr {
	case "end_turn", "max_tokens", "tool_use", "stop_sequence", "refusal":
		return sr
	case "":
		return canonical.StopEndTurn
	default:
		return sr
	}
}

func parseBlock(b block) (canonical.Block, bool) {
	switch b.Type {
	case "text":
		return canonical.Block{Type: canonical.BlockText, Text: b.Text}, true
	case "thinking":
		return canonical.Block{Type: canonical.BlockThinking, Text: b.Thinking, Signature: b.Signature}, true
	case "tool_use":
		return canonical.Block{
			Type:  canonical.BlockToolUse,
			ID:    b.ID,
			Name:  b.Name,
			Input: b.Input,
		}, true
	}
	return canonical.Block{}, false
}
