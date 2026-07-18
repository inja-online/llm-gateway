package openai

import (
	"encoding/json"

	"github.com/inja-online/llm-gateway/canonical"
)

// stopReasonToFinish maps a canonical stop reason to an OpenAI finish_reason.
func stopReasonToFinish(sr string) string {
	switch sr {
	case canonical.StopEndTurn:
		return "stop"
	case canonical.StopMaxTokens:
		return "length"
	case canonical.StopToolUse:
		return "tool_calls"
	case canonical.StopStopSequence:
		return "stop"
	case canonical.StopRefusal:
		return "content_filter"
	default:
		return "stop"
	}
}

// SerializeResponse renders a canonical response as an OpenAI chat-completions
// JSON body. created is the unix timestamp to stamp.
func SerializeResponse(resp *canonical.Response, created int64) ([]byte, error) {
	out := chatResponse{
		ID:      responseID(resp.ID),
		Object:  "chat.completion",
		Created: created,
		Model:   resp.Model,
		Choices: []chatChoice{{
			Index:        0,
			Message:      blocksToOutMsg(resp.Content),
			FinishReason: ptr(stopReasonToFinish(resp.StopReason)),
		}},
	}
	if resp.Usage.HasUsage {
		out.Usage = &usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		}
	}
	return json.Marshal(out)
}

// blocksToOutMsg flattens canonical content blocks into a single OpenAI
// assistant message: text blocks concatenate into content; tool_use blocks
// become tool_calls; thinking maps to reasoning_content.
func blocksToOutMsg(blocks []canonical.Block) chatOutMsg {
	msg := chatOutMsg{Role: canonical.RoleAssistant}
	var text string
	for _, b := range blocks {
		switch b.Type {
		case canonical.BlockText:
			text += b.Text
		case canonical.BlockThinking:
			// Represent thinking as reasoning_content (best-effort, JSON string).
			if b.Text != "" {
				raw, _ := json.Marshal(b.Text)
				msg.Reasoning = raw
			}
		case canonical.BlockToolUse:
			args := string(b.Input)
			if args == "" {
				args = "{}"
			}
			msg.ToolCalls = append(msg.ToolCalls, outToolCall{
				ID:       b.ID,
				Type:     "function",
				Function: outFunctionDelta{Name: b.Name, Arguments: args},
			})
		}
	}
	if text != "" || len(msg.ToolCalls) == 0 {
		msg.Content = &text
	}
	return msg
}

func ptr[T any](v T) *T { return &v }

func responseID(id string) string {
	if id == "" {
		return "chatcmpl-gateway"
	}
	return id
}
