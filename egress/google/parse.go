package google

import (
	"encoding/json"
	"fmt"

	"github.com/inja-online/llm-gateway/canonical"
)

// ParseResponse converts a non-streaming Gemini response into canonical form.
func ParseResponse(body []byte) (*canonical.Response, error) {
	var in generateResponse
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, err
	}
	resp := &canonical.Response{
		ID:         in.id(),
		Model:      in.model(),
		StopReason: canonical.StopEndTurn,
	}
	if len(in.Candidates) > 0 {
		ch := in.Candidates[0]
		if ch.Content != nil {
			for i, p := range ch.Content.Parts {
				switch {
				case p.Thought && p.Text != "":
					resp.Content = append(resp.Content, canonical.Block{
						Type:      canonical.BlockThinking,
						Text:      p.Text,
						Signature: p.ThoughtSignature,
					})
				case p.Text != "":
					resp.Content = append(resp.Content, canonical.Block{Type: canonical.BlockText, Text: p.Text})
				case p.FunctionCall != nil:
					args := p.FunctionCall.Args
					if len(args) == 0 {
						args = json.RawMessage(`{}`)
					}
					resp.Content = append(resp.Content, canonical.Block{
						Type:  canonical.BlockToolUse,
						ID:    fmt.Sprintf("call_%s_%d", p.FunctionCall.Name, i),
						Name:  p.FunctionCall.Name,
						Input: args,
					})
				}
			}
		}
		if fr := ch.finish(); fr != "" {
			resp.StopReason = finishToStop(fr, resp.Content)
		}
	}
	if u := in.usage(); u != nil {
		resp.Usage = canonical.Usage{
			InputTokens:     u.prompt(),
			OutputTokens:    u.candidates(),
			HasUsage:        true,
			CacheReadTokens: u.cached(),
			ReasoningTokens: u.thoughts(),
		}
	}
	return resp, nil
}

func finishToStop(fr string, content []canonical.Block) string {
	// If the model returned function calls, treat as tool_use regardless of STOP.
	for _, b := range content {
		if b.Type == canonical.BlockToolUse {
			return canonical.StopToolUse
		}
	}
	// Universal finishReason catalog (#156) — map Google → canonical stop reasons.
	switch fr {
	case "STOP", "stop", "FINISH_REASON_STOP", "STOP_SEQUENCE":
		return canonical.StopEndTurn
	case "MAX_TOKENS", "max_tokens", "FINISH_REASON_MAX_TOKENS", "LENGTH":
		return canonical.StopMaxTokens
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII",
		"FINISH_REASON_SAFETY", "FINISH_REASON_RECITATION", "CONTENT_FILTER":
		return canonical.StopRefusal
	case "MALFORMED_FUNCTION_CALL", "FINISH_REASON_MALFORMED_FUNCTION_CALL":
		return canonical.StopToolUse
	case "OTHER", "FINISH_REASON_OTHER", "FINISH_REASON_UNSPECIFIED", "":
		return canonical.StopEndTurn
	default:
		return canonical.StopEndTurn
	}
}
