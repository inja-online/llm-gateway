package google

import (
	"encoding/json"
	"fmt"

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
	if in.GenerationConfig != nil {
		req.Temperature = in.GenerationConfig.Temperature
		req.TopP = in.GenerationConfig.TopP
		req.MaxTokens = in.GenerationConfig.MaxOutputTokens
		req.StopSequences = in.GenerationConfig.StopSequences
		// Multi-candidate policy: only candidateCount=1 (or unset) is supported.
		cc := in.GenerationConfig.CandidateCount
		if in.GenerationConfig.CandidateCountCamel > 0 {
			cc = in.GenerationConfig.CandidateCountCamel
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
			blocks = append(blocks, canonical.Block{Type: canonical.BlockThinking, Text: p.Text})
		case p.Text != "":
			blocks = append(blocks, canonical.Block{Type: canonical.BlockText, Text: p.Text})
		case p.InlineData != nil:
			blocks = append(blocks, canonical.Block{
				Type: canonical.BlockImage,
				Image: &canonical.ImageSource{
					Kind:      "base64",
					MediaType: p.InlineData.MIMEType,
					Data:      p.InlineData.Data,
				},
			})
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

func mapRoleIn(role string) string {
	switch role {
	case "model", "assistant":
		return canonical.RoleAssistant
	default:
		return canonical.RoleUser
	}
}
