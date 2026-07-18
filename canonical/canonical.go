// Package canonical is the gateway's provider- and dialect-neutral
// representation of a chat request, response, and stream. It is Anthropic-
// shaped (content blocks, top-level system, tool_use/tool_result blocks)
// because that is the structural superset: OpenAI's flat messages map into
// blocks losslessly, while the reverse is lossy. Translation only happens on
// cross-dialect requests; same-dialect traffic uses the passthrough path and
// never touches these types.
package canonical

import "encoding/json"

// BlockType discriminates a content Block.
type BlockType string

const (
	BlockText       BlockType = "text"
	BlockImage      BlockType = "image"
	BlockToolUse    BlockType = "tool_use"
	BlockToolResult BlockType = "tool_result"
	BlockThinking   BlockType = "thinking"
	// BlockAudio is chat input audio only (not TTS output as a content block).
	BlockAudio BlockType = "audio"
	// BlockDocument is a non-image document (PDF, file parts, etc.).
	BlockDocument BlockType = "document"
)

// Role values for a Message.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Stop reasons (Anthropic vocabulary; the canonical set).
const (
	StopEndTurn      = "end_turn"
	StopMaxTokens    = "max_tokens"
	StopToolUse      = "tool_use"
	StopStopSequence = "stop_sequence"
	StopRefusal      = "refusal"
)

// ImageSource holds an inline or referenced image.
type ImageSource struct {
	// Kind is "base64" or "url".
	Kind      string
	MediaType string // for base64
	Data      string // base64 payload or URL
	// Detail is OpenAI image_url.detail: "auto" | "low" | "high". Empty means unset
	// (do not invent "auto" on the wire).
	Detail string
}

// AudioSource holds chat input audio (BlockAudio). Kind is "base64" or "url".
// Format is the OpenAI input_audio.format when known (wav, mp3, …).
type AudioSource struct {
	Kind       string // base64 | url
	MediaType  string
	Data       string // base64 payload or URL
	Format     string // OpenAI format string (wav, mp3, …)
	Transcript string // optional client-supplied transcript
}

// DocumentSource holds a non-image document (BlockDocument).
// Kind is "base64", "url", or "file_id".
type DocumentSource struct {
	Kind      string // base64 | url | file_id
	MediaType string
	Data      string // base64 payload, URL, or file_id
	Filename  string
	Title     string
}

// Block is one unit of message content. Only the fields relevant to Type are
// populated; the tagged-struct shape avoids an interface for trivial JSON.
type Block struct {
	Type BlockType

	Text      string // text, thinking
	Signature string // thinking

	Image    *ImageSource    // image
	Audio    *AudioSource    // audio (input)
	Document *DocumentSource // document / file

	ID    string          // tool_use id
	Name  string          // tool_use name
	Input json.RawMessage // tool_use arguments (JSON object)

	ToolUseID    string  // tool_result: the tool_use id it answers
	Result       string  // tool_result: plain text (backward compatible)
	ResultBlocks []Block // tool_result: multimodal content (text/image); not nested tool_use/tool_result
	IsError      bool    // tool_result

	Redacted bool // thinking: redacted_thinking (Anthropic)
}

// Message is one turn.
type Message struct {
	Role    string
	Content []Block
}

// Tool is a function definition offered to the model.
type Tool struct {
	Name        string
	Description string
	Schema      json.RawMessage // JSON Schema for the arguments
}

// ToolChoiceMode controls tool selection.
type ToolChoiceMode string

const (
	ToolAuto     ToolChoiceMode = "auto"
	ToolNone     ToolChoiceMode = "none"
	ToolRequired ToolChoiceMode = "required" // must call some tool ("any" in Anthropic)
	ToolSpecific ToolChoiceMode = "tool"     // must call Name
)

type ToolChoice struct {
	Mode ToolChoiceMode
	Name string // for ToolSpecific
}

// ResponseFormatKind is a dialect-neutral structured-output mode.
// Names are internal; wire dialects map their own field names.
type ResponseFormatKind string

const (
	ResponseFormatText       ResponseFormatKind = "text"
	ResponseFormatJSONObject ResponseFormatKind = "json_object"
	ResponseFormatJSONSchema ResponseFormatKind = "json_schema"
)

// ResponseFormat carries structured-output / JSON-mode configuration.
// Nil on Request means the client did not request structured output.
type ResponseFormat struct {
	Kind        ResponseFormatKind
	Name        string          // schema name when Kind is json_schema
	Description string          // optional schema description
	Schema      json.RawMessage // JSON Schema body (raw; not validated here)
	Strict      *bool           // OpenAI strict flag when present
}

// ThinkingMode describes whether thinking/reasoning controls are on.
type ThinkingMode string

const (
	ThinkingEnabled  ThinkingMode = "enabled"
	ThinkingDisabled ThinkingMode = "disabled"
	ThinkingAdaptive ThinkingMode = "adaptive"
)

