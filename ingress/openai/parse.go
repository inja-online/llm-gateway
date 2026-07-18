package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/inja-online/llm-gateway/canonical"
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
		Model:             in.Model,
		Stream:            in.Stream,
		Temperature:       in.Temperature,
		TopP:              in.TopP,
		ServiceTier:       in.ServiceTier,
		ParallelToolCalls: in.ParallelToolCalls,
		FrequencyPenalty:  in.FrequencyPenalty,
		PresencePenalty:   in.PresencePenalty,
		Seed:              in.Seed,
	}
	// Multi-choice policy: translation only supports a single choice (n=1).
	// n omitted or n=1 is accepted; n>1 is rejected so clients never silently
	// receive one of many requested choices.
	if in.N != nil {
		if *in.N != 1 {
			return nil, &ValidationError{Msg: fmt.Sprintf("n=%d is not supported on the translation path; only n=1 is allowed", *in.N)}
		}
		req.N = 1
	}
	// max_tokens vs max_completion_tokens: prefer max_tokens when both set
	// (matches historical gateway behavior). Record source so OpenAI egress can
	// re-emit the correct field for reasoning models.
	switch {
	case in.MaxTokens != nil:
		req.MaxTokens = *in.MaxTokens
		req.MaxTokensField = canonical.MaxTokensSourceTokens
	case in.MaxCompletion != nil:
		req.MaxTokens = *in.MaxCompletion
		req.MaxTokensField = canonical.MaxTokensSourceCompletionTokens
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
	if rf, err := parseResponseFormat(in.ResponseFormat); err != nil {
		return nil, err
	} else if rf != nil {
		req.ResponseFormat = rf
	}
	if in.ReasoningEffort != "" {
		req.Thinking = &canonical.ThinkingConfig{
			Mode:   canonical.ThinkingEnabled,
			Effort: in.ReasoningEffort,
		}
	}
	if err := parseMessages(in.Messages, req); err != nil {
		return nil, err
	}
	return req, nil
}

