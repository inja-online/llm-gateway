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
	// BlockDocument is a non-image document (primarily PDF).
	BlockDocument BlockType = "document"
	// BlockAudio is chat input audio. It is input-only for the chat path
	// (not TTS/speech output as a content block; STT/TTS use separate modalities).
	BlockAudio BlockType = "audio"
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

// ResponseFormat kind values (canonical names; not provider wire keys).
const (
	ResponseFormatText       = "text"
	ResponseFormatJSONObject = "json_object"
	ResponseFormatJSONSchema = "json_schema"
)

// MaxTokensField records which OpenAI-style max field the client used so
// egress can re-emit the same knob. Empty means unset / not applicable.
const (
	MaxTokensFieldMaxTokens           = "max_tokens"
	MaxTokensFieldMaxCompletionTokens = "max_completion_tokens"
)

// ImageSource holds an inline or referenced image.
type ImageSource struct {
	// Kind is "base64" or "url".
	Kind      string
	MediaType string // for base64
	Data      string // base64 payload or URL
	// Detail is OpenAI image_url.detail: "auto" | "low" | "high".
	// Empty means unset — do not default to "auto" on the wire.
	Detail string
}

// DocumentSource holds an inline or referenced non-image document (e.g. PDF).
// Kind is "base64" | "url" | "file_uri".
type DocumentSource struct {
	Kind      string
	MediaType string // e.g. application/pdf
	Data      string // base64 payload, URL, or file URI
	Filename  string // optional
	Title     string // optional
}

// AudioSource holds chat input audio.
// Kind is "base64" | "url". Input-only for chat content blocks.
type AudioSource struct {
	Kind      string
	MediaType string
	Data      string // base64 payload or URL
	// Transcript is an optional client-supplied transcript of the audio.
	Transcript string
}

// Block is one unit of message content. Only the fields relevant to Type are
// populated; the tagged-struct shape avoids an interface for trivial JSON.
type Block struct {
	Type BlockType

	Text      string // text, thinking
	Signature string // thinking

	Image    *ImageSource    // image
	Document *DocumentSource // document
	// Audio is chat input audio (BlockAudio). Input-only; not used for TTS output.
	Audio *AudioSource

	ID    string          // tool_use id
	Name  string          // tool_use name
	Input json.RawMessage // tool_use arguments (JSON object)

	ToolUseID    string  // tool_result: the tool_use id it answers
	Result       string  // tool_result: plain text (backward compatible)
	ResultBlocks []Block // tool_result: multimodal content (text/image); not nested tool_use/tool_result
	IsError      bool    // tool_result

	// Redacted marks Anthropic redacted_thinking blocks. When true, Text may
	// hold an opaque payload that must be preserved for multi-turn Claude
	// thinking continuity; do not attempt to decrypt or display it.
	// (Issue #48: redacted_thinking support.)
	Redacted bool
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

// ResponseFormat is structured-output / JSON-mode configuration.
// Nil on Request means no structured output was requested.
type ResponseFormat struct {
	// Kind is text | json_object | json_schema.
	Kind string
	// Name is an optional schema name (OpenAI json_schema.name).
	Name string
	// Description is an optional schema description.
	Description string
	// Schema is the JSON Schema document when Kind is json_schema.
	Schema json.RawMessage
	// Strict, when non-nil, requests strict schema adherence where supported.
	Strict *bool
}

// ThinkingConfig is request-side reasoning/thinking controls.
//
// Nil on Request means the client did not request thinking controls (do not
// invent budgets or effort). Mapping between Effort and BudgetTokens across
// dialects is best-effort, not bit-identical.
type ThinkingConfig struct {
	// Type is the thinking mode when known: "enabled", "disabled", "adaptive",
	// or empty when only other fields convey the request.
	Type string
	// Effort is a provider effort string (e.g. "minimal", "low", "medium", "high").
	// Effort ↔ BudgetTokens mapping is best-effort across dialects.
	Effort string
	// BudgetTokens is a numeric thinking/reasoning token budget when set.
	BudgetTokens *int
	// IncludeThoughts, when non-nil, controls whether thinking content is
	// returned to the client (Google includeThoughts / display preference).
	IncludeThoughts *bool
}

// Request is a canonical chat request.
type Request struct {
	Model         string
	System        []Block // text blocks
	Messages      []Message
	MaxTokens     int
	Stream        bool
	Temperature   *float64
	TopP          *float64
	// TopK is optional top-k sampling (Anthropic/Google). Nil = unset.
	// OpenAI chat-completions generally omits this on egress.
	TopK *int
	// FrequencyPenalty and PresencePenalty are OpenAI-style sampling penalties.
	// Nil = unset. Anthropic Messages generally drops them on egress.
	FrequencyPenalty *float64
	PresencePenalty  *float64
	// Seed is an optional deterministic-sampling seed (field-level fidelity;
	// does not guarantee bit-identical outputs across providers).
	Seed *int64
	StopSequences []string
	Tools         []Tool
	ToolChoice    *ToolChoice
	// ParallelToolCalls, when non-nil, prefers (true) or disables (false)
	// parallel tool calls. Unset means omit (provider default).
	// OpenAI: parallel_tool_calls; Anthropic: disable_parallel_tool_use with
	// inverted polarity (parallel=false ↔ disable=true).
	ParallelToolCalls *bool
	Metadata          map[string]string

	// ResponseFormat is optional structured-output / JSON-mode config.
	// Nil means unset (current behavior: free-form text).
	ResponseFormat *ResponseFormat

	// Thinking is optional request-side reasoning/thinking configuration.
	// Nil means the client did not request thinking controls.
	Thinking *ThinkingConfig

	// MaxTokensField records which wire field supplied MaxTokens on OpenAI-style
	// ingress: "" | "max_tokens" | "max_completion_tokens". MaxTokens remains
	// the numeric budget; egress uses this to re-emit the correct field name.
	// Anthropic/Google always map the numeric budget to their native field.
	MaxTokensField string

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
