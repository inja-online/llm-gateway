package openai

import (
	"encoding/json"

	"github.com/inja-online/llm-gateway/canonical"
)

// SpeechPath is POST /audio/speech (TTS; binary response).
func SpeechPath() string { return "/audio/speech" }

// TranscriptionsPath is POST /audio/transcriptions (STT).
func TranscriptionsPath() string { return "/audio/transcriptions" }

// TranslationsPath is POST /audio/translations (STT → English).
func TranslationsPath() string { return "/audio/translations" }

// BuildSpeechRequest builds an OpenAI TTS JSON body from canonical.
func BuildSpeechRequest(req *canonical.AudioSpeechRequest, model string) ([]byte, error) {
	out := map[string]any{
		"model": model,
		"input": req.Input,
	}
	if req.Voice != "" {
		out["voice"] = req.Voice
	}
	format := req.Format
	if format == "" {
		format = "mp3"
	}
	out["response_format"] = format
	if req.Speed > 0 {
		out["speed"] = req.Speed
	}
	return json.Marshal(out)
}
