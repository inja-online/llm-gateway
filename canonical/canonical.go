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

// Block is one unit of message content. Only the fields relevant to Type are
// populated; the tagged-struct shape avoids an interface for trivial JSON.
type Block struct {
	Type BlockType

	Text      string // text, thinking
	Signature string // thinking

	Image *ImageSource // image

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

// Request is a canonical chat request.
type Request struct {
	Model         string
	System        []Block // text blocks
	Messages      []Message
	MaxTokens     int
	Stream        bool
	Temperature   *float64
	TopP          *float64
	StopSequences []string
	Tools         []Tool
	ToolChoice    *ToolChoice
	Metadata      map[string]string

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
