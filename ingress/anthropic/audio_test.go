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

func TestParseSpeechRequestInvalidAndMissingInput(t *testing.T) {
	if _, err := ParseSpeechRequest([]byte(`{`)); err == nil {
		t.Fatal("invalid json")
	}
	if _, err := ParseSpeechRequest([]byte(`{"model":"m"}`)); err == nil {
		t.Fatal("missing input")
	}
}

func TestParseTranscribeJSONRequestEdges(t *testing.T) {
	if _, err := ParseTranscribeJSONRequest([]byte(`{`)); err == nil {
		t.Fatal("invalid json")
	}
	if _, err := ParseTranscribeJSONRequest([]byte(`{"language":"en"}`)); err == nil {
		t.Fatal("missing model")
	}
	req, err := ParseTranscribeJSONRequest([]byte(`{"model":"w","prompt":"p","response_format":"verbose_json"}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Prompt != "p" || req.ResponseFormat != "verbose_json" {
		t.Fatalf("%+v", req)
	}
}
