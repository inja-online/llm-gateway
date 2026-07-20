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
	// SafetySettings is re-emitted when present on canonical (Google ingress).
	SafetySettings json.RawMessage `json:"safety_settings,omitempty"`
	// CachedContent is a context-cache resource name (#108).
	CachedContent string `json:"cachedContent,omitempty"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type part struct {
	Text             string            `json:"text,omitempty"`
	InlineData       *blob             `json:"inline_data,omitempty"`
	FileData         *fileData         `json:"file_data,omitempty"`
	FunctionCall     *functionCall     `json:"function_call,omitempty"`
	FunctionResponse *functionResponse `json:"function_response,omitempty"`
	Thought          bool              `json:"thought,omitempty"`
}

type blob struct {
	MIMEType string `json:"mime_type"`
	Data     string `json:"data"`
}

// fileData is Gemini's file_data part (Files API URI or remote http(s) URL).
// Pass-through avoids server-side fetch/SSRF; Gemini resolves the URI.
type fileData struct {
	MIMEType string `json:"mime_type,omitempty"`
	FileURI  string `json:"file_uri"`
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
	// TopK sampling (#37).
	TopK *int `json:"top_k,omitempty"`
	// Seed for deterministic sampling (#39).
	Seed *int64 `json:"seed,omitempty"`
	// Structured outputs (#28).
	ResponseMIMEType string          `json:"response_mime_type,omitempty"`
	ResponseSchema   json.RawMessage `json:"response_schema,omitempty"`
	// Thinking (#30): thinking_config.thinking_budget / include_thoughts.
	ThinkingConfig *thinkingConfigWire `json:"thinking_config,omitempty"`
}

// thinkingConfigWire is generation_config.thinking_config on Gemini.
type thinkingConfigWire struct {
	IncludeThoughts *bool `json:"include_thoughts,omitempty"`
	ThinkingBudget  *int  `json:"thinking_budget,omitempty"`
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
	// Cached / thoughts when present on newer Gemini responses.
	CachedContentTokenCount int `json:"cachedContentTokenCount"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
	// snake_case
	PromptTokensSnake       int `json:"prompt_token_count"`
	CandidatesTokensSnake   int `json:"candidates_token_count"`
	TotalTokensSnake        int `json:"total_token_count"`
	CachedContentTokenSnake int `json:"cached_content_token_count"`
	ThoughtsTokenSnake      int `json:"thoughts_token_count"`
}

func (u *usageMetadata) cached() int {
	if u == nil {
		return 0
	}
	if u.CachedContentTokenCount != 0 {
		return u.CachedContentTokenCount
	}
	return u.CachedContentTokenSnake
}

func (u *usageMetadata) thoughts() int {
	if u == nil {
		return 0
	}
	if u.ThoughtsTokenCount != 0 {
		return u.ThoughtsTokenCount
	}
	return u.ThoughtsTokenSnake
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

// CountTokensPath builds the relative path for Gemini countTokens.
func CountTokensPath(model string) string {
	return "/models/" + model + ":countTokens"
}

// ModelsPath is the relative path for listing models (GET).
func ModelsPath() string { return "/models" }

// ModelPath is the relative path for getting one model (GET).
func ModelPath(model string) string { return "/models/" + model }
