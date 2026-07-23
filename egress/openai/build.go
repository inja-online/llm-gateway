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
		Model:                model,
		Stream:               req.Stream,
		Temperature:          req.Temperature,
		TopP:                 req.TopP,
		Stop:                 req.StopSequences,
		ServiceTier:          req.ServiceTier,
		PromptCacheKey:       req.PromptCacheKey,
		PromptCacheRetention: req.PromptCacheRetention,
		ParallelToolCalls:    req.ParallelToolCalls,
		FrequencyPenalty:     req.FrequencyPenalty,
		PresencePenalty:      req.PresencePenalty,
		Seed:                 req.Seed,
	}
	// Preserve max_tokens vs max_completion_tokens source for reasoning models.
	// Default (empty / max_tokens): emit max_tokens. Completion source: emit
	// max_completion_tokens only.
	if req.MaxTokens > 0 {
		switch req.MaxTokensField {
		case canonical.MaxTokensFieldMaxCompletionTokens:
			out.MaxCompletion = req.MaxTokens
		default:
			out.MaxTokens = req.MaxTokens
		}
	}
	if req.Stream {
		// Prefer client stream_options when present; else force include_usage for metering.
		if len(req.StreamOptions) > 0 {
			var so streamOptions
			if json.Unmarshal(req.StreamOptions, &so) == nil {
				out.StreamOpts = &so
			}
		}
		if out.StreamOpts == nil {
			out.StreamOpts = &streamOptions{IncludeUsage: true}
		} else if !out.StreamOpts.IncludeUsage {
			// Always meter streams through the gateway.
			out.StreamOpts.IncludeUsage = true
		}
	}
	if req.Thinking != nil && req.Thinking.Effort != "" {
		out.ReasoningEffort = req.Thinking.Effort
	}
	if rf := buildResponseFormat(req.ResponseFormat); rf != nil {
		out.ResponseFormat = rf
	}
	// system blocks -> a single system message
	if sys := concatText(req.System); sys != "" {
		out.Messages = append(out.Messages, chatMessage{Role: "system", Content: jsonString(sys)})
	}
	for _, t := range req.Tools {
		out.Tools = append(out.Tools, buildOpenAITool(t))
	}
	// Fidelity fields (#163/#115) — OpenAI egress only.
	out.SafetyIdentifier = req.SafetyIdentifier
	out.Verbosity = req.Verbosity
	out.Prediction = req.Prediction
	out.PromptCacheOptions = req.PromptCacheOptions
	out.Logprobs = req.Logprobs
	out.TopLogprobs = req.TopLogprobs
	out.Modalities = append([]string(nil), req.Modalities...)
	out.User = req.User
	if req.ToolChoice != nil {
		out.ToolChoice = buildToolChoice(req.ToolChoice)
	}
	for _, m := range req.Messages {
		out.Messages = append(out.Messages, buildMessages(m)...)
	}
	return json.Marshal(out)
}

func buildOpenAITool(t canonical.Tool) chatTool {
	kind := t.Kind
	if kind == "" {
		kind = canonical.ToolKindFunction
	}
	switch kind {
	case canonical.ToolKindCustom:
		format := t.Extra
		if len(format) == 0 && (t.Grammar != "" || t.GrammarType != "") {
			// Reconstruct minimal format object.
			m := map[string]any{}
			if t.GrammarType != "" {
				m["type"] = t.GrammarType
			}
			if t.Grammar != "" {
				m["definition"] = t.Grammar
			}
			format, _ = json.Marshal(m)
		}
		return chatTool{
			Type: "custom",
			Custom: &customToolWire{
				Name:        t.Name,
				Description: t.Description,
				Format:      format,
			},
		}
	case canonical.ToolKindComputer, canonical.ToolKindServer:
		return chatTool{
			Type:        firstNonEmptyStr(kind, "server"),
			Name:        t.Name,
			Description: t.Description,
			Format:      t.Extra,
		}
	default:
		params := t.Schema
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object"}`)
		}
		return chatTool{
			Type: "function",
			Function: &toolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		}
	}
}

