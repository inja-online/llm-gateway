package google

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/inja-online/llm-gateway/canonical"
)

// SpeechGenerateSpeechPath is the gateway-facing Google TTS route
// (design §5.4). Upstream kind:google is rewritten to generateContent.
func SpeechGenerateSpeechPath(model string) string {
	return "/models/" + model + ":generateSpeech"
}

// SpeechGenerateContentPath is the real Gemini TTS upstream path.
func SpeechGenerateContentPath(model string) string {
	return Path(model, false)
}

// BuildSpeechGenerateContentRequest builds a Gemini generateContent body that
// requests AUDIO modality (native Gemini TTS models).
//
// Wire shape (camelCase per REST samples):
//
//	{
//	  "contents": [{"parts":[{"text":"..."}]}],
//	  "generationConfig": {
//	    "responseModalities": ["AUDIO"],
//	    "speechConfig": {
//	      "voiceConfig": {
//	        "prebuiltVoiceConfig": {"voiceName": "Kore"}
//	      }
//	    }
//	  }
//	}
func BuildSpeechGenerateContentRequest(req *canonical.AudioSpeechRequest) ([]byte, error) {
	if req == nil || req.Input == "" {
		return nil, fmt.Errorf("speech input text is required")
	}
	voice := req.Voice
	if voice == "" {
		voice = "Kore"
	}
	// Map common OpenAI voice names to a Gemini prebuilt voice when translating.
	voice = mapOpenAIVoiceToGemini(voice)

	gc := map[string]any{
		"responseModalities": []string{"AUDIO"},
		"speechConfig": map[string]any{
			"voiceConfig": map[string]any{
				"prebuiltVoiceConfig": map[string]any{
					"voiceName": voice,
				},
			},
		},
	}
	out := map[string]any{
		"contents": []map[string]any{
			{"parts": []map[string]any{{"text": req.Input}}},
		},
		"generationConfig": gc,
	}
	return json.Marshal(out)
}

// mapOpenAIVoiceToGemini best-effort maps OpenAI TTS voice ids to Gemini
// prebuilt names. Unknown voices are passed through (may be Gemini names already).
func mapOpenAIVoiceToGemini(voice string) string {
	switch voice {
	case "alloy", "echo", "fable", "onyx", "nova", "shimmer", "ash", "ballad", "coral", "sage", "verse":
		// Stable default for OpenAI→Gemini translation.
		return "Kore"
	default:
		return voice
	}
}

// ParsedSpeechAudio is decoded audio from a Gemini TTS generateContent response.
type ParsedSpeechAudio struct {
	// Data is raw audio bytes (typically PCM s16le 24kHz mono for Gemini TTS).
	Data []byte
	// MIMEType from inlineData (e.g. audio/L16;codec=pcm;rate=24000).
	MIMEType string
}

// ParseSpeechGenerateContentResponse extracts the first inline audio part.
// No re-encoding is performed — bytes are base64-decoded only.
func ParseSpeechGenerateContentResponse(body []byte) (*ParsedSpeechAudio, error) {
	var wire struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					InlineData *struct {
						MIMEType      string `json:"mimeType"`
						MIMETypeSnake string `json:"mime_type"`
						Data          string `json:"data"`
					} `json:"inlineData"`
					InlineDataSnake *struct {
						MIMEType      string `json:"mimeType"`
						MIMETypeSnake string `json:"mime_type"`
						Data          string `json:"data"`
					} `json:"inline_data"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, err
	}
	for _, c := range wire.Candidates {
		for _, p := range c.Content.Parts {
			blob := p.InlineData
			if blob == nil {
				blob = p.InlineDataSnake
			}
			if blob == nil || blob.Data == "" {
				continue
			}
			raw, err := base64.StdEncoding.DecodeString(blob.Data)
			if err != nil {
				// Some transports send URL-safe base64.
				raw, err = base64.RawStdEncoding.DecodeString(blob.Data)
				if err != nil {
					return nil, fmt.Errorf("decode speech audio: %w", err)
				}
			}
			mt := blob.MIMEType
			if mt == "" {
				mt = blob.MIMETypeSnake
			}
			if mt == "" {
				mt = "audio/L16"
			}
			return &ParsedSpeechAudio{Data: raw, MIMEType: mt}, nil
		}
	}
	return nil, fmt.Errorf("no audio inlineData in generateContent response")
}