// ThinkingConfig is request-side reasoning/thinking configuration (not content).
// Effort ↔ budget mapping across dialects is best-effort, not bit-identical.
// Nil means the client did not request thinking controls (do not invent budgets).
type ThinkingConfig struct {
	Mode            ThinkingMode // enabled / disabled / adaptive; empty when only Effort/Budget set
	Effort          string       // OpenAI-style: minimal|low|medium|high|…
	BudgetTokens    *int         // Anthropic/Google-style token budget
	IncludeThoughts *bool        // whether thoughts should be returned to the client
}

// MaxTokens field source markers for OpenAI wire fidelity.
// Empty means unspecified / default max_tokens emission on OpenAI egress.
const (
	MaxTokensSourceTokens           = "max_tokens"
	MaxTokensSourceCompletionTokens = "max_completion_tokens"
)

// Request is a canonical chat request.
type Request struct {
	Model     string
	System    []Block // text blocks
	Messages  []Message
	MaxTokens int
	// MaxTokensField records which OpenAI wire field supplied MaxTokens
	// (MaxTokensSourceTokens or MaxTokensSourceCompletionTokens). Empty when
	// unset or non-OpenAI ingress; OpenAI egress uses it to re-emit the correct
	// field name. Other dialects always map MaxTokens to their numeric budget.
	MaxTokensField string
	Stream         bool
	Temperature    *float64
	TopP           *float64
	StopSequences  []string
	Tools          []Tool
	ToolChoice     *ToolChoice
	// ParallelToolCalls is OpenAI parallel_tool_calls / inverse of Anthropic
	// disable_parallel_tool_use. Nil means omit (provider default).
	ParallelToolCalls *bool
	Metadata          map[string]string

	// FrequencyPenalty / PresencePenalty are OpenAI-style sampling penalties.
	// Nil means unset. Anthropic egress omits them.
	FrequencyPenalty *float64
	PresencePenalty  *float64

	// Seed is a sampling seed for field-level fidelity (not cross-provider
	// reproducibility). Nil means unset.
	Seed *int64

	// ResponseFormat is structured-output config. Nil = not requested.
	ResponseFormat *ResponseFormat

	// Thinking is request-side reasoning/thinking config. Nil = not requested.
	Thinking *ThinkingConfig

	// ServiceTier is an optional OpenAI request hint (e.g. "auto", "default").
	// Other dialects omit it on egress.
	ServiceTier string

	// SafetySettings is raw Google Gemini safetySettings JSON when present on
	// Google ingress. Re-emitted only on Google egress; other dialects drop it.
	// Passthrough Google→Google does not use this field (byte path).
	SafetySettings json.RawMessage

	// N is the requested multi-choice count (OpenAI n / Google candidateCount).
	// Translation supports only n=1; see policy in ingress parsers.
	// 0 means unset (treat as 1). Values >1 are rejected at ingress.
	N int
}

// Usage is token accounting from the upstream.
//
// Detail fields are optional and zero when unknown. totals (InputTokens /
// OutputTokens) keep provider-reported totals; reasoning tokens are not
// double-counted into OutputTokens beyond what the upstream already included.
type Usage struct {
	InputTokens  int
	OutputTokens int
	HasUsage     bool // false when the upstream reported nothing

	// CacheReadTokens is prompt tokens served from cache
	// (OpenAI prompt_tokens_details.cached_tokens, Anthropic cache_read_input_tokens).
	CacheReadTokens int
	// CacheWriteTokens is tokens written into cache
	// (Anthropic cache_creation_input_tokens). No standard OpenAI analogue.
	CacheWriteTokens int
	// ReasoningTokens is completion-side reasoning/thinking tokens
	// (OpenAI completion_tokens_details.reasoning_tokens). Included in
	// OutputTokens when the provider already folded them into completion totals.
	ReasoningTokens int
}

// Response is a completed canonical response.
type Response struct {
	ID         string
	Model      string // the model that actually served
	Content    []Block
	StopReason string
	Usage      Usage

	// SystemFingerprint and ServiceTier are optional OpenAI response metadata.
	// Never invented for non-OpenAI upstreams.
	SystemFingerprint string
	ServiceTier       string
}

// EventType discriminates a StreamEvent.
type EventType int

const (
	EventStart         EventType = iota // message start (ID, Model)
	EventBlockStart                     // a content block begins (Index, BlockType, tool id/name)
	EventTextDelta                      // Index, Text
	EventJSONDelta                      // Index, PartialJSON (tool_use argument stream)
	EventThinkingDelta                  // Index, Text
	EventBlockStop                      // Index
	EventFinish                         // StopReason, Usage
)

// StreamEvent is one canonical streaming event. It models Anthropic's block-
// structured stream (richer than OpenAI's flat deltas); the OpenAI serializer
// flattens it back down.
type StreamEvent struct {
	Type EventType

	ID    string // EventStart
	Model string // EventStart

	Index     int       // block index (EventBlockStart/Delta/Stop)
	BlockType BlockType // EventBlockStart
	ToolID    string    // EventBlockStart (tool_use)
	ToolName  string    // EventBlockStart (tool_use)

	Text        string // EventTextDelta / EventThinkingDelta
	PartialJSON string // EventJSONDelta

	StopReason string // EventFinish
	Usage      Usage  // EventFinish
}
