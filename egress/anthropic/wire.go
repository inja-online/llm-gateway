// Package anthropic implements the Anthropic Messages provider: building an
// Anthropic wire request from canonical form and parsing Anthropic responses
// and streams back into canonical form.
package anthropic

import "encoding/json"

// --- request wire types ---

type messagesRequest struct {
	Model         string          `json:"model"`
	System        []systemBlock   `json:"system,omitempty"`
	Messages      []message       `json:"messages"`
	MaxTokens     int             `json:"max_tokens"`
	Stream        bool            `json:"stream,omitempty"`
	Temperature   *float64        `json:"temperature,omitempty"`
	TopP          *float64        `json:"top_p,omitempty"`
	StopSequences []string        `json:"stop_sequences,omitempty"`
	Tools         []tool          `json:"tools,omitempty"`
	ToolChoice    json.RawMessage `json:"tool_choice,omitempty"`
}

type systemBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type message struct {
	Role    string  `json:"role"`
	Content []block `json:"content"`
}

// block is the wire form of a content block in both request and response.
type block struct {
	Type string `json:"type"`

	Text string `json:"text,omitempty"`

	// image
	Source *imageSourceWire `json:"source,omitempty"`

	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"` // string or array
	IsError   bool            `json:"is_error,omitempty"`

	// thinking
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`
}

type imageSourceWire struct {
	Type      string `json:"type"` // base64 | url
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

type tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// --- response wire types ---

type messagesResponse struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Role       string          `json:"role"`
	Model      string          `json:"model"`
	Content    []block         `json:"content"`
	StopReason string          `json:"stop_reason"`
	Usage      *anthropicUsage `json:"usage"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
