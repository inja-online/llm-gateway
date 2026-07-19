package canonical

// AudioSpeechRequest is the dialect-neutral text-to-speech request.
// Used on cross-family paths (OpenAI↔Google, Anthropic gateway→OpenAI/Google).
type AudioSpeechRequest struct {
	Model  string
	Input  string // text to speak
	Voice  string
	Format string  // mp3 | opus | aac | flac | wav | pcm (OpenAI); encoding hint for Google
	Speed  float64 // OpenAI 0.25–4.0; 0 means unset
}

// AudioTranscribeRequest is the dialect-neutral speech-to-text request.
// Multipart bodies typically stay byte-passthrough; this type is for JSON paths
// and capability/routing metadata.
type AudioTranscribeRequest struct {
	Model          string
	Language       string
	Prompt         string
	ResponseFormat string // json | text | srt | verbose_json | vtt
	Translate      bool   // true → translations endpoint (English)
}
