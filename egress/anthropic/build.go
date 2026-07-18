package anthropic

import (
	"encoding/json"

	"github.com/mamad/llm-gateway/canonical"
)

// defaultMaxTokens is injected when the canonical request carries none, since
// Anthropic requires max_tokens.
const defaultMaxTokens = 4096

// BuildRequest converts a canonical request into an Anthropic Messages wire
// body. model is the upstream model id (already stripped of any provider
// prefix by the router).
func BuildRequest(req *canonical.Request, model string) ([]byte, error) {
	out := messagesRequest{
		Model:         model,
		MaxTokens:     req.MaxTokens,
		Stream:        req.Stream,
		Temperature:   req.Temperature,
		TopP:          req.TopP,
		StopSequences: req.StopSequences,
	}
	if out.MaxTokens <= 0 {
		out.MaxTokens = defaultMaxTokens
	}
	for _, b := range req.System {
		if b.Type == canonical.BlockText {
			out.System = append(out.System, systemBlock{Type: "text", Text: b.Text})
		}
	}
	for _, t := range req.Tools {
		schema := t.Schema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object"}`)
		}
		out.Tools = append(out.Tools, tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schema,
		})
	}
	if req.ToolChoice != nil {
		out.ToolChoice = buildToolChoice(req.ToolChoice)
	}
	for _, m := range req.Messages {
		wm := message{Role: m.Role}
		for _, b := range m.Content {
			wb, ok := buildBlock(b)
			if ok {
				wm.Content = append(wm.Content, wb)
			}
		}
		// Anthropic rejects empty content arrays; skip fully-empty turns.
		if len(wm.Content) == 0 {
			continue
		}
		out.Messages = append(out.Messages, wm)
	}
	return json.Marshal(out)
}

func buildToolChoice(tc *canonical.ToolChoice) json.RawMessage {
	switch tc.Mode {
	case canonical.ToolAuto:
		return json.RawMessage(`{"type":"auto"}`)
	case canonical.ToolNone:
		return json.RawMessage(`{"type":"none"}`)
	case canonical.ToolRequired:
		return json.RawMessage(`{"type":"any"}`)
	case canonical.ToolSpecific:
		raw, _ := json.Marshal(map[string]string{"type": "tool", "name": tc.Name})
		return raw
	}
	return nil
}

func buildBlock(b canonical.Block) (block, bool) {
	switch b.Type {
	case canonical.BlockText:
		if b.Text == "" {
			return block{}, false
		}
		return block{Type: "text", Text: b.Text}, true

	case canonical.BlockImage:
		if b.Image == nil {
			return block{}, false
		}
		src := &imageSourceWire{Type: b.Image.Kind}
		if b.Image.Kind == "base64" {
			src.MediaType = b.Image.MediaType
			src.Data = b.Image.Data
		} else {
			src.URL = b.Image.Data
		}
		return block{Type: "image", Source: src}, true

	case canonical.BlockToolUse:
		input := b.Input
		if len(input) == 0 {
			input = json.RawMessage(`{}`)
		}
		return block{Type: "tool_use", ID: b.ID, Name: b.Name, Input: input}, true

	case canonical.BlockToolResult:
		content, _ := json.Marshal(b.Result)
		return block{
			Type:      "tool_result",
			ToolUseID: b.ToolUseID,
			Content:   content,
			IsError:   b.IsError,
		}, true

	case canonical.BlockThinking:
		return block{Type: "thinking", Thinking: b.Text, Signature: b.Signature}, true
	}
	return block{}, false
}
