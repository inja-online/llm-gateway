package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/mamad/llm-gateway/canonical"
)

// ValidationError marks a client request problem (HTTP 400 class).
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// ParseRequest converts an Anthropic /v1/messages request body into canonical
// form.
func ParseRequest(body []byte) (*canonical.Request, error) {
	var in messagesRequest
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, &ValidationError{Msg: "request body is not valid JSON"}
	}
	if in.Model == "" {
		return nil, &ValidationError{Msg: "missing required field: model"}
	}
	if in.MaxTokens <= 0 {
		return nil, &ValidationError{Msg: "max_tokens must be a positive integer"}
	}
	req := &canonical.Request{
		Model:         in.Model,
		MaxTokens:     in.MaxTokens,
		Stream:        in.Stream,
		Temperature:   in.Temperature,
		TopP:          in.TopP,
		StopSequences: in.StopSequences,
	}
	sys, err := parseSystem(in.System)
	if err != nil {
		return nil, err
	}
	req.System = sys

	for _, t := range in.Tools {
		req.Tools = append(req.Tools, canonical.Tool{
			Name:        t.Name,
			Description: t.Description,
			Schema:      t.InputSchema,
		})
	}
	if in.ToolChoice != nil {
		tc, err := parseToolChoice(in.ToolChoice)
		if err != nil {
			return nil, err
		}
		req.ToolChoice = tc
	}
	for _, m := range in.Messages {
		blocks, err := parseContent(m.Content)
		if err != nil {
			return nil, err
		}
		req.Messages = append(req.Messages, canonical.Message{Role: m.Role, Content: blocks})
	}
	return req, nil
}

func parseSystem(raw json.RawMessage) ([]canonical.Block, error) {
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
	var blocks []block
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, &ValidationError{Msg: "system must be a string or array of text blocks"}
	}
	var out []canonical.Block
	for _, b := range blocks {
		if b.Type == "text" {
			out = append(out, canonical.Block{Type: canonical.BlockText, Text: b.Text})
		}
	}
	return out, nil
}

func parseContent(raw json.RawMessage) ([]canonical.Block, error) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if s == "" {
			return nil, nil
		}
		return []canonical.Block{{Type: canonical.BlockText, Text: s}}, nil
	}
	var blocks []block
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, &ValidationError{Msg: "message content must be a string or array of blocks"}
	}
	var out []canonical.Block
	for _, b := range blocks {
		if cb, ok := parseBlock(b); ok {
			out = append(out, cb)
		}
	}
	return out, nil
}

func parseBlock(b block) (canonical.Block, bool) {
	switch b.Type {
	case "text":
		return canonical.Block{Type: canonical.BlockText, Text: b.Text}, true
	case "image":
		if b.Source == nil {
			return canonical.Block{}, false
		}
		img := &canonical.ImageSource{Kind: b.Source.Type}
		if b.Source.Type == "base64" {
			img.MediaType = b.Source.MediaType
			img.Data = b.Source.Data
		} else {
			img.Data = b.Source.URL
		}
		return canonical.Block{Type: canonical.BlockImage, Image: img}, true
	case "tool_use":
		return canonical.Block{Type: canonical.BlockToolUse, ID: b.ID, Name: b.Name, Input: b.Input}, true
	case "tool_result":
		return canonical.Block{
			Type:      canonical.BlockToolResult,
			ToolUseID: b.ToolUseID,
			Result:    toolResultText(b.Content),
			IsError:   b.IsError,
		}, true
	case "thinking":
		return canonical.Block{Type: canonical.BlockThinking, Text: b.Thinking, Signature: b.Signature}, true
	}
	return canonical.Block{}, false
}

// toolResultText flattens an Anthropic tool_result content (string, or array of
// text blocks) into a plain string.
func toolResultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []block
	if json.Unmarshal(raw, &blocks) == nil {
		var out string
		for _, b := range blocks {
			if b.Type == "text" {
				out += b.Text
			}
		}
		return out
	}
	return string(raw)
}

func parseToolChoice(raw json.RawMessage) (*canonical.ToolChoice, error) {
	var obj struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &obj) != nil {
		return nil, &ValidationError{Msg: "invalid tool_choice"}
	}
	switch obj.Type {
	case "auto":
		return &canonical.ToolChoice{Mode: canonical.ToolAuto}, nil
	case "none":
		return &canonical.ToolChoice{Mode: canonical.ToolNone}, nil
	case "any":
		return &canonical.ToolChoice{Mode: canonical.ToolRequired}, nil
	case "tool":
		return &canonical.ToolChoice{Mode: canonical.ToolSpecific, Name: obj.Name}, nil
	}
	return nil, &ValidationError{Msg: fmt.Sprintf("unknown tool_choice type %q", obj.Type)}
}
