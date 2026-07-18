package openai

import (
	"encoding/json"

	"github.com/inja-online/llm-gateway/canonical"
)

// ParseResponse converts a non-streaming OpenAI response into canonical form.
func ParseResponse(body []byte) (*canonical.Response, error) {
	var in chatResponse
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, err
	}
	resp := &canonical.Response{ID: in.ID, Model: in.Model, StopReason: canonical.StopEndTurn}
	if len(in.Choices) > 0 {
		ch := in.Choices[0]
		if ch.Message.Content != nil && *ch.Message.Content != "" {
			resp.Content = append(resp.Content, canonical.Block{Type: canonical.BlockText, Text: *ch.Message.Content})
		}
		for _, tc := range ch.Message.ToolCalls {
			args := tc.Function.Arguments
			if args == "" {
				args = "{}"
			}
			resp.Content = append(resp.Content, canonical.Block{
				Type:  canonical.BlockToolUse,
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: json.RawMessage(args),
			})
		}
		if ch.FinishReason != nil {
			resp.StopReason = finishToStop(*ch.FinishReason)
		}
	}
	if in.Usage != nil {
		resp.Usage = canonical.Usage{
			InputTokens:  in.Usage.PromptTokens,
			OutputTokens: in.Usage.CompletionTokens,
			HasUsage:     true,
		}
	}
	return resp, nil
}

// finishToStop maps an OpenAI finish_reason to a canonical stop reason.
func finishToStop(fr string) string {
	switch fr {
	case "stop":
		return canonical.StopEndTurn
	case "length":
		return canonical.StopMaxTokens
	case "tool_calls", "function_call":
		return canonical.StopToolUse
	case "content_filter":
		return canonical.StopRefusal
	default:
		return canonical.StopEndTurn
	}
}
