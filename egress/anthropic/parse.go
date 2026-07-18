package anthropic

import (
	"encoding/json"

	"github.com/inja-online/llm-gateway/canonical"
)

// ParseResponse converts a non-streaming Anthropic Messages response into a
// canonical response.
func ParseResponse(body []byte) (*canonical.Response, error) {
	var in messagesResponse
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, err
	}
	resp := &canonical.Response{
		ID:         in.ID,
		Model:      in.Model,
		StopReason: normalizeStop(in.StopReason),
	}
	for _, b := range in.Content {
		if cb, ok := parseBlock(b); ok {
			resp.Content = append(resp.Content, cb)
		}
	}
	if in.Usage != nil {
		resp.Usage = usageFromWire(in.Usage)
	}
	return resp, nil
}

func usageFromWire(u *anthropicUsage) canonical.Usage {
	return canonical.Usage{
		InputTokens:      u.InputTokens,
		OutputTokens:     u.OutputTokens,
		HasUsage:         true,
		CacheReadTokens:  u.CacheReadInputTokens,
		CacheWriteTokens: u.CacheCreationInputTokens,
	}
}

func normalizeStop(sr string) string {
	switch sr {
	case "end_turn", "max_tokens", "tool_use", "stop_sequence", "refusal":
		return sr
	case "":
		return canonical.StopEndTurn
	default:
		return sr
	}
}

func parseBlock(b block) (canonical.Block, bool) {
	switch b.Type {
	case "text":
		return canonical.Block{Type: canonical.BlockText, Text: b.Text}, true
	case "thinking":
		return canonical.Block{Type: canonical.BlockThinking, Text: b.Thinking, Signature: b.Signature}, true
	case "redacted_thinking":
		return canonical.Block{
			Type:     canonical.BlockThinking,
			Text:     b.Data,
			Redacted: true,
		}, true
	case "tool_use":
		return canonical.Block{
			Type:  canonical.BlockToolUse,
			ID:    b.ID,
			Name:  b.Name,
			Input: b.Input,
		}, true
	case "document":
		if b.Source == nil {
			return canonical.Block{}, false
		}
		doc := documentFromSource(b.Source)
		doc.Title = b.Title
		return canonical.Block{Type: canonical.BlockDocument, Document: doc}, true
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
