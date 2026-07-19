package openai

import (
	"encoding/json"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestBuildSpeechRequest(t *testing.T) {
	body, err := BuildSpeechRequest(&canonical.AudioSpeechRequest{
		Input:  "hi",
		Voice:  "alloy",
		Format: "wav",
		Speed:  1.25,
	}, "tts-1")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	if m["model"] != "tts-1" || m["input"] != "hi" || m["voice"] != "alloy" {
		t.Fatalf("%v", m)
	}
	if m["response_format"] != "wav" {
		t.Fatalf("%v", m["response_format"])
	}
	if m["speed"].(float64) != 1.25 {
		t.Fatalf("%v", m["speed"])
	}
}

func TestAudioPaths(t *testing.T) {
	if SpeechPath() != "/audio/speech" {
		t.Fatal(SpeechPath())
	}
	if TranscriptionsPath() != "/audio/transcriptions" {
		t.Fatal(TranscriptionsPath())
	}
	if TranslationsPath() != "/audio/translations" {
		t.Fatal(TranslationsPath())
	}
}
