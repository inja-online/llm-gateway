package openai

import "testing"

func TestParseSpeechRequestOK(t *testing.T) {
	req, err := ParseSpeechRequest([]byte(`{"model":"tts-1","input":"hello","voice":"alloy","response_format":"wav","speed":1.25}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Model != "tts-1" || req.Input != "hello" || req.Voice != "alloy" || req.Format != "wav" || req.Speed != 1.25 {
		t.Fatalf("%+v", req)
	}
}

func TestParseSpeechRequestDefaultFormat(t *testing.T) {
	req, err := ParseSpeechRequest([]byte(`{"model":"tts-1","input":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Format != "mp3" {
		t.Fatalf("format %q", req.Format)
	}
}

func TestParseSpeechRequestMissingModel(t *testing.T) {
	_, err := ParseSpeechRequest([]byte(`{"input":"x"}`))
	if err == nil {
		t.Fatal("expected error")
	}
	if ve, ok := err.(*ValidationError); !ok || ve.Msg == "" {
		t.Fatalf("%T %v", err, err)
	}
}

func TestParseSpeechRequestMissingInput(t *testing.T) {
	_, err := ParseSpeechRequest([]byte(`{"model":"tts-1"}`))
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("%T %v", err, err)
	}
}

func TestParseSpeechRequestInvalidJSON(t *testing.T) {
	_, err := ParseSpeechRequest([]byte(`{`))
	if err == nil {
		t.Fatal("expected error")
	}
}
