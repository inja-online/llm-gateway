// Package google implements the Gemini native generateContent dialect:
// parsing requests into canonical form and serializing responses/streams
// back to Google's wire format.
package google

import "encoding/json"

// --- request wire types ---

type generateRequest struct {
	// Model is optional on the wire (native API puts it in the URL path).
	// The gateway accepts it in the body for routing convenience.
	Model             string            `json:"model,omitempty"`
	Contents          []content         `json:"contents"`
	SystemInstruction *content          `json:"system_instruction,omitempty"`
	Tools             []tool            `json:"tools,omitempty"`
	ToolConfig        *toolConfig       `json:"tool_config,omitempty"`
	GenerationConfig  *generationConfig `json:"generation_config,omitempty"`
	// camelCase variant used by Google REST samples / client libraries.
	GenerationConfigCamel *generationConfig `json:"generationConfig,omitempty"`
	// SafetySettings is Gemini harm category thresholds. Preserved on Google
	// translate egress only; OpenAI/Anthropic clients have no mapping.
	SafetySettings json.RawMessage `json:"safety_settings,omitempty"`
	// Also accept camelCase as used by some clients/docs.
	SafetySettingsCamel json.RawMessage `json:"safetySettings,omitempty"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type part struct {
	Text             string            `json:"text,omitempty"`
	InlineData       *blob             `json:"inline_data,omitempty"`
	FileData         *fileData         `json:"file_data,omitempty"`
	// camelCase variants used by some Google client libraries / docs.
	InlineDataCamel  *blob             `json:"inlineData,omitempty"`
	FileDataCamel    *fileData         `json:"fileData,omitempty"`
	FunctionCall     *functionCall     `json:"function_call,omitempty"`
	FunctionResponse *functionResponse `json:"function_response,omitempty"`
	Thought          bool              `json:"thought,omitempty"`
}

type blob struct {
	MIMEType      string `json:"mime_type"`
	MIMETypeCamel string `json:"mimeType,omitempty"`
	Data          string `json:"data"`
}

// fileData is Gemini's file_data part (Files API URI or remote URL).
type fileData struct {
	MIMEType      string `json:"mime_type,omitempty"`
	MIMETypeCamel string `json:"mimeType,omitempty"`
	FileURI       string `json:"file_uri,omitempty"`
	FileURICamel  string `json:"fileUri,omitempty"`
}

func (b *blob) mime() string {
	if b == nil {
		return ""
	}
	if b.MIMEType != "" {
		return b.MIMEType
	}
	return b.MIMETypeCamel
}

func (f *fileData) mime() string {
	if f == nil {
		return ""
	}
	if f.MIMEType != "" {
		return f.MIMEType
	}
	return f.MIMETypeCamel
}

func (f *fileData) uri() string {
	if f == nil {
		return ""
	}
	if f.FileURI != "" {
		return f.FileURI
	}
	return f.FileURICamel
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
	Mode                 string   `json:"mode,omitempty"` // AUTO, ANY, NONE
	AllowedFunctionNames []string `json:"allowed_function_names,omitempty"`
}

type generationConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"top_p,omitempty"`
	TopPCamel       *float64 `json:"topP,omitempty"`
	MaxOutputTokens int      `json:"max_output_tokens,omitempty"`
	// camelCase / alternate max tokens field names.
	MaxOutputTokensCamel int      `json:"maxOutputTokens,omitempty"`
	StopSequences        []string `json:"stop_sequences,omitempty"`
	StopSequencesCamel   []string `json:"stopSequences,omitempty"`
	// TopK sampling (#37).
	TopK      *int `json:"top_k,omitempty"`
	TopKCamel *int `json:"topK,omitempty"`
	// Seed for deterministic sampling (#39).
	Seed *int64 `json:"seed,omitempty"`
	// Structured outputs (#28): responseMimeType + responseSchema / responseJsonSchema.
	ResponseMIMEType        string          `json:"response_mime_type,omitempty"`
	ResponseMIMETypeCamel   string          `json:"responseMimeType,omitempty"`
	ResponseSchema          json.RawMessage `json:"response_schema,omitempty"`
	ResponseSchemaCamel     json.RawMessage `json:"responseSchema,omitempty"`
	ResponseJSONSchema      json.RawMessage `json:"response_json_schema,omitempty"`
	ResponseJSONSchemaCamel json.RawMessage `json:"responseJsonSchema,omitempty"`
	// Thinking (#30).
	ThinkingConfig      *thinkingConfigWire `json:"thinking_config,omitempty"`
	ThinkingConfigCamel *thinkingConfigWire `json:"thinkingConfig,omitempty"`
	// CandidateCount is multi-choice. Translation only supports 1; >1 is rejected.
	CandidateCount int `json:"candidate_count,omitempty"`
	// camelCase variant
	CandidateCountCamel int `json:"candidateCount,omitempty"`
}

