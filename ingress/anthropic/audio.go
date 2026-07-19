package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/inja-online/llm-gateway/canonical"
)

// ParseSpeechRequest parses Gateway Media Contract v1 Anthropic TTS
// (POST /v1/audio/speech with anthropic-version).
// Accepts OpenAI-shaped fields (input, voice, response_format) and text alias.
func ParseSpeechRequest(body []byte) (*canonical.AudioSpeechRequest, error) {
	var wire struct {
		Model          string  `json:"model"`
		Input          string  `json:"input"`
		Text           string  `json:"text"`
		Voice          string  `json:"voice"`
		ResponseFormat string  `json:"response_format"`
		Format         string  `json:"format"`
		Speed          float64 `json:"speed"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("invalid speech request JSON: %w", err)
	}
	if wire.Model == "" {
		return nil, &ValidationError{Msg: "missing or invalid required field: model"}
	}
	input := wire.Input
	if input == "" {
		input = wire.Text
	}
	if input == "" {
		return nil, &ValidationError{Msg: "missing or invalid required field: input"}
	}
	format := wire.ResponseFormat
	if format == "" {
		format = wire.Format
	}
	if format == "" {
		format = "mp3"
	}
	return &canonical.AudioSpeechRequest{
		Model:  wire.Model,
		Input:  input,
		Voice:  wire.Voice,
		Format: format,
		Speed:  wire.Speed,
	}, nil
}

// ParseTranscribeJSONRequest peeks model (and optional fields) from a JSON STT body.
func ParseTranscribeJSONRequest(body []byte) (*canonical.AudioTranscribeRequest, error) {
	var wire struct {
		Model          string `json:"model"`
		Language       string `json:"language"`
		Prompt         string `json:"prompt"`
		ResponseFormat string `json:"response_format"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("invalid transcription request JSON: %w", err)
	}
	if wire.Model == "" {
		return nil, &ValidationError{Msg: "missing or invalid required field: model"}
	}
	return &canonical.AudioTranscribeRequest{
		Model:          wire.Model,
		Language:       wire.Language,
		Prompt:         wire.Prompt,
		ResponseFormat: wire.ResponseFormat,
	}, nil
}
