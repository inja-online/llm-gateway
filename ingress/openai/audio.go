package openai

import (
	"encoding/json"
	"fmt"

	"github.com/inja-online/llm-gateway/canonical"
)

// ParseSpeechRequest parses OpenAI POST /v1/audio/speech JSON.
func ParseSpeechRequest(body []byte) (*canonical.AudioSpeechRequest, error) {
	var wire struct {
		Model          string  `json:"model"`
		Input          string  `json:"input"`
		Voice          string  `json:"voice"`
		ResponseFormat string  `json:"response_format"`
		Speed          float64 `json:"speed"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("invalid speech request JSON: %w", err)
	}
	if wire.Model == "" {
		return nil, &ValidationError{Msg: "missing or invalid required field: model"}
	}
	if wire.Input == "" {
		return nil, &ValidationError{Msg: "missing or invalid required field: input"}
	}
	format := wire.ResponseFormat
	if format == "" {
		format = "mp3"
	}
	return &canonical.AudioSpeechRequest{
		Model:  wire.Model,
		Input:  wire.Input,
		Voice:  wire.Voice,
		Format: format,
		Speed:  wire.Speed,
	}, nil
}
