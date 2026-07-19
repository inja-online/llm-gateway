package google

import (
	"encoding/json"
	"fmt"

	"github.com/inja-online/llm-gateway/canonical"
)

// ParseSpeechRequest parses a gateway Google :generateSpeech body.
// Accepts text/input + voice/voiceName + optional model (for routing).
func ParseSpeechRequest(body []byte, pathModel string) (*canonical.AudioSpeechRequest, error) {
	var wire struct {
		Model      string  `json:"model"`
		Text       string  `json:"text"`
		Input      string  `json:"input"`
		Voice      string  `json:"voice"`
		VoiceName  string  `json:"voiceName"`
		VoiceSnake string  `json:"voice_name"`
		// Encoding / format hints (gateway; not always sent upstream).
		AudioEncoding string  `json:"audioEncoding"`
		EncodingSnake string  `json:"audio_encoding"`
		Format        string  `json:"format"`
		ResponseFmt   string  `json:"response_format"`
		Speed         float64 `json:"speed"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("invalid speech request JSON: %w", err)
	}
	model := wire.Model
	if model == "" {
		model = pathModel
	}
	if model == "" {
		return nil, &ValidationError{Msg: "missing model"}
	}
	text := wire.Text
	if text == "" {
		text = wire.Input
	}
	if text == "" {
		return nil, &ValidationError{Msg: "missing text (or input)"}
	}
	voice := wire.Voice
	if voice == "" {
		voice = wire.VoiceName
	}
	if voice == "" {
		voice = wire.VoiceSnake
	}
	format := wire.Format
	if format == "" {
		format = wire.ResponseFmt
	}
	if format == "" {
		format = wire.AudioEncoding
	}
	if format == "" {
		format = wire.EncodingSnake
	}
	return &canonical.AudioSpeechRequest{
		Model:  model,
		Input:  text,
		Voice:  voice,
		Format: format,
		Speed:  wire.Speed,
	}, nil
}

// SerializeSpeechGenerateContentResponse is a no-op identity for Google clients
// that receive the native generateContent JSON (includes base64 audio).
// Exposed for symmetry with other media serializers.
func SerializeSpeechGenerateContentResponse(upstreamBody []byte) []byte {
	return upstreamBody
}
