package canonical

// Realtime IR (intermediate representation) is reserved for a future optional
// OpenAI Realtime ↔ Google Live protocol bridge.
//
// Status (M3 scope decision):
//   - Same-protocol passthrough is implemented: OpenAI Realtime → openai /
//     openai_compat, and Google Live → kind:google.
//   - Cross-protocol bridge is NOT implemented and is deferred to a future
//     milestone. Attempts to use a cross-protocol realtime path fail closed
//     with gateway error code "unsupported_realtime_bridge".
//   - No Anthropic WebSocket dialect will be invented.
//
// Types below are placeholders so the package layout matches the design
// (canonical/realtime.go) without shipping a half-built bridge.

// RealtimeSessionConfig is a dialect-neutral session open / update config.
// Fields are intentionally minimal until the bridge ships.
type RealtimeSessionConfig struct {
	// Model is the public or upstream model id for the session.
	Model string
	// Voice is an optional voice identifier when the protocol supports it.
	Voice string
	// Modalities lists accepted modalities (e.g. "text", "audio").
	Modalities []string
	// Extra holds vendor-specific keys with no canonical mapping yet.
	Extra map[string]any
}

// RealtimeEventType discriminates a RealtimeEvent.
type RealtimeEventType string

const (
	// RealtimeEventSessionUpdate is a session configuration update.
	RealtimeEventSessionUpdate RealtimeEventType = "session.update"
	// RealtimeEventInputAudio is a chunk of client audio input.
	RealtimeEventInputAudio RealtimeEventType = "input_audio"
	// RealtimeEventOutputAudio is a chunk of model audio output.
	RealtimeEventOutputAudio RealtimeEventType = "output_audio"
	// RealtimeEventTextDelta is a text token/delta from the model.
	RealtimeEventTextDelta RealtimeEventType = "text.delta"
	// RealtimeEventToolCall is a model tool/function call in a live session.
	RealtimeEventToolCall RealtimeEventType = "tool.call"
	// RealtimeEventToolResult is a client tool result in a live session.
	RealtimeEventToolResult RealtimeEventType = "tool.result"
	// RealtimeEventTranscript is a speech-to-text transcript delta.
	RealtimeEventTranscript RealtimeEventType = "transcript.delta"
	// RealtimeEventError is a protocol or bridge error.
	RealtimeEventError RealtimeEventType = "error"
	// RealtimeEventClose marks an intentional session end.
	RealtimeEventClose RealtimeEventType = "close"
)

// RealtimeEvent is a dialect-neutral live-session event for a future bridge.
// Until the bridge exists, proxy uses raw frame passthrough only.
type RealtimeEvent struct {
	Type RealtimeEventType
	// Text is set for text deltas and human-readable errors.
	Text string
	// Audio holds optional PCM (or other) bytes for audio events.
	Audio []byte
	// MediaType is the audio encoding when Audio is set (e.g. "audio/pcm").
	MediaType string
	// Session holds session config when Type is RealtimeEventSessionUpdate.
	Session *RealtimeSessionConfig
	// Extra holds unmapped vendor fields (drop-list / future mapping).
	Extra map[string]any
}

// UnsupportedRealtimeBridge is the stable gateway error code for cross-protocol
// realtime attempts. Keep in sync with proxy.CodeUnsupportedRealtimeBridge.
const UnsupportedRealtimeBridge = "unsupported_realtime_bridge"
