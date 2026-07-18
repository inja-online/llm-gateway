// Package openai implements the OpenAI chat-completions provider (egress):
// building an OpenAI request from canonical form and parsing OpenAI responses
// and streams back into canonical form.
package openai

import "encoding/json"

// --- request wire types ---

type chatRequest struct {
	Model       string          `json:"model"`
	Messages    []chatMessage   `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	StreamOpts  *streamOptions  `json:"stream_options,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	TopP        *float64        `json:"top_p,omitempty"`
	Stop        []string        `json:"stop,omitempty"`
	Tools       []chatTool      `json:"tools,omitempty"`
	ToolChoice  json.RawMessage `json:"tool_choice,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"`
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
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *imageURLObject `json:"image_url,omitempty"`
}

type imageURLObject struct {
	URL string `json:"url"`
}

// --- response wire types ---

type chatResponse struct {
	ID      string       `json:"id"`
	Model   string       `json:"model"`
	Choices []chatChoice `json:"choices"`
	Usage   *usage       `json:"usage"`
}

type chatChoice struct {
	Message      respMessage `json:"message"`
	Delta        *respDelta  `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

type respMessage struct {
	Role      string         `json:"role"`
	Content   *string        `json:"content"`
	ToolCalls []respToolCall `json:"tool_calls"`
}

type respDelta struct {
	Role      string         `json:"role"`
	Content   *string        `json:"content"`
	ToolCalls []respToolCall `json:"tool_calls"`
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
}
