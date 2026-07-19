package google

import "testing"

func TestParseSpeechRequest(t *testing.T) {
	req, err := ParseSpeechRequest([]byte(`{"text":"hello","voiceName":"Kore"}`), "gemini-tts")
	if err != nil {
		t.Fatal(err)
	}
	if req.Model != "gemini-tts" || req.Input != "hello" || req.Voice != "Kore" {
		t.Fatalf("%+v", req)
	}
}

func TestParseSpeechRequestInputAlias(t *testing.T) {
	req, err := ParseSpeechRequest([]byte(`{"model":"m","input":"x","voice":"Puck"}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if req.Input != "x" || req.Voice != "Puck" {
		t.Fatalf("%+v", req)
	}
}

func TestParseSpeechRequestMissingText(t *testing.T) {
	_, err := ParseSpeechRequest([]byte(`{"model":"m"}`), "m")
	if err == nil {
		t.Fatal("expected error")
	}
}
