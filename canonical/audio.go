package canonical

// BlockAudio is chat input audio. It is input-only for the chat path
// (not TTS/speech output as a content block; STT/TTS use separate modalities).
const BlockAudio BlockType = "audio"

// AudioSource holds chat input audio.
// Kind is "base64" | "url". Input-only for chat content blocks.
type AudioSource struct {
	Kind      string
	MediaType string
	Data      string // base64 payload or URL
	// Transcript is an optional client-supplied transcript of the audio.
	Transcript string
}
