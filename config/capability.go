package config

// Modalities a provider may advertise. Kept as strings so hooks/proxy can
// share the same vocabulary without importing each other for constants only.
const (
	ModalityText            = "text"
	ModalityImageGen        = "image_gen"
	ModalityVideoGen        = "video_gen"
	ModalityAudioSpeech     = "audio_speech"
	ModalityAudioTranscribe = "audio_transcribe"
	ModalityRealtime        = "realtime"
)

// Capabilities declares which modalities a provider can serve.
// When Provider.Capabilities is nil, DefaultCapabilities(kind) applies.
type Capabilities struct {
	Text            bool `yaml:"text"`
	ImageGen        bool `yaml:"image_gen"`
	VideoGen        bool `yaml:"video_gen"`
	AudioSpeech     bool `yaml:"audio_speech"`
	AudioTranscribe bool `yaml:"audio_transcribe"`
	Realtime        bool `yaml:"realtime"`
}

// DefaultCapabilities returns built-in defaults for a provider kind.
//
//	openai      — all modalities
//	google      — text + media + realtime (Live)
//	anthropic   — text only (no native image/video/audio/realtime APIs)
//	openai_compat — text only (media/realtime must be opted in)
func DefaultCapabilities(kind string) Capabilities {
	switch kind {
	case KindOpenAI:
		return Capabilities{
			Text: true, ImageGen: true, VideoGen: true,
			AudioSpeech: true, AudioTranscribe: true, Realtime: true,
		}
	case KindGoogle:
		return Capabilities{
			Text: true, ImageGen: true, VideoGen: true,
			AudioSpeech: true, AudioTranscribe: true, Realtime: true,
		}
	case KindAnthropic:
		return Capabilities{Text: true}
	case KindOpenAICompat:
		return Capabilities{Text: true}
	default:
		return Capabilities{}
	}
}

// EffectiveCapabilities returns explicit overrides or kind defaults.
func (p Provider) EffectiveCapabilities() Capabilities {
	if p.Capabilities != nil {
		return *p.Capabilities
	}
	return DefaultCapabilities(p.Kind)
}

// Supports reports whether the provider can serve modality.
func (p Provider) Supports(modality string) bool {
	c := p.EffectiveCapabilities()
	switch modality {
	case ModalityText:
		return c.Text
	case ModalityImageGen:
		return c.ImageGen
	case ModalityVideoGen:
		return c.VideoGen
	case ModalityAudioSpeech:
		return c.AudioSpeech
	case ModalityAudioTranscribe:
		return c.AudioTranscribe
	case ModalityRealtime:
		return c.Realtime
	default:
		return false
	}
}
