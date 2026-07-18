package openai

import (
	"encoding/json"
	"fmt"

	"github.com/mamad/llm-gateway/canonical"
)

// ParseRequest converts an OpenAI chat-completions request body into canonical
// form. Errors are ValidationError so the caller can render the OpenAI error
// envelope with the right HTTP status.
func ParseRequest(body []byte) (*canonical.Request, error) {
	var in chatRequest
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, &ValidationError{Msg: "request body is not valid JSON"}
	}
	if in.Model == "" {
		return nil, &ValidationError{Msg: "missing required field: model"}
	}

	req := &canonical.Request{
		Model:       in.Model,
		Stream:      in.Stream,
		Temperature: in.Temperature,
		TopP:        in.TopP,
	}
	switch {
	case in.MaxTokens != nil:
		req.MaxTokens = *in.MaxTokens
	case in.MaxCompletion != nil:
		req.MaxTokens = *in.MaxCompletion
	}
	if in.Stop != nil {
		stops, err := parseStop(in.Stop)
		if err != nil {
			return nil, err
		}
		req.StopSequences = stops
	}
	if err := parseTools(in.Tools, req); err != nil {
		return nil, err
	}
	if in.ToolChoice != nil {
		tc, err := parseToolChoice(in.ToolChoice)
		if err != nil {
			return nil, err
		}
		req.ToolChoice = tc
	}
	if err := parseMessages(in.Messages, req); err != nil {
		return nil, err
	}
	return req, nil
}

func parseStop(raw json.RawMessage) ([]string, error) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []string{s}, nil
	}
	var arr []string
	if json.Unmarshal(raw, &arr) == nil {
		return arr, nil
	}
	return nil, &ValidationError{Msg: "stop must be a string or array of strings"}
}

func parseTools(tools []chatTool, req *canonical.Request) error {
	for _, t := range tools {
		if t.Type != "" && t.Type != "function" {
			continue // only function tools are representable
		}
		if t.Function.Name == "" {
			return &ValidationError{Msg: "tool function name is required"}
		}
		req.Tools = append(req.Tools, canonical.Tool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Schema:      t.Function.Parameters,
		})
	}
	return nil
}

func parseToolChoice(raw json.RawMessage) (*canonical.ToolChoice, error) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		switch s {
		case "auto":
			return &canonical.ToolChoice{Mode: canonical.ToolAuto}, nil
		case "none":
			return &canonical.ToolChoice{Mode: canonical.ToolNone}, nil
		case "required":
			return &canonical.ToolChoice{Mode: canonical.ToolRequired}, nil
		default:
			return nil, &ValidationError{Msg: fmt.Sprintf("unknown tool_choice %q", s)}
		}
	}
	var obj struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if json.Unmarshal(raw, &obj) == nil && obj.Type == "function" {
		return &canonical.ToolChoice{Mode: canonical.ToolSpecific, Name: obj.Function.Name}, nil
	}
	return nil, &ValidationError{Msg: "invalid tool_choice"}
}

// parseMessages maps the flat OpenAI message list into canonical turns.
// system messages become req.System; consecutive tool results collapse into a
// single user turn holding tool_result blocks (Anthropic grouping).
func parseMessages(msgs []chatMessage, req *canonical.Request) error {
	for _, m := range msgs {
		switch m.Role {
		case "system", "developer":
			blocks, err := parseContentBlocks(m.Content)
			if err != nil {
				return err
			}
			req.System = append(req.System, blocks...)

		case "user":
			blocks, err := parseContentBlocks(m.Content)
			if err != nil {
				return err
			}
			appendUserBlocks(req, blocks)

		case "assistant":
			var blocks []canonical.Block
			if len(m.Content) > 0 && string(m.Content) != "null" {
				cb, err := parseContentBlocks(m.Content)
				if err != nil {
					return err
				}
				blocks = append(blocks, cb...)
			}
			for _, tc := range m.ToolCalls {
				args := tc.Function.Arguments
				if args == "" {
					args = "{}"
				}
				blocks = append(blocks, canonical.Block{
					Type:  canonical.BlockToolUse,
					ID:    tc.ID,
					Name:  tc.Function.Name,
					Input: json.RawMessage(args),
				})
			}
			req.Messages = append(req.Messages, canonical.Message{Role: canonical.RoleAssistant, Content: blocks})

		case "tool":
			var result string
			if len(m.Content) > 0 {
				// tool content is normally a string; fall back to raw.
				if json.Unmarshal(m.Content, &result) != nil {
					result = string(m.Content)
				}
			}
			appendUserBlocks(req, []canonical.Block{{
				Type:      canonical.BlockToolResult,
				ToolUseID: m.ToolCallID,
				Result:    result,
			}})

		default:
			return &ValidationError{Msg: fmt.Sprintf("unsupported message role %q", m.Role)}
		}
	}
	return nil
}

// appendUserBlocks appends blocks to the last message if it is a user turn,
// otherwise starts a new user turn. This is what groups tool results.
func appendUserBlocks(req *canonical.Request, blocks []canonical.Block) {
	n := len(req.Messages)
	if n > 0 && req.Messages[n-1].Role == canonical.RoleUser {
		req.Messages[n-1].Content = append(req.Messages[n-1].Content, blocks...)
		return
	}
	req.Messages = append(req.Messages, canonical.Message{Role: canonical.RoleUser, Content: blocks})
}

// parseContentBlocks handles the three OpenAI content shapes: a JSON string,
// an array of parts (multimodal), or null.
func parseContentBlocks(raw json.RawMessage) ([]canonical.Block, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if s == "" {
			return nil, nil
		}
		return []canonical.Block{{Type: canonical.BlockText, Text: s}}, nil
	}
	var parts []contentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, &ValidationError{Msg: "message content must be a string or array of content parts"}
	}
	var blocks []canonical.Block
	for _, p := range parts {
		switch p.Type {
		case "text":
			blocks = append(blocks, canonical.Block{Type: canonical.BlockText, Text: p.Text})
		case "image_url":
			if p.ImageURL == nil {
				continue
			}
			blocks = append(blocks, canonical.Block{
				Type:  canonical.BlockImage,
				Image: parseImageURL(p.ImageURL.URL),
			})
		}
	}
	return blocks, nil
}
