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
	BlockDocument   BlockType = "document"
	BlockToolUse    BlockType = "tool_use"
	BlockToolResult BlockType = "tool_result"
	BlockThinking   BlockType = "thinking"
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
}

// DocumentSource holds an inline or referenced non-image document (primarily PDF).
// Kind is "base64", "url", or "file" (file_id / file_uri style references).
type DocumentSource struct {
	Kind      string
	MediaType string // e.g. application/pdf
	Data      string // base64 payload, URL, or file id/uri
	Title     string // optional filename / title
}

// Block is one unit of message content. Only the fields relevant to Type are
// populated; the tagged-struct shape avoids an interface for trivial JSON.
type Block struct {
	Type BlockType

	Text      string // text, thinking (or redacted_thinking opaque data when Redacted)
	Signature string // thinking

	Image    *ImageSource    // image
	Document *DocumentSource // document / PDF

	ID    string          // tool_use id
	Name  string          // tool_use name
	Input json.RawMessage // tool_use arguments (JSON object)

	ToolUseID    string  // tool_result: the tool_use id it answers
	Result       string  // tool_result: plain text (backward compatible)
	ResultBlocks []Block // tool_result: multimodal content (text/image); not nested tool_use/tool_result
	IsError      bool    // tool_result

	// Redacted marks Anthropic redacted_thinking blocks. Text holds the opaque
	// data payload; do not attempt to decrypt or display it.
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

// ResponseFormatKind selects structured-output mode on a chat request.
type ResponseFormatKind string

const (
	ResponseFormatText       ResponseFormatKind = "text"
	ResponseFormatJSONObject ResponseFormatKind = "json_object"
	ResponseFormatJSONSchema ResponseFormatKind = "json_schema"
)

// ResponseFormat is a dialect-neutral structured-output / JSON-mode config.
// Nil / zero means the client did not request structured outputs.
type ResponseFormat struct {
	Kind        ResponseFormatKind
	Name        string          // optional schema name (OpenAI json_schema.name)
	Description string          // optional schema description
	Schema      json.RawMessage // JSON Schema document when Kind is json_schema
	Strict      *bool           // optional strictness flag when present on wire
}

// ThinkingMode controls request-side reasoning/thinking enablement.
type ThinkingMode string

const (
	ThinkingEnabled  ThinkingMode = "enabled"
	ThinkingDisabled ThinkingMode = "disabled"
	ThinkingAdaptive ThinkingMode = "adaptive"
)

// ThinkingConfig is request-side reasoning/thinking configuration.
//
// Effort ↔ budget is best-effort, not bit-identical across providers:
//
//	effort "minimal"/"low" ≈ 1024 tokens
//	effort "medium"        ≈ 8192 tokens
//	effort "high"          ≈ 16384 tokens
//
// Nil means the client did not request thinking controls (do not invent defaults).
type ThinkingConfig struct {
	// Mode: enabled, disabled, adaptive. Empty with BudgetTokens set implies enabled.
	Mode ThinkingMode
	// Effort is OpenAI-style: minimal|low|medium|high (best-effort ↔ BudgetTokens).
	Effort string
	// BudgetTokens is Anthropic/Google-style thinking token budget.
	BudgetTokens *int
	// IncludeThoughts asks the provider to return thinking content when supported.
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
	TopK          *int // Anthropic/Google; OpenAI egress omits
	StopSequences []string
	Tools         []Tool
	ToolChoice    *ToolChoice
	Metadata      map[string]string

	// ParallelToolCalls prefers parallel tool use when true, sequential when
	// false. Nil means unset (provider default; omit on wire).
	// OpenAI: parallel_tool_calls. Anthropic: disable_parallel_tool_use (inverted).
	ParallelToolCalls *bool

	// ResponseFormat carries structured-output / JSON schema constraints.
	ResponseFormat *ResponseFormat

	// Thinking is request-side reasoning config (effort, budget, adaptive).
	// Distinct from BlockThinking content blocks in Messages / Response.
	Thinking *ThinkingConfig

	// FrequencyPenalty / PresencePenalty are OpenAI-style sampling penalties.
	// Anthropic Messages has no equivalent — egress omits without error.
	FrequencyPenalty *float64
	PresencePenalty  *float64

	// Seed is a deterministic sampling hint (OpenAI / Google). Field-level
	// fidelity only — does not guarantee bit-identical outputs across providers.
	// Anthropic Messages has no equivalent — egress omits without error.
	Seed *int64

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

	// Redacted is set on EventBlockStart for Anthropic redacted_thinking blocks.
	// Text may carry the opaque data payload on that start event (no deltas).
	Redacted bool

	Text        string // EventTextDelta / EventThinkingDelta / redacted data on start
	PartialJSON string // EventJSONDelta

	StopReason string // EventFinish
	Usage      Usage  // EventFinish
}
