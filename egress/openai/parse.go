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
		// Prefer thinking before text so assemble order matches stream
		// (reasoning_content deltas typically precede content).
		if reason := rawMessageString(ch.Message.Reasoning); reason != "" {
			resp.Content = append(resp.Content, canonical.Block{Type: canonical.BlockThinking, Text: reason})
		}
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
	resp.SystemFingerprint = in.SystemFingerprint
	resp.ServiceTier = in.ServiceTier
	if in.Usage != nil {
		resp.Usage = usageFromWire(in.Usage)
	}
	return resp, nil
}

func usageFromWire(u *usage) canonical.Usage {
	out := canonical.Usage{
		InputTokens:  u.PromptTokens,
		OutputTokens: u.CompletionTokens,
		HasUsage:     true,
	}
	if u.PromptTokensDetails != nil {
		out.CacheReadTokens = u.PromptTokensDetails.CachedTokens
	}
	if u.CompletionTokensDetails != nil {
		out.ReasoningTokens = u.CompletionTokensDetails.ReasoningTokens
	}
	return out
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
