package google

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/inja-online/llm-gateway/canonical"
)

// ParseRequest converts a Gemini generateContent body into canonical form.
// modelFromPath is used when the body omits model (native path-style clients).
func ParseRequest(body []byte, modelFromPath string) (*canonical.Request, error) {
	var in generateRequest
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, &ValidationError{Msg: "request body is not valid JSON"}
	}
	model := in.Model
	if model == "" {
		model = modelFromPath
	}
	if model == "" {
		return nil, &ValidationError{Msg: "missing required field: model"}
	}
	if len(in.Contents) == 0 {
		return nil, &ValidationError{Msg: "missing required field: contents"}
	}

	req := &canonical.Request{Model: model}
	// safetySettings: preserve raw JSON for Google egress re-emit.
	if len(in.SafetySettings) > 0 {
		req.SafetySettings = in.SafetySettings
	} else if len(in.SafetySettingsCamel) > 0 {
		req.SafetySettings = in.SafetySettingsCamel
	}
	if in.CachedContent != "" {
		req.CachedContent = in.CachedContent
	} else if in.CachedContentCamel != "" {
		req.CachedContent = in.CachedContentCamel
	}
	gc := in.GenerationConfig
	if gc == nil {
		gc = in.GenerationConfigCamel
	}
	if gc != nil {
		req.Temperature = gc.Temperature
		req.TopP = gc.topP()
		req.MaxTokens = gc.maxOutputTokens()
		req.StopSequences = gc.stopSequences()
		req.TopK = gc.topK()
		req.Seed = gc.Seed
		if rf := parseResponseFormat(gc); rf != nil {
			req.ResponseFormat = rf
		}
		if tc := parseThinkingConfig(gc.thinking()); tc != nil {
			req.Thinking = tc
		}
		// Multi-candidate policy: only candidateCount=1 (or unset) is supported.
		cc := gc.CandidateCount
		if gc.CandidateCountCamel > 0 {
			cc = gc.CandidateCountCamel
		}
		if cc > 1 {
			return nil, &ValidationError{Msg: fmt.Sprintf("candidateCount=%d is not supported on the translation path; only candidateCount=1 is allowed", cc)}
		}
		if cc == 1 {
			req.N = 1
		}
	}
	if in.SystemInstruction != nil {
		for _, p := range in.SystemInstruction.Parts {
			if p.Text != "" {
				req.System = append(req.System, canonical.Block{Type: canonical.BlockText, Text: p.Text})
			}
		}
	}
	for _, t := range in.Tools {
		for _, fd := range t.FunctionDeclarations {
			if fd.Name == "" {
				return nil, &ValidationError{Msg: "function declaration name is required"}
			}
			req.Tools = append(req.Tools, canonical.Tool{
				Name:        fd.Name,
				Description: fd.Description,
				Schema:      fd.Parameters,
			})
		}
	}
	if in.ToolConfig != nil && in.ToolConfig.FunctionCallingConfig != nil {
		tc, err := parseToolChoice(in.ToolConfig.FunctionCallingConfig)
		if err != nil {
			return nil, err
		}
		req.ToolChoice = tc
	}
	for _, c := range in.Contents {
		msg, err := parseContent(c)
		if err != nil {
			return nil, err
		}
		req.Messages = append(req.Messages, msg)
	}
	return req, nil
}

// parseResponseFormat maps responseMimeType + responseSchema/responseJsonSchema
// into canonical ResponseFormat (#28).
func parseResponseFormat(gc *generationConfig) *canonical.ResponseFormat {
	if gc == nil {
		return nil
	}
	mime := strings.ToLower(strings.TrimSpace(gc.responseMIMEType()))
	schema := gc.responseSchema()
	hasSchema := len(schema) > 0 && string(schema) != "null"

	switch {
	case hasSchema:
		// Schema present → json_schema (mime typically application/json).
		return &canonical.ResponseFormat{
			Kind:   canonical.ResponseFormatJSONSchema,
			Schema: schema,
		}
	case mime == "application/json" || mime == "text/x.enum":
		// JSON mode without schema.
		return &canonical.ResponseFormat{Kind: canonical.ResponseFormatJSONObject}
	case mime == "text/plain":
		return &canonical.ResponseFormat{Kind: canonical.ResponseFormatText}
	case mime != "":
		// Unknown mime: treat application/*json-ish already handled; leave unset
		// rather than invent a kind. text/plain above covers free-form.
		return nil
	default:
		return nil
	}
}