func firstNonEmptyStr(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

func buildResponseFormat(rf *canonical.ResponseFormat) *responseFormatWire {
	if rf == nil {
		return nil
	}
	switch rf.Kind {
	case canonical.ResponseFormatText:
		return &responseFormatWire{Type: "text"}
	case canonical.ResponseFormatJSONObject:
		return &responseFormatWire{Type: "json_object"}
	case canonical.ResponseFormatJSONSchema:
		w := &responseFormatWire{Type: "json_schema"}
		w.JSONSchema = &struct {
			Name        string          `json:"name,omitempty"`
			Description string          `json:"description,omitempty"`
			Schema      json.RawMessage `json:"schema,omitempty"`
			Strict      *bool           `json:"strict,omitempty"`
		}{
			Name:        rf.Name,
			Description: rf.Description,
			Schema:      rf.Schema,
			Strict:      rf.Strict,
		}
		return w
	default:
		return nil
	}
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
			// OpenAI policy for redacted thinking: omit (do not invent opaque
			// reasoning_content). Non-redacted thinking is preserved for
			// multi-turn tool loops (DeepSeek/Kimi/Z.AI).
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
				parts = append(parts, contentPart{
					Type: "image_url",
					ImageURL: &imageURLObject{
						URL:    imageDataURL(b.Image),
						Detail: b.Image.Detail,
					},
				})
			}
		case canonical.BlockAudio:
			if b.Audio != nil {
				format := audioMediaTypeFormat(b.Audio.MediaType)
				if format == "" {
					format = "wav"
				}
				if format == "" && b.Audio.MediaType != "" {
					format = mediaTypeToAudioFormat(b.Audio.MediaType)
				}
				parts = append(parts, contentPart{
					Type: "input_audio",
					InputAudio: &inputAudioObject{
						Data:   b.Audio.Data,
						Format: format,
					},
				})
			}
		case canonical.BlockDocument:
			if b.Document != nil {
				parts = append(parts, contentPart{
					Type: "file",
					File: documentToFile(b.Document),
				})
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

func documentToFile(d *canonical.DocumentSource) *fileObject {
	switch d.Kind {
	case "file_id":
		return &fileObject{FileID: d.Data, Filename: d.Filename}
	case "url":
		// OpenAI file parts do not take remote URLs natively; emit as file_data.
		return &fileObject{Filename: d.Filename, FileData: d.Data}
	default: // base64
		media := d.MediaType
		if media == "" {
			media = "application/octet-stream"
		}
		return &fileObject{
			Filename: d.Filename,
			FileData: "data:" + media + ";base64," + d.Data,
		}
	}
}

func mediaTypeToAudioFormat(mt string) string {
	switch mt {
	case "audio/wav", "audio/wave", "audio/x-wav":
		return "wav"
	case "audio/mpeg", "audio/mp3":
		return "mp3"
	case "audio/flac":
		return "flac"
	case "audio/opus":
		return "opus"
	case "audio/pcm":
		return "pcm16"
	default:
		return ""
	}
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
						Type: "image_url",
						ImageURL: &imageURLObject{
							URL:    imageDataURL(rb.Image),
							Detail: rb.Image.Detail,
						},
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

// audioMediaTypeFormat maps MIME types back to OpenAI input_audio format names.
func audioMediaTypeFormat(mediaType string) string {
	switch mediaType {
	case "audio/wav", "audio/x-wav", "wav":
		return "wav"
	case "audio/mpeg", "audio/mp3", "mp3":
		return "mp3"
	case "audio/mp4", "audio/m4a", "mp4", "m4a":
		return "mp4"
	case "audio/webm", "webm":
		return "webm"
	case "audio/ogg", "ogg":
		return "ogg"
	case "audio/flac", "flac":
		return "flac"
	default:
		if mediaType == "" {
			return ""
		}
		// Already a short format name
		if len(mediaType) <= 5 && !containsSlash(mediaType) {
			return mediaType
		}
		return "wav"
	}
}

func containsSlash(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return true
		}
	}
	return false
}
