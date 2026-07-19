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

func TestParseSpeechRequestFormatAliases(t *testing.T) {
	req, err := ParseSpeechRequest([]byte(`{"model":"m","text":"t","voice_name":"Aoede","audio_encoding":"LINEAR16"}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if req.Voice != "Aoede" || req.Format != "LINEAR16" {
		t.Fatalf("%+v", req)
	}
	req2, err := ParseSpeechRequest([]byte(`{"input":"i","response_format":"mp3"}`), "path-model")
	if err != nil {
		t.Fatal(err)
	}
	if req2.Model != "path-model" || req2.Format != "mp3" || req2.Input != "i" {
		t.Fatalf("%+v", req2)
	}
}

func TestParseSpeechRequestMissingModel(t *testing.T) {
	_, err := ParseSpeechRequest([]byte(`{"text":"x"}`), "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseSpeechRequestInvalidJSON(t *testing.T) {
	_, err := ParseSpeechRequest([]byte(`{`), "m")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSerializeSpeechGenerateContentResponse(t *testing.T) {
	in := []byte(`{"candidates":[]}`)
	if string(SerializeSpeechGenerateContentResponse(in)) != string(in) {
		t.Fatal("identity")
	}
}