// parseThinkingConfig maps generationConfig.thinkingConfig → ThinkingConfig (#30).
// thinkingBudget 0 → type "disabled". Non-zero budget → "enabled".
// thinkingLevel maps best-effort onto Effort.
func parseThinkingConfig(tw *thinkingConfigWire) *canonical.ThinkingConfig {
	if tw == nil {
		return nil
	}
	budget := tw.thinkingBudget()
	include := tw.includeThoughts()
	level := tw.thinkingLevel()
	if budget == nil && include == nil && level == "" {
		return nil
	}
	tc := &canonical.ThinkingConfig{
		BudgetTokens:    budget,
		IncludeThoughts: include,
	}
	if budget != nil && *budget == 0 {
		tc.Type = "disabled"
	} else if budget != nil || level != "" {
		tc.Type = "enabled"
	}
	if level != "" {
		tc.Effort = thinkingLevelToEffort(level)
	}
	return tc
}

func thinkingLevelToEffort(level string) string {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "MINIMAL", "THINKING_LEVEL_MINIMAL":
		return "minimal"
	case "LOW", "THINKING_LEVEL_LOW":
		return "low"
	case "MEDIUM", "THINKING_LEVEL_MEDIUM":
		return "medium"
	case "HIGH", "THINKING_LEVEL_HIGH":
		return "high"
	default:
		return strings.ToLower(level)
	}
}

func parseToolChoice(fc *functionCallingConfig) (*canonical.ToolChoice, error) {
	switch fc.Mode {
	case "", "AUTO", "auto":
		return &canonical.ToolChoice{Mode: canonical.ToolAuto}, nil
	case "NONE", "none":
		return &canonical.ToolChoice{Mode: canonical.ToolNone}, nil
	case "ANY", "any":
		if len(fc.AllowedFunctionNames) == 1 {
			return &canonical.ToolChoice{Mode: canonical.ToolSpecific, Name: fc.AllowedFunctionNames[0]}, nil
		}
		return &canonical.ToolChoice{Mode: canonical.ToolRequired}, nil
	default:
		return nil, &ValidationError{Msg: fmt.Sprintf("unknown function_calling_config.mode %q", fc.Mode)}
	}
}

func parseContent(c content) (canonical.Message, error) {
	role := mapRoleIn(c.Role)
	var blocks []canonical.Block
	for i, p := range c.Parts {
		switch {
		case p.Text != "" && p.Thought:
			blocks = append(blocks, canonical.Block{
				Type:      canonical.BlockThinking,
				Text:      p.Text,
				Signature: p.ThoughtSignature,
			})
		case p.Text != "":
			blocks = append(blocks, canonical.Block{Type: canonical.BlockText, Text: p.Text})
		case p.InlineData != nil || p.InlineDataCamel != nil:
			bl := p.InlineData
			if bl == nil {
				bl = p.InlineDataCamel
			}
			if bl != nil && bl.Data != "" {
				blocks = append(blocks, mediaBlockFromBlob(bl.mime(), "base64", bl.Data))
			}
		case p.FileData != nil || p.FileDataCamel != nil:
			fd := p.FileData
			if fd == nil {
				fd = p.FileDataCamel
			}
			if uri := fd.uri(); uri != "" {
				// file_uri → Kind "url" so Google egress can re-emit file_data.
				blocks = append(blocks, mediaBlockFromBlob(fd.mime(), "url", uri))
			}
		case p.FunctionCall != nil:
			args := p.FunctionCall.Args
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			id := fmt.Sprintf("call_%s_%d", p.FunctionCall.Name, i)
			blocks = append(blocks, canonical.Block{
				Type:  canonical.BlockToolUse,
				ID:    id,
				Name:  p.FunctionCall.Name,
				Input: args,
			})
		case p.FunctionResponse != nil:
			result := string(p.FunctionResponse.Response)
			if result == "" {
				result = "{}"
			}
			// Gemini function responses identify the tool by name; we stash name in ToolUseID
			// when no id is available (cross-dialect best-effort).
			blocks = append(blocks, canonical.Block{
				Type:      canonical.BlockToolResult,
				ToolUseID: p.FunctionResponse.Name,
				Result:    result,
			})
		}
	}
	return canonical.Message{Role: role, Content: blocks}, nil
}

// mediaBlockFromBlob maps inline_data / file_data into BlockDocument for PDF
// (#32) and BlockImage otherwise (images + temporary policy for non-PDF audio).
func mediaBlockFromBlob(mime, kind, data string) canonical.Block {
	if isPDFMIME(mime) {
		return canonical.Block{
			Type: canonical.BlockDocument,
			Document: &canonical.DocumentSource{
				Kind:      kind,
				MediaType: mime,
				Data:      data,
			},
		}
	}
	return canonical.Block{
		Type: canonical.BlockImage,
		Image: &canonical.ImageSource{
			Kind:      kind,
			MediaType: mime,
			Data:      data,
		},
	}
}

func isPDFMIME(mime string) bool {
	return strings.EqualFold(strings.TrimSpace(mime), "application/pdf")
}

func mapRoleIn(role string) string {
	switch role {
	case "model", "assistant":
		return canonical.RoleAssistant
	default:
		return canonical.RoleUser
	}
}
