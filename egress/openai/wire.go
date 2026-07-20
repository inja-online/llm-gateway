// Package openai implements the OpenAI chat-completions provider (egress):
// building an OpenAI request from canonical form and parsing OpenAI responses
// and streams back into canonical form.
package openai

import "encoding/json"

// --- request wire types ---

type chatRequest struct {
	Model             string              `json:"model"`
	Messages          []chatMessage       `json:"messages"`
	MaxTokens         int                 `json:"max_tokens,omitempty"`
	MaxCompletion     int                 `json:"max_completion_tokens,omitempty"`
	Stream            bool                `json:"stream,omitempty"`
	StreamOpts        *streamOptions      `json:"stream_options,omitempty"`
	Temperature       *float64            `json:"temperature,omitempty"`
	TopP              *float64            `json:"top_p,omitempty"`
	Stop              []string            `json:"stop,omitempty"`
	Tools             []chatTool          `json:"tools,omitempty"`
	ToolChoice        json.RawMessage     `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool               `json:"parallel_tool_calls,omitempty"`
	FrequencyPenalty  *float64            `json:"frequency_penalty,omitempty"`
	PresencePenalty   *float64            `json:"presence_penalty,omitempty"`
	Seed              *int64              `json:"seed,omitempty"`
	ResponseFormat    *responseFormatWire `json:"response_format,omitempty"`
	ReasoningEffort      string `json:"reasoning_effort,omitempty"`
	ServiceTier          string `json:"service_tier,omitempty"`
	PromptCacheKey       string `json:"prompt_cache_key,omitempty"`
	PromptCacheRetention string `json:"prompt_cache_retention,omitempty"`
}

// responseFormatWire is the OpenAI chat-completions response_format object.
type responseFormatWire struct {
	Type       string `json:"type"`
	JSONSchema *struct {
		Name        string          `json:"name,omitempty"`
		Description string          `json:"description,omitempty"`
		Schema      json.RawMessage `json:"schema,omitempty"`
		Strict      *bool           `json:"strict,omitempty"`
	} `json:"json_schema,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"`
	Reasoning  json.RawMessage `json:"reasoning_content,omitempty"`
	ToolCalls  []toolCall      `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function functionCall `json:"function"`
}

type functionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatTool struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type toolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type contentPart struct {
	Type       string            `json:"type"`
	Text       string            `json:"text,omitempty"`
	ImageURL   *imageURLObject   `json:"image_url,omitempty"`
	InputAudio *inputAudioObject `json:"input_audio,omitempty"`
	File       *fileObject       `json:"file,omitempty"`
}

type imageURLObject struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type inputAudioObject struct {
	Data   string `json:"data"`
	Format string `json:"format"`
}

type fileObject struct {
	FileID   string `json:"file_id,omitempty"`
	Filename string `json:"filename,omitempty"`
	FileData string `json:"file_data,omitempty"`
}

// --- response wire types ---

type chatResponse struct {
	ID                string       `json:"id"`
	Model             string       `json:"model"`
	Choices           []chatChoice `json:"choices"`
	Usage             *usage       `json:"usage"`
	SystemFingerprint string       `json:"system_fingerprint,omitempty"`
	ServiceTier       string       `json:"service_tier,omitempty"`
}

type chatChoice struct {
	Message      respMessage `json:"message"`
	Delta        *respDelta  `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

type respMessage struct {
	Role      string          `json:"role"`
	Content   *string         `json:"content"`
	// Reasoning is DeepSeek/Kimi/Z.AI-style reasoning_content (string or JSON string).
	Reasoning json.RawMessage `json:"reasoning_content,omitempty"`
	ToolCalls []respToolCall  `json:"tool_calls"`
}

type respDelta struct {
	Role      string          `json:"role"`
	Content   *string         `json:"content"`
	// Reasoning carries streaming reasoning_content deltas (string JSON value).
	Reasoning json.RawMessage `json:"reasoning_content,omitempty"`
	ToolCalls []respToolCall  `json:"tool_calls"`
}

type respToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	// TotalTokens is present on some OpenAI responses; unused for parse totals.
	TotalTokens int `json:"total_tokens,omitempty"`

	PromptTokensDetails     *promptTokensDetails     `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *completionTokensDetails `json:"completion_tokens_details,omitempty"`
}

type promptTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
}

type completionTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
}

// chatRequest optional OpenAI-only fields used on egress rebuild.
// (service_tier lives on the request wire in ingress; egress build adds it.)
