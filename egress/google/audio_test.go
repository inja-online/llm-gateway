package google

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestBuildSpeechGenerateContentRequest(t *testing.T) {
	body, err := BuildSpeechGenerateContentRequest(&canonical.AudioSpeechRequest{
		Input: "Hello",
		Voice: "Puck",
	})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	gc := m["generationConfig"].(map[string]any)
	mods := gc["responseModalities"].([]any)
	if mods[0] != "AUDIO" {
		t.Fatalf("%v", mods)
	}
	sc := gc["speechConfig"].(map[string]any)
	vc := sc["voiceConfig"].(map[string]any)
	pvc := vc["prebuiltVoiceConfig"].(map[string]any)
	if pvc["voiceName"] != "Puck" {
		t.Fatalf("%v", pvc)
	}
}

func TestBuildSpeechMapsOpenAIVoice(t *testing.T) {
	body, err := BuildSpeechGenerateContentRequest(&canonical.AudioSpeechRequest{
		Input: "x",
		Voice: "nova",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Kore") {
		t.Fatalf("%s", body)
	}
}

func TestParseSpeechGenerateContentResponse(t *testing.T) {
	raw := []byte{1, 2, 3, 4, 5}
	b64 := base64.StdEncoding.EncodeToString(raw)
	wire := `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"audio/L16","data":"` + b64 + `"}}]}}]}`
	got, err := ParseSpeechGenerateContentResponse([]byte(wire))
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Data) != string(raw) {
		t.Fatalf("%v", got.Data)
	}
	if got.MIMEType != "audio/L16" {
		t.Fatalf("%s", got.MIMEType)
	}
}

func TestSpeechPaths(t *testing.T) {
	if SpeechGenerateSpeechPath("m") != "/models/m:generateSpeech" {
		t.Fatal(SpeechGenerateSpeechPath("m"))
	}
	if !strings.HasSuffix(SpeechGenerateContentPath("m"), ":generateContent") {
		t.Fatal(SpeechGenerateContentPath("m"))
	}
}

func TestBuildSpeechGenerateContentRequestNilAndDefaultVoice(t *testing.T) {
	if _, err := BuildSpeechGenerateContentRequest(nil); err == nil {
		t.Fatal("nil req")
	}
	if _, err := BuildSpeechGenerateContentRequest(&canonical.AudioSpeechRequest{}); err == nil {
		t.Fatal("empty input")
	}
	body, err := BuildSpeechGenerateContentRequest(&canonical.AudioSpeechRequest{Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Kore") {
		t.Fatalf("default voice: %s", body)
	}
}

func TestParseSpeechGenerateContentResponseEdges(t *testing.T) {
	// invalid JSON
	if _, err := ParseSpeechGenerateContentResponse([]byte(`{`)); err == nil {
		t.Fatal("invalid json")
	}
	// no candidates
	if _, err := ParseSpeechGenerateContentResponse([]byte(`{"candidates":[]}`)); err == nil {
		t.Fatal("empty candidates")
	}
	// skip nil / empty parts then succeed with snake_case + mime_type
	raw := []byte{9, 8, 7}
	// RawStdEncoding path: strip padding from std b64
	std := base64.StdEncoding.EncodeToString(raw)
	rawB64 := strings.TrimRight(std, "=")
	// Prefer a path that fails StdEncoding (padding missing) then succeeds RawStdEncoding
	wire := `{"candidates":[{"content":{"parts":[
		{"text":"skip"},
		{"inline_data":{"mime_type":"audio/custom","data":"` + rawB64 + `"}}
	]}}]}`
	got, err := ParseSpeechGenerateContentResponse([]byte(wire))
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Data) != string(raw) {
		t.Fatalf("data %v", got.Data)
	}
	if got.MIMEType != "audio/custom" {
		t.Fatalf("mime %q", got.MIMEType)
	}

	// empty mime → default audio/L16
	b64 := base64.StdEncoding.EncodeToString([]byte{1})
	got2, err := ParseSpeechGenerateContentResponse([]byte(
		`{"candidates":[{"content":{"parts":[{"inlineData":{"data":"` + b64 + `"}}]}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got2.MIMEType != "audio/L16" {
		t.Fatalf("default mime %q", got2.MIMEType)
	}

	// undecodable base64
	if _, err := ParseSpeechGenerateContentResponse([]byte(
		`{"candidates":[{"content":{"parts":[{"inlineData":{"data":"!!!"}}]}}]}`)); err == nil {
		t.Fatal("bad b64")
	}
}

func TestMapOpenAIVoiceToGeminiPassthrough(t *testing.T) {
	if mapOpenAIVoiceToGemini("Puck") != "Puck" {
		t.Fatal("passthrough")
	}
	if mapOpenAIVoiceToGemini("alloy") != "Kore" {
		t.Fatal("map alloy")
	}
}
