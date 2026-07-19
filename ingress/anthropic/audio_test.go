package anthropic

import "testing"

func TestParseSpeechRequest(t *testing.T) {
	req, err := ParseSpeechRequest([]byte(`{"model":"tts-1","input":"hi","voice":"alloy"}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Model != "tts-1" || req.Input != "hi" || req.Format != "mp3" {
		t.Fatalf("%+v", req)
	}
}

func TestParseSpeechRequestTextAlias(t *testing.T) {
	req, err := ParseSpeechRequest([]byte(`{"model":"tts-1","text":"yo","format":"wav"}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Input != "yo" || req.Format != "wav" {
		t.Fatalf("%+v", req)
	}
}

func TestParseSpeechRequestMissingModel(t *testing.T) {
	_, err := ParseSpeechRequest([]byte(`{"input":"x"}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseTranscribeJSONRequest(t *testing.T) {
	req, err := ParseTranscribeJSONRequest([]byte(`{"model":"whisper-1","language":"en"}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Model != "whisper-1" || req.Language != "en" {
		t.Fatalf("%+v", req)
	}
}
