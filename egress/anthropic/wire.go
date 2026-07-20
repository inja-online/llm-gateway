// Package anthropic implements the Anthropic Messages provider: building an
// Anthropic wire request from canonical form and parsing Anthropic responses
// and streams back into canonical form.
package anthropic

import "encoding/json"

// --- request wire types ---

type messagesRequest struct {
	Model         string            `json:"model"`
	System        []systemBlock     `json:"system,omitempty"`
	Messages      []message         `json:"messages"`
	MaxTokens     int               `json:"max_tokens"`
	Stream        bool              `json:"stream,omitempty"`
	Temperature   *float64          `json:"temperature,omitempty"`
	TopP          *float64          `json:"top_p,omitempty"`
	TopK          *int              `json:"top_k,omitempty"`
	StopSequences []string          `json:"stop_sequences,omitempty"`
	Tools         []tool            `json:"tools,omitempty"`
	ToolChoice    json.RawMessage   `json:"tool_choice,omitempty"`
	Thinking      *thinkingWire     `json:"thinking,omitempty"`
	OutputConfig  *outputConfigWire `json:"output_config,omitempty"`
}

// thinkingWire is Anthropic extended/adaptive thinking request config.
type thinkingWire struct {
	Type         string `json:"type"` // enabled | disabled | adaptive
	BudgetTokens *int   `json:"budget_tokens,omitempty"`
}

type outputConfigWire struct {
	Format *outputFormatWire `json:"format,omitempty"`
	Effort string            `json:"effort,omitempty"`
}

type outputFormatWire struct {
	Type   string          `json:"type"` // json_schema
	Schema json.RawMessage `json:"schema,omitempty"`
	Name   string          `json:"name,omitempty"`
}

type systemBlock struct {
	Type         string            `json:"type"`
	Text         string            `json:"text"`
	CacheControl *cacheControlWire `json:"cache_control,omitempty"`
}

// cacheControlWire is Anthropic cache_control on system/content/tools (#108).
type cacheControlWire struct {
	Type string `json:"type"`
	TTL  string `json:"ttl,omitempty"`
}

type message struct {
	Role    string  `json:"role"`
	Content []block `json:"content"`
}

// block is the wire form of a content block in both request and response.
type block struct {
	Type string `json:"type"`

	Text string `json:"text,omitempty"`

	// image / document
	Source *imageSourceWire `json:"source,omitempty"`
	Title  string           `json:"title,omitempty"`

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
	// redacted_thinking opaque payload
	Data string `json:"data,omitempty"`

	CacheControl *cacheControlWire `json:"cache_control,omitempty"`
}

type imageSourceWire struct {
	Type      string `json:"type"` // base64 | url | file
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
	FileID    string `json:"file_id,omitempty"`
}

type tool struct {
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	InputSchema  json.RawMessage   `json:"input_schema"`
	CacheControl *cacheControlWire `json:"cache_control,omitempty"`
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
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}
