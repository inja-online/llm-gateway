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