func parseResponseFormat(w *responseFormatWire) (*canonical.ResponseFormat, error) {
	if w == nil || w.Type == "" {
		return nil, nil
	}
	switch w.Type {
	case "text":
		return &canonical.ResponseFormat{Kind: canonical.ResponseFormatText}, nil
	case "json_object":
		return &canonical.ResponseFormat{Kind: canonical.ResponseFormatJSONObject}, nil
	case "json_schema":
		rf := &canonical.ResponseFormat{Kind: canonical.ResponseFormatJSONSchema}
		if w.JSONSchema != nil {
			rf.Name = w.JSONSchema.Name
			rf.Description = w.JSONSchema.Description
			rf.Schema = w.JSONSchema.Schema
			rf.Strict = w.JSONSchema.Strict
		}
		return rf, nil
	default:
		return nil, &ValidationError{Msg: fmt.Sprintf("unsupported response_format.type %q", w.Type)}
	}
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

// parseTools accepts function tools (type empty or "function"). Non-function
// tool types (e.g. custom, built-in) are rejected with bad_request so clients
// do not believe unsupported tools were registered. Policy: error, not skip.
func parseTools(tools []chatTool, req *canonical.Request) error {
	for _, t := range tools {
		if t.Type != "" && t.Type != "function" {
			return &ValidationError{Msg: fmt.Sprintf("unsupported tool type %q; only type \"function\" is supported on the translation path", t.Type)}
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
			// reasoning_content → BlockThinking (required for multi-turn tool loops).
			if thinking := parseReasoningContent(m.Reasoning); thinking != nil {
				blocks = append(blocks, *thinking)
			}
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
			tr, err := parseToolMessageContent(m.Content)
			if err != nil {
				return err
			}
			tr.Type = canonical.BlockToolResult
			tr.ToolUseID = m.ToolCallID
			appendUserBlocks(req, []canonical.Block{tr})

		default:
			return &ValidationError{Msg: fmt.Sprintf("unsupported message role %q", m.Role)}
		}
	}
	return nil
}

// parseReasoningContent maps assistant reasoning_content into a thinking block.
// Accepts a JSON string (common) or raw text; empty/null yields nil.
func parseReasoningContent(raw json.RawMessage) *canonical.Block {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if s == "" {
			return nil
		}
		return &canonical.Block{Type: canonical.BlockThinking, Text: s}
	}
	// Non-string JSON: preserve as opaque text of the raw bytes.
	t := strings.TrimSpace(string(raw))
	if t == "" {
		return nil
	}
	return &canonical.Block{Type: canonical.BlockThinking, Text: t}
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

// parseToolMessageContent maps role:tool content into a tool_result block.
// String content → Result; array of parts → ResultBlocks (+ concatenated Result).
func parseToolMessageContent(raw json.RawMessage) (canonical.Block, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return canonical.Block{}, nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return canonical.Block{Result: s}, nil
	}
	blocks, err := parseContentBlocks(raw)
	if err != nil {
		// Fall back to raw string for non-standard tool content.
		return canonical.Block{Result: string(raw)}, nil
	}
	if len(blocks) == 0 {
		return canonical.Block{}, nil
	}
	// Single text part collapses to Result for compatibility.
	if len(blocks) == 1 && blocks[0].Type == canonical.BlockText {
		return canonical.Block{Result: blocks[0].Text}, nil
	}
	var text string
	for _, b := range blocks {
		if b.Type == canonical.BlockText {
			text += b.Text
		}
	}
	return canonical.Block{Result: text, ResultBlocks: blocks}, nil
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
				return nil, &ValidationError{Msg: "image_url part is missing image_url object"}
			}
			if p.ImageURL.URL == "" {
				return nil, &ValidationError{Msg: "image_url.url is required"}
			}
			img := parseImageURL(p.ImageURL.URL)
			img.Detail = p.ImageURL.Detail
			blocks = append(blocks, canonical.Block{
				Type:  canonical.BlockImage,
				Image: img,
			})
		case "input_audio":
			if p.InputAudio == nil {
				return nil, &ValidationError{Msg: "input_audio part is missing input_audio object"}
			}
			if p.InputAudio.Data == "" {
				return nil, &ValidationError{Msg: "input_audio.data is required"}
			}
			if p.InputAudio.Format == "" {
				return nil, &ValidationError{Msg: "input_audio.format is required"}
			}
			blocks = append(blocks, canonical.Block{
				Type: canonical.BlockAudio,
				Audio: &canonical.AudioSource{
					Kind:      "base64",
					Data:      p.InputAudio.Data,
					Format:    p.InputAudio.Format,
					MediaType: audioFormatMediaType(p.InputAudio.Format),
				},
			})
		case "file":
			if p.File == nil {
				return nil, &ValidationError{Msg: "file part is missing file object"}
			}
			doc, err := parseFilePart(p.File)
			if err != nil {
				return nil, err
			}
			blocks = append(blocks, canonical.Block{
				Type:     canonical.BlockDocument,
				Document: doc,
			})
		default:
			// Unknown part types are ignored (forward-compat); incomplete known
			// types above return validation errors.
		}
	}
	return blocks, nil
}

func parseFilePart(f *fileObject) (*canonical.DocumentSource, error) {
	switch {
	case f.FileID != "":
		return &canonical.DocumentSource{
			Kind:     "file_id",
			Data:     f.FileID,
			Filename: f.Filename,
		}, nil
	case f.FileData != "":
		doc := &canonical.DocumentSource{Filename: f.Filename}
		if strings.HasPrefix(f.FileData, "data:") {
			// data:<media>;base64,<payload>
			rest := f.FileData[len("data:"):]
			meta, data, ok := strings.Cut(rest, ",")
			if !ok {
				return nil, &ValidationError{Msg: "file.file_data data URL is incomplete"}
			}
			mediaType, enc, _ := strings.Cut(meta, ";")
			if enc != "base64" {
				return nil, &ValidationError{Msg: "file.file_data must be a base64 data URL"}
			}
			doc.Kind = "base64"
			doc.MediaType = mediaType
			doc.Data = data
			return doc, nil
		}
		doc.Kind = "base64"
		doc.Data = f.FileData
		if doc.MediaType == "" {
			doc.MediaType = "application/octet-stream"
		}
		return doc, nil
	default:
		return nil, &ValidationError{Msg: "file part requires file_id or file_data"}
	}
}

func audioFormatMediaType(format string) string {
	switch strings.ToLower(format) {
	case "wav":
		return "audio/wav"
	case "mp3":
		return "audio/mpeg"
	case "flac":
		return "audio/flac"
	case "opus":
		return "audio/opus"
	case "pcm16":
		return "audio/pcm"
	default:
		if format == "" {
			return ""
		}
		return "audio/" + strings.ToLower(format)
	}
}
