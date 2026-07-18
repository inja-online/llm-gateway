// Package openai implements the OpenAI chat-completions dialect: parsing
// requests into canonical form and serializing canonical responses/streams
// back to the OpenAI wire format.
package openai

import "encoding/json"

// --- request wire types ---

type chatRequest struct {
	Model         string          `json:"model"`
	Messages      []chatMessage   `json:"messages"`
	MaxTokens     *int            `json:"max_tokens,omitempty"`
	MaxCompletion *int            `json:"max_completion_tokens,omitempty"`
	Stream        bool            `json:"stream,omitempty"`
	Temperature   *float64        `json:"temperature,omitempty"`
	TopP          *float64        `json:"top_p,omitempty"`
	Stop          json.RawMessage `json:"stop,omitempty"` // string or []string
	Tools         []chatTool      `json:"tools,omitempty"`
	ToolChoice    json.RawMessage `json:"tool_choice,omitempty"` // string or object
	// N is multi-choice count. Translation only supports n=1; n>1 is rejected.
	N *int `json:"n,omitempty"`
	// ServiceTier is optional OpenAI routing/priority hint.
	ServiceTier string `json:"service_tier,omitempty"`
}

type chatMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"` // string, array, or null
	Name       string          `json:"name,omitempty"`
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
	Arguments string `json:"arguments"` // JSON-encoded string
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

// contentPart is one element of a multimodal content array.
type contentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *imageURLObject `json:"image_url,omitempty"`
}

type imageURLObject struct {
	URL string `json:"url"`
}

// --- response wire types ---

type chatResponse struct {
	ID                string       `json:"id"`
	Object            string       `json:"object"`
	Created           int64        `json:"created"`
	Model             string       `json:"model"`
	Choices           []chatChoice `json:"choices"`
	Usage             *usage       `json:"usage,omitempty"`
	SystemFingerprint string       `json:"system_fingerprint,omitempty"`
	ServiceTier       string       `json:"service_tier,omitempty"`
}

type chatChoice struct {
	Index        int         `json:"index"`
	Message      chatOutMsg  `json:"message,omitempty"`
	Delta        *chatOutMsg `json:"delta,omitempty"`
	FinishReason *string     `json:"finish_reason"`
}

type chatOutMsg struct {
	Role      string          `json:"role,omitempty"`
	Content   *string         `json:"content,omitempty"`
	ToolCalls []outToolCall   `json:"tool_calls,omitempty"`
	Reasoning json.RawMessage `json:"reasoning_content,omitempty"`
}

type outToolCall struct {
	Index    int              `json:"index,omitempty"`
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"`
	Function outFunctionDelta `json:"function"`
}

type outFunctionDelta struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type usage struct {
	PromptTokens            int                      `json:"prompt_tokens"`
	CompletionTokens        int                      `json:"completion_tokens"`
	TotalTokens             int                      `json:"total_tokens"`
	PromptTokensDetails     *promptTokensDetails     `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *completionTokensDetails `json:"completion_tokens_details,omitempty"`
}

type promptTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
}

type completionTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
}
