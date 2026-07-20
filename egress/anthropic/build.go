package anthropic

import (
	"encoding/json"

	"github.com/inja-online/llm-gateway/canonical"
)

// defaultMaxTokens is injected when the canonical request carries none, since
// Anthropic requires max_tokens.
const defaultMaxTokens = 4096

// Drop policy (Anthropic egress): the following canonical fields have no
// Messages API equivalent and are omitted without error:
//   - FrequencyPenalty, PresencePenalty (#38)
//   - Seed (#39)
// OpenAI service_tier / Google safetySettings are also omitted.

// BuildRequest converts a canonical request into an Anthropic Messages wire
// body. model is the upstream model id (already stripped of any provider
// prefix by the router).
//
// Seed / frequency_penalty / presence_penalty are intentionally dropped
// (documented drop policy) — no error is returned when they are set.
func BuildRequest(req *canonical.Request, model string) ([]byte, error) {
	out := messagesRequest{
		Model:         model,
		MaxTokens:     req.MaxTokens,
		Stream:        req.Stream,
		Temperature:   req.Temperature,
		TopP:          req.TopP,
		TopK:          req.TopK,
		StopSequences: req.StopSequences,
	}
	if out.MaxTokens <= 0 {
		out.MaxTokens = defaultMaxTokens
	}
	for _, b := range req.System {
		if b.Type == canonical.BlockText {
			out.System = append(out.System, systemBlock{
				Type:         "text",
				Text:         b.Text,
				CacheControl: toWireCacheControl(b.CacheControl),
			})
		}
	}
	for _, t := range req.Tools {
		schema := t.Schema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object"}`)
		}
		out.Tools = append(out.Tools, tool{
			Name:         t.Name,
			Description:  t.Description,
			InputSchema:  schema,
			CacheControl: toWireCacheControl(t.CacheControl),
		})
	}
	if req.ToolChoice != nil || req.ParallelToolCalls != nil {
		out.ToolChoice = buildToolChoice(req.ToolChoice, req.ParallelToolCalls)
	}
	if tw := buildThinking(req.Thinking); tw != nil {
		out.Thinking = tw
	}
	if oc := buildOutputConfig(req.ResponseFormat, req.Thinking); oc != nil {
		out.OutputConfig = oc
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

// buildThinking maps canonical ThinkingConfig → Anthropic thinking object.
// Nil config or empty mode without budget yields nil (omit; no accidental enable).
func buildThinking(tc *canonical.ThinkingConfig) *thinkingWire {
	if tc == nil {
		return nil
	}
	mode := tc.Type
	if mode == "" {
		if tc.BudgetTokens != nil {
			mode = "enabled"
		} else if tc.Effort != "" {
			// OpenAI-style effort without explicit mode → adaptive on Anthropic
			// when no numeric budget, else enabled with mapped budget.
			mode = "adaptive"
		} else {
			return nil
		}
	}
	switch mode {
	case "disabled":
		return &thinkingWire{Type: "disabled"}
	case "adaptive":
		return &thinkingWire{Type: "adaptive"}
	case "enabled":
		budget := tc.BudgetTokens
		if budget == nil && tc.Effort != "" {
			budget = effortToBudget(tc.Effort)
		}
		return &thinkingWire{Type: "enabled", BudgetTokens: budget}
	default:
		return nil
	}
}

// effortToBudget is the documented best-effort OpenAI effort → Anthropic budget table.
func effortToBudget(effort string) *int {
	var n int
	switch effort {
	case "minimal", "low":
		n = 1024
	case "medium":
		n = 8192
	case "high", "xhigh", "max":
		n = 16384
	default:
		return nil
	}
	return &n
}

// buildOutputConfig maps ResponseFormat → output_config.format.
// json_schema preserves schema body; json_object has no native Anthropic
// equivalent without a schema — emitted as json_schema with empty object schema
// is avoided; we omit json_object (document drop) unless schema is present.
// Thinking effort may also be placed on output_config.effort for adaptive mode.
func buildOutputConfig(rf *canonical.ResponseFormat, thinking *canonical.ThinkingConfig) *outputConfigWire {
	var oc *outputConfigWire
	if rf != nil {
		switch rf.Kind {
		case canonical.ResponseFormatJSONSchema:
			if len(rf.Schema) > 0 || rf.Name != "" {
				oc = &outputConfigWire{
					Format: &outputFormatWire{
						Type:   "json_schema",
						Schema: rf.Schema,
						Name:   rf.Name,
					},
				}
			}
		case canonical.ResponseFormatJSONObject:
			// Anthropic only supports json_schema. Policy: drop json_object without schema.
			// Callers that need JSON mode should supply a schema.
		}
	}
	// Adaptive thinking effort rides on output_config when present.
	if thinking != nil && thinking.Effort != "" &&
		(thinking.Type == "adaptive" || thinking.Type == "") {
		if oc == nil {
			oc = &outputConfigWire{}
		}
		oc.Effort = thinking.Effort
	}
	return oc
}

// buildToolChoice emits Anthropic tool_choice including disable_parallel_tool_use.
// Polarity: ParallelToolCalls=false → disable_parallel_tool_use=true.
// Unset ParallelToolCalls omits the disable field entirely.
func buildToolChoice(tc *canonical.ToolChoice, parallel *bool) json.RawMessage {
	type wire struct {
		Type                   string `json:"type"`
		Name                   string `json:"name,omitempty"`
		DisableParallelToolUse *bool  `json:"disable_parallel_tool_use,omitempty"`
	}
	var w wire
	if tc != nil {
		switch tc.Mode {
		case canonical.ToolAuto:
			w.Type = "auto"
		case canonical.ToolNone:
			w.Type = "none"
		case canonical.ToolRequired:
			w.Type = "any"
		case canonical.ToolSpecific:
			w.Type = "tool"
			w.Name = tc.Name
		default:
			if parallel == nil {
				return nil
			}
			// Unknown mode but parallel set: default type auto so disable can ride.
			w.Type = "auto"
		}
	} else {
		// Parallel only: Anthropic requires a type; use auto.
		w.Type = "auto"
	}
	if parallel != nil {
		// invert: parallel=false → disable=true
		disable := !*parallel
		w.DisableParallelToolUse = &disable
	}
	raw, _ := json.Marshal(w)
	return raw
}

func buildBlock(b canonical.Block) (block, bool) {
	cc := toWireCacheControl(b.CacheControl)
	switch b.Type {
	case canonical.BlockText:
		if b.Text == "" {
			return block{}, false
		}
		return block{Type: "text", Text: b.Text, CacheControl: cc}, true

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
		return block{Type: "image", Source: src, CacheControl: cc}, true

	case canonical.BlockDocument:
		if b.Document == nil {
			return block{}, false
		}
		src := documentToSource(b.Document)
		return block{Type: "document", Source: src, Title: b.Document.Title, CacheControl: cc}, true

	case canonical.BlockToolUse:
		input := b.Input
		if len(input) == 0 {
			input = json.RawMessage(`{}`)
		}
		return block{Type: "tool_use", ID: b.ID, Name: b.Name, Input: input, CacheControl: cc}, true

	case canonical.BlockToolResult:
		return block{
			Type:         "tool_result",
			ToolUseID:    b.ToolUseID,
			Content:      buildToolResultContent(b),
			IsError:      b.IsError,
			CacheControl: cc,
		}, true

	case canonical.BlockThinking:
		if b.Redacted {
			return block{Type: "redacted_thinking", Data: b.Text, CacheControl: cc}, true
		}
		return block{Type: "thinking", Thinking: b.Text, Signature: b.Signature, CacheControl: cc}, true
	}
	return block{}, false
}

func toWireCacheControl(cc *canonical.CacheControl) *cacheControlWire {
	if cc == nil || (cc.Type == "" && cc.TTL == "") {
		return nil
	}
	w := &cacheControlWire{Type: cc.Type}
	if cc.TTL != "" {
		w.TTL = cc.TTL
	}
	if w.Type == "" {
		w.Type = "ephemeral"
	}
	return w
}

func documentToSource(doc *canonical.DocumentSource) *imageSourceWire {
	src := &imageSourceWire{Type: doc.Kind, MediaType: doc.MediaType}
	switch doc.Kind {
	case "base64":
		src.Data = doc.Data
	case "url":
		src.URL = doc.Data
	case "file":
		src.Type = "file"
		src.FileID = doc.Data
	default:
		src.Data = doc.Data
	}
	return src
}

// buildToolResultContent emits a JSON string when only Result is set, or an
// array of content blocks when ResultBlocks carries multimodal tool output.
func buildToolResultContent(b canonical.Block) json.RawMessage {
	if len(b.ResultBlocks) > 0 {
		var blocks []block
		for _, rb := range b.ResultBlocks {
			if wb, ok := buildBlock(rb); ok {
				// Only text/image (and thinking) make sense inside tool_result.
				if wb.Type == "text" || wb.Type == "image" {
					blocks = append(blocks, wb)
				}
			}
		}
		if len(blocks) > 0 {
			raw, _ := json.Marshal(blocks)
			return raw
		}
	}
	content, _ := json.Marshal(b.Result)
	return content
}