// thinkingConfigWire is generationConfig.thinkingConfig on Gemini.
type thinkingConfigWire struct {
	IncludeThoughts      *bool  `json:"include_thoughts,omitempty"`
	IncludeThoughtsCamel *bool  `json:"includeThoughts,omitempty"`
	ThinkingBudget       *int   `json:"thinking_budget,omitempty"`
	ThinkingBudgetCamel  *int   `json:"thinkingBudget,omitempty"`
	ThinkingLevel        string `json:"thinking_level,omitempty"`
	ThinkingLevelCamel   string `json:"thinkingLevel,omitempty"`
}

func (gc *generationConfig) topP() *float64 {
	if gc == nil {
		return nil
	}
	if gc.TopP != nil {
		return gc.TopP
	}
	return gc.TopPCamel
}

func (gc *generationConfig) maxOutputTokens() int {
	if gc == nil {
		return 0
	}
	if gc.MaxOutputTokens > 0 {
		return gc.MaxOutputTokens
	}
	return gc.MaxOutputTokensCamel
}

func (gc *generationConfig) stopSequences() []string {
	if gc == nil {
		return nil
	}
	if len(gc.StopSequences) > 0 {
		return gc.StopSequences
	}
	return gc.StopSequencesCamel
}

func (gc *generationConfig) topK() *int {
	if gc == nil {
		return nil
	}
	if gc.TopK != nil {
		return gc.TopK
	}
	return gc.TopKCamel
}

func (gc *generationConfig) responseMIMEType() string {
	if gc == nil {
		return ""
	}
	if gc.ResponseMIMEType != "" {
		return gc.ResponseMIMEType
	}
	return gc.ResponseMIMETypeCamel
}

func (gc *generationConfig) responseSchema() json.RawMessage {
	if gc == nil {
		return nil
	}
	if len(gc.ResponseSchema) > 0 {
		return gc.ResponseSchema
	}
	if len(gc.ResponseSchemaCamel) > 0 {
		return gc.ResponseSchemaCamel
	}
	if len(gc.ResponseJSONSchema) > 0 {
		return gc.ResponseJSONSchema
	}
	return gc.ResponseJSONSchemaCamel
}

func (gc *generationConfig) thinking() *thinkingConfigWire {
	if gc == nil {
		return nil
	}
	if gc.ThinkingConfig != nil {
		return gc.ThinkingConfig
	}
	return gc.ThinkingConfigCamel
}

func (tc *thinkingConfigWire) includeThoughts() *bool {
	if tc == nil {
		return nil
	}
	if tc.IncludeThoughts != nil {
		return tc.IncludeThoughts
	}
	return tc.IncludeThoughtsCamel
}

func (tc *thinkingConfigWire) thinkingBudget() *int {
	if tc == nil {
		return nil
	}
	if tc.ThinkingBudget != nil {
		return tc.ThinkingBudget
	}
	return tc.ThinkingBudgetCamel
}

func (tc *thinkingConfigWire) thinkingLevel() string {
	if tc == nil {
		return ""
	}
	if tc.ThinkingLevel != "" {
		return tc.ThinkingLevel
	}
	return tc.ThinkingLevelCamel
}

// --- response wire types ---

type generateResponse struct {
	Candidates    []candidate    `json:"candidates"`
	UsageMetadata *usageMetadata `json:"usage_metadata,omitempty"`
	ModelVersion  string         `json:"model_version,omitempty"`
	ResponseID    string         `json:"response_id,omitempty"`
}

type candidate struct {
	Content      *content `json:"content,omitempty"`
	FinishReason string   `json:"finish_reason,omitempty"`
	Index        int      `json:"index,omitempty"`
}

type usageMetadata struct {
	PromptTokenCount        int `json:"prompt_token_count"`
	CandidatesTokenCount    int `json:"candidates_token_count"`
	TotalTokenCount         int `json:"total_token_count"`
	CachedContentTokenCount int `json:"cached_content_token_count,omitempty"`
	ThoughtsTokenCount      int `json:"thoughts_token_count,omitempty"`
}

// ValidationError marks a client request problem.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }
