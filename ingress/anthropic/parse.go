package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/inja-online/llm-gateway/canonical"
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
		TopK:          in.TopK,
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
		tc, parallel, err := parseToolChoice(in.ToolChoice)
		if err != nil {
			return nil, err
		}
		req.ToolChoice = tc
		req.ParallelToolCalls = parallel
	}
	if in.Thinking != nil {
		req.Thinking = parseThinking(in.Thinking)
	}
	if in.OutputConfig != nil {
		req.ResponseFormat = parseOutputConfig(in.OutputConfig)
		// Adaptive thinking effort may ride on output_config.effort.
		if in.OutputConfig.Effort != "" {
			if req.Thinking == nil {
				req.Thinking = &canonical.ThinkingConfig{}
			}
			if req.Thinking.Effort == "" {
				req.Thinking.Effort = in.OutputConfig.Effort
			}
		}
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

func parseThinking(tw *thinkingWire) *canonical.ThinkingConfig {
	if tw == nil {
		return nil
	}
	tc := &canonical.ThinkingConfig{BudgetTokens: tw.BudgetTokens}
	switch tw.Type {
	case "enabled":
		tc.Mode = canonical.ThinkingEnabled
	case "disabled":
		tc.Mode = canonical.ThinkingDisabled
	case "adaptive":
		tc.Mode = canonical.ThinkingAdaptive
	default:
		if tw.BudgetTokens != nil {
			tc.Mode = canonical.ThinkingEnabled
		}
	}
	return tc
}

func parseOutputConfig(oc *outputConfigWire) *canonical.ResponseFormat {
	if oc == nil || oc.Format == nil {
		return nil
	}
	f := oc.Format
	switch f.Type {
	case "json_schema":
		return &canonical.ResponseFormat{
			Kind:   canonical.ResponseFormatJSONSchema,
			Name:   f.Name,
			Schema: f.Schema,
		}
	case "json_object":
		// Not native Anthropic; accept if a client sends it via translate.
		return &canonical.ResponseFormat{Kind: canonical.ResponseFormatJSONObject}
	default:
		return nil
	}
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
	case "document":
		if b.Source == nil {
			return canonical.Block{}, false
		}
		doc := documentFromSource(b.Source)
		doc.Title = b.Title
		return canonical.Block{Type: canonical.BlockDocument, Document: doc}, true
	case "tool_use":
		return canonical.Block{Type: canonical.BlockToolUse, ID: b.ID, Name: b.Name, Input: b.Input}, true
	case "tool_result":
		result, resultBlocks := parseToolResultContent(b.Content)
		return canonical.Block{
			Type:         canonical.BlockToolResult,
			ToolUseID:    b.ToolUseID,
			Result:       result,
			ResultBlocks: resultBlocks,
			IsError:      b.IsError,
		}, true
	case "thinking":
		return canonical.Block{Type: canonical.BlockThinking, Text: b.Thinking, Signature: b.Signature}, true
	case "redacted_thinking":
		// Preserve opaque data so multi-turn Claude thinking history stays valid.
		return canonical.Block{
			Type:     canonical.BlockThinking,
			Text:     b.Data,
			Redacted: true,
		}, true
	}
	return canonical.Block{}, false
}

func documentFromSource(src *imageSourceWire) *canonical.DocumentSource {
	doc := &canonical.DocumentSource{MediaType: src.MediaType}
	switch src.Type {
	case "base64":
		doc.Kind = "base64"
		doc.Data = src.Data
	case "url":
		doc.Kind = "url"
		doc.Data = src.URL
	case "file", "file_id":
		doc.Kind = "file"
		if src.FileID != "" {
			doc.Data = src.FileID
		} else {
			doc.Data = src.Data
		}
	default:
		doc.Kind = src.Type
		if src.Data != "" {
			doc.Data = src.Data
		} else if src.URL != "" {
			doc.Data = src.URL
		} else {
			doc.Data = src.FileID
		}
	}
	return doc
}

// toolResultText is the plain-string view of tool_result content (tests + compat).
func toolResultText(raw json.RawMessage) string {
	s, _ := parseToolResultContent(raw)
	return s
}

// parseToolResultContent maps Anthropic tool_result content (string or array of
// blocks) into a plain Result string plus optional ResultBlocks for multimodal.
func parseToolResultContent(raw json.RawMessage) (string, []canonical.Block) {
	if len(raw) == 0 {
		return "", nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil
	}
	var blocks []block
	if json.Unmarshal(raw, &blocks) != nil {
		return string(raw), nil
	}
	var text string
	var out []canonical.Block
	multimodal := false
	for _, b := range blocks {
		switch b.Type {
		case "text":
			text += b.Text
			out = append(out, canonical.Block{Type: canonical.BlockText, Text: b.Text})
		case "image":
			multimodal = true
			if b.Source == nil {
				continue
			}
			img := &canonical.ImageSource{Kind: b.Source.Type}
			if b.Source.Type == "base64" {
				img.MediaType = b.Source.MediaType
				img.Data = b.Source.Data
			} else {
				img.Data = b.Source.URL
			}
			out = append(out, canonical.Block{Type: canonical.BlockImage, Image: img})
		default:
			// Unknown nested types ignored for tool_result.
		}
	}
	if !multimodal && len(out) <= 1 {
		// Pure text array collapses to Result only (backward compatible).
		return text, nil
	}
	return text, out
}

// parseToolChoice maps Anthropic tool_choice and optional disable_parallel_tool_use.
// Polarity: disable_parallel_tool_use=true ↔ ParallelToolCalls=false.
// Unset disable field leaves ParallelToolCalls nil (omit on egress).
func parseToolChoice(raw json.RawMessage) (*canonical.ToolChoice, *bool, error) {
	var obj struct {
		Type                   string `json:"type"`
		Name                   string `json:"name"`
		DisableParallelToolUse *bool  `json:"disable_parallel_tool_use"`
	}
	if json.Unmarshal(raw, &obj) != nil {
		return nil, nil, &ValidationError{Msg: "invalid tool_choice"}
	}
	var parallel *bool
	if obj.DisableParallelToolUse != nil {
		// invert: disable=true → parallel=false
		v := !*obj.DisableParallelToolUse
		parallel = &v
	}
	var tc *canonical.ToolChoice
	switch obj.Type {
	case "auto":
		tc = &canonical.ToolChoice{Mode: canonical.ToolAuto}
	case "none":
		tc = &canonical.ToolChoice{Mode: canonical.ToolNone}
	case "any":
		tc = &canonical.ToolChoice{Mode: canonical.ToolRequired}
	case "tool":
		tc = &canonical.ToolChoice{Mode: canonical.ToolSpecific, Name: obj.Name}
	default:
		return nil, nil, &ValidationError{Msg: fmt.Sprintf("unknown tool_choice type %q", obj.Type)}
	}
	return tc, parallel, nil
}
