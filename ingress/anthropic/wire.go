// Package anthropic implements the Anthropic Messages dialect on the ingress
// side: parsing /v1/messages requests into canonical form and serializing
// canonical responses/streams back to Anthropic's wire format.
package anthropic

import "encoding/json"

// --- request wire types ---

type messagesRequest struct {
	Model         string           `json:"model"`
	System        json.RawMessage  `json:"system,omitempty"` // string or []block
	Messages      []message        `json:"messages"`
	MaxTokens     int              `json:"max_tokens"`
	Stream        bool             `json:"stream,omitempty"`
	Temperature   *float64         `json:"temperature,omitempty"`
	TopP          *float64         `json:"top_p,omitempty"`
	TopK          *int             `json:"top_k,omitempty"`
	StopSequences []string         `json:"stop_sequences,omitempty"`
	Tools         []tool           `json:"tools,omitempty"`
	ToolChoice    json.RawMessage  `json:"tool_choice,omitempty"`
	Thinking      *thinkingWire    `json:"thinking,omitempty"`
	OutputConfig  *outputConfigWire `json:"output_config,omitempty"`
}

// thinkingWire is Anthropic extended/adaptive thinking request config.
// type: "enabled" | "disabled" | "adaptive"; budget_tokens for enabled.
type thinkingWire struct {
	Type         string `json:"type"`
	BudgetTokens *int   `json:"budget_tokens,omitempty"`
}

// outputConfigWire carries structured outputs (format) and optional effort.
type outputConfigWire struct {
	Format *outputFormatWire `json:"format,omitempty"`
	Effort string            `json:"effort,omitempty"` // adaptive thinking effort hint
}

type outputFormatWire struct {
	Type   string          `json:"type"` // "json_schema"
	Schema json.RawMessage `json:"schema,omitempty"`
	Name   string          `json:"name,omitempty"`
}

type message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // string or []block
}

type block struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`

	Source *imageSourceWire `json:"source,omitempty"`
	Title  string           `json:"title,omitempty"` // document

	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`

	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`
	Data      string `json:"data,omitempty"` // redacted_thinking opaque payload
}

type imageSourceWire struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
	FileID    string `json:"file_id,omitempty"`
}

type tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// --- response wire types ---

type messagesResponse struct {
	ID         string     `json:"id"`
	Type       string     `json:"type"`
	Role       string     `json:"role"`
	Model      string     `json:"model"`
	Content    []outBlock `json:"content"`
	StopReason string     `json:"stop_reason"`
	Usage      outUsage   `json:"usage"`
}

type outBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Signature string          `json:"signature,omitempty"`
	Data      string          `json:"data,omitempty"` // redacted_thinking
}

type outUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}
