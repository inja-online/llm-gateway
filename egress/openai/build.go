package openai

import (
	"encoding/json"

	"github.com/inja-online/llm-gateway/canonical"
)

// BuildRequest converts a canonical request into an OpenAI chat-completions
// wire body. Canonical is Anthropic-shaped, so this flattens content blocks:
// system blocks become a system message, tool_use blocks become assistant
// tool_calls, and tool_result blocks become role:tool messages.
func BuildRequest(req *canonical.Request, model string) ([]byte, error) {
	out := chatRequest{
		Model:       model,
		MaxTokens:   req.MaxTokens,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stop:        req.StopSequences,
		ServiceTier: req.ServiceTier,
	}
	if req.Stream {
		out.StreamOpts = &streamOptions{IncludeUsage: true}
	}
	// system blocks -> a single system message
	if sys := concatText(req.System); sys != "" {
		out.Messages = append(out.Messages, chatMessage{Role: "system", Content: jsonString(sys)})
	}
	for _, t := range req.Tools {
		params := t.Schema
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object"}`)
		}
		out.Tools = append(out.Tools, chatTool{
			Type:     "function",
			Function: toolFunction{Name: t.Name, Description: t.Description, Parameters: params},
		})
	}
	if req.ToolChoice != nil {
		out.ToolChoice = buildToolChoice(req.ToolChoice)
	}
	for _, m := range req.Messages {
		out.Messages = append(out.Messages, buildMessages(m)...)
	}
	return json.Marshal(out)
}

// buildMessages expands one canonical turn into one or more OpenAI messages.
// A user turn with tool_result blocks becomes N role:tool messages (plus any
// text/image as a user message). An assistant turn with tool_use blocks
// becomes one assistant message carrying tool_calls.
func buildMessages(m canonical.Message) []chatMessage {
	if m.Role == canonical.RoleAssistant {
		return buildAssistant(m)
	}
	return buildUser(m)
}

func buildAssistant(m canonical.Message) []chatMessage {
	msg := chatMessage{Role: "assistant"}
	var text string
	var reasoning string
	for _, b := range m.Content {
		switch b.Type {
		case canonical.BlockText:
			text += b.Text
		case canonical.BlockThinking:
			// CRITICAL: preserve thinking for multi-turn tool loops (DeepSeek/Kimi/Z.AI).
			if !b.Redacted {
				reasoning += b.Text
			}
		case canonical.BlockToolUse:
			args := string(b.Input)
			if args == "" {
				args = "{}"
			}
			msg.ToolCalls = append(msg.ToolCalls, toolCall{
				ID:       b.ID,
				Type:     "function",
				Function: functionCall{Name: b.Name, Arguments: args},
			})
		}
	}
	if text != "" {
		msg.Content = jsonString(text)
	}
	if reasoning != "" {
		raw, _ := json.Marshal(reasoning)
		msg.Reasoning = raw
	}
	return []chatMessage{msg}
}

func buildUser(m canonical.Message) []chatMessage {
	var msgs []chatMessage
	var parts []contentPart
	for _, b := range m.Content {
		switch b.Type {
		case canonical.BlockText:
			parts = append(parts, contentPart{Type: "text", Text: b.Text})
		case canonical.BlockImage:
			if b.Image != nil {
				parts = append(parts, contentPart{Type: "image_url", ImageURL: &imageURLObject{URL: imageDataURL(b.Image)}})
			}
		case canonical.BlockToolResult:
			// tool results are separate role:tool messages.
			msgs = append(msgs, chatMessage{
				Role:       "tool",
				ToolCallID: b.ToolUseID,
				Content:    toolResultContent(b),
			})
		}
	}
	if len(parts) > 0 {
		// Tool results must come first: OpenAI requires role:tool messages to
		// directly follow the assistant turn that made the calls. Any user text
		// in the same canonical turn goes after them.
		var userMsg chatMessage
		if len(parts) == 1 && parts[0].Type == "text" {
			userMsg = chatMessage{Role: "user", Content: jsonString(parts[0].Text)}
		} else {
			raw, _ := json.Marshal(parts)
			userMsg = chatMessage{Role: "user", Content: raw}
		}
		msgs = append(msgs, userMsg)
	}
	return msgs
}

func buildToolChoice(tc *canonical.ToolChoice) json.RawMessage {
	switch tc.Mode {
	case canonical.ToolAuto:
		return json.RawMessage(`"auto"`)
	case canonical.ToolNone:
		return json.RawMessage(`"none"`)
	case canonical.ToolRequired:
		return json.RawMessage(`"required"`)
	case canonical.ToolSpecific:
		raw, _ := json.Marshal(map[string]any{"type": "function", "function": map[string]string{"name": tc.Name}})
		return raw
	}
	return nil
}

func concatText(blocks []canonical.Block) string {
	var s string
	for _, b := range blocks {
		if b.Type == canonical.BlockText {
			s += b.Text
		}
	}
	return s
}

func jsonString(s string) json.RawMessage {
	raw, _ := json.Marshal(s)
	return raw
}

func imageDataURL(img *canonical.ImageSource) string {
	if img.Kind == "base64" {
		return "data:" + img.MediaType + ";base64," + img.Data
	}
	return img.Data
}

// toolResultContent emits a string when only Result is set, or a multimodal
// content-part array when ResultBlocks is present.
func toolResultContent(b canonical.Block) json.RawMessage {
	if len(b.ResultBlocks) > 0 {
		var parts []contentPart
		for _, rb := range b.ResultBlocks {
			switch rb.Type {
			case canonical.BlockText:
				parts = append(parts, contentPart{Type: "text", Text: rb.Text})
			case canonical.BlockImage:
				if rb.Image != nil {
					parts = append(parts, contentPart{
						Type:     "image_url",
						ImageURL: &imageURLObject{URL: imageDataURL(rb.Image)},
					})
				}
			}
		}
		if len(parts) > 0 {
			raw, _ := json.Marshal(parts)
			return raw
		}
	}
	return jsonString(b.Result)
}
