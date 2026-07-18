// Package google implements the Gemini native generateContent provider (egress):
// building a Google request from canonical form and parsing responses/streams
// back into canonical form.
package google

import "encoding/json"

// --- request wire types ---

type generateRequest struct {
	Contents          []content         `json:"contents"`
	SystemInstruction *content          `json:"system_instruction,omitempty"`
	Tools             []tool            `json:"tools,omitempty"`
	ToolConfig        *toolConfig       `json:"tool_config,omitempty"`
	GenerationConfig  *generationConfig `json:"generation_config,omitempty"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type part struct {
	Text             string            `json:"text,omitempty"`
	InlineData       *blob             `json:"inline_data,omitempty"`
	FunctionCall     *functionCall     `json:"function_call,omitempty"`
	FunctionResponse *functionResponse `json:"function_response,omitempty"`
	Thought          bool              `json:"thought,omitempty"`
}

type blob struct {
	MIMEType string `json:"mime_type"`
	Data     string `json:"data"`
}

type functionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

type functionResponse struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

type tool struct {
	FunctionDeclarations []functionDeclaration `json:"function_declarations,omitempty"`
}

type functionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type toolConfig struct {
	FunctionCallingConfig *functionCallingConfig `json:"function_calling_config,omitempty"`
}

type functionCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"`
	AllowedFunctionNames []string `json:"allowed_function_names,omitempty"`
}

type generationConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"top_p,omitempty"`
	MaxOutputTokens int      `json:"max_output_tokens,omitempty"`
	StopSequences   []string `json:"stop_sequences,omitempty"`
}

// --- response wire types ---

type generateResponse struct {
	Candidates    []candidate    `json:"candidates"`
	UsageMetadata *usageMetadata `json:"usageMetadata"` // camelCase also common
	// snake_case variants appear in some responses / docs
	UsageMetadataSnake *usageMetadata `json:"usage_metadata"`
	ModelVersion       string         `json:"modelVersion"`
	ModelVersionSnake  string         `json:"model_version"`
	ResponseID         string         `json:"responseId"`
	ResponseIDSnake    string         `json:"response_id"`
}

type candidate struct {
	Content      *content `json:"content"`
	FinishReason string   `json:"finishReason"`
	FinishSnake  string   `json:"finish_reason"`
	Index        int      `json:"index"`
}

type usageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
	// snake_case
	PromptTokensSnake     int `json:"prompt_token_count"`
	CandidatesTokensSnake int `json:"candidates_token_count"`
	TotalTokensSnake      int `json:"total_token_count"`
}

func (u *usageMetadata) prompt() int {
	if u == nil {
		return 0
	}
	if u.PromptTokenCount != 0 {
		return u.PromptTokenCount
	}
	return u.PromptTokensSnake
}

func (u *usageMetadata) candidates() int {
	if u == nil {
		return 0
	}
	if u.CandidatesTokenCount != 0 {
		return u.CandidatesTokenCount
	}
	return u.CandidatesTokensSnake
}

func (r generateResponse) usage() *usageMetadata {
	if r.UsageMetadata != nil {
		return r.UsageMetadata
	}
	return r.UsageMetadataSnake
}

func (r generateResponse) model() string {
	if r.ModelVersion != "" {
		return r.ModelVersion
	}
	return r.ModelVersionSnake
}

func (r generateResponse) id() string {
	if r.ResponseID != "" {
		return r.ResponseID
	}
	return r.ResponseIDSnake
}

func (c candidate) finish() string {
	if c.FinishReason != "" {
		return c.FinishReason
	}
	return c.FinishSnake
}

// Path builds the relative path for a native Gemini call.
// base_url should be e.g. https://generativelanguage.googleapis.com/v1beta
func Path(model string, stream bool) string {
	if stream {
		return "/models/" + model + ":streamGenerateContent?alt=sse"
	}
	return "/models/" + model + ":generateContent"
}
