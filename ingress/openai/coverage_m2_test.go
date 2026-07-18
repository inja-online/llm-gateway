package openai

import (
	"encoding/json"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestParseReasoningContentEdges(t *testing.T) {
	if parseReasoningContent(nil) != nil {
		t.Fatal("nil")
	}
	if parseReasoningContent(json.RawMessage(`null`)) != nil {
		t.Fatal("null")
	}
	if parseReasoningContent(json.RawMessage(`""`)) != nil {
		t.Fatal("empty string")
	}
	b := parseReasoningContent(json.RawMessage(`"think"`))
	if b == nil || b.Type != canonical.BlockThinking || b.Text != "think" {
		t.Fatalf("%+v", b)
	}
	// non-string JSON preserved as raw text
	b = parseReasoningContent(json.RawMessage(`{"step":1}`))
	if b == nil || b.Text != `{"step":1}` {
		t.Fatalf("%+v", b)
	}
	// whitespace-only raw (invalid as string already handled) — array
	b = parseReasoningContent(json.RawMessage(`[1]`))
	if b == nil || b.Text != `[1]` {
		t.Fatalf("%+v", b)
	}
	// pure whitespace raw that unmarshals neither as non-empty string
	if parseReasoningContent(json.RawMessage(`   `)) != nil {
		// TrimSpace of spaces → empty → nil
		t.Fatal("whitespace")
	}
}

func TestAudioFormatMediaTypeAll(t *testing.T) {
	cases := map[string]string{
		"wav":   "audio/wav",
		"WAV":   "audio/wav",
		"mp3":   "audio/mpeg",
		"flac":  "audio/flac",
		"opus":  "audio/opus",
		"pcm16": "audio/pcm",
		"":      "",
		"ogg":   "audio/ogg",
		"webm":  "audio/webm",
	}
	for in, want := range cases {
		if got := audioFormatMediaType(in); got != want {
			t.Fatalf("%q → %q want %q", in, got, want)
		}
	}
}

func TestParseFilePartEdges(t *testing.T) {
	// file_id
	doc, err := parseFilePart(&fileObject{FileID: "file-1", Filename: "a.pdf"})
	if err != nil || doc.Kind != "file_id" || doc.Data != "file-1" || doc.Filename != "a.pdf" {
		t.Fatalf("%+v %v", doc, err)
	}
	// raw base64 without data URL
	doc, err = parseFilePart(&fileObject{FileData: "QQ==", Filename: "b.bin"})
	if err != nil || doc.Kind != "base64" || doc.Data != "QQ==" || doc.MediaType != "application/octet-stream" {
		t.Fatalf("%+v %v", doc, err)
	}
	// proper data URL
	doc, err = parseFilePart(&fileObject{FileData: "data:application/pdf;base64,JVBERi0="})
	if err != nil || doc.Kind != "base64" || doc.MediaType != "application/pdf" || doc.Data != "JVBERi0=" {
		t.Fatalf("%+v %v", doc, err)
	}
	// incomplete data URL (no comma)
	if _, err = parseFilePart(&fileObject{FileData: "data:application/pdf;base64"}); err == nil {
		t.Fatal("want incomplete error")
	}
	// non-base64 data URL
	if _, err = parseFilePart(&fileObject{FileData: "data:text/plain,hello"}); err == nil {
		t.Fatal("want base64 error")
	}
	// empty
	if _, err = parseFilePart(&fileObject{}); err == nil {
		t.Fatal("want empty error")
	}
}

func TestParseInputAudioAllFormats(t *testing.T) {
	for _, format := range []string{"wav", "mp3", "flac", "opus", "pcm16", "ogg"} {
		body := []byte(`{"model":"m","messages":[{"role":"user","content":[
			{"type":"input_audio","input_audio":{"data":"QQ==","format":"` + format + `"}}
		]}]}`)
		req, err := ParseRequest(body)
		if err != nil {
			t.Fatalf("%s: %v", format, err)
		}
		a := req.Messages[0].Content[0]
		if a.Type != canonical.BlockAudio || a.Audio == nil || a.Audio.Data != "QQ==" {
			t.Fatalf("%s: %+v", format, a)
		}
		want := audioFormatMediaType(format)
		if a.Audio.MediaType != want {
			t.Fatalf("%s: media %q want %q", format, a.Audio.MediaType, want)
		}
	}
}

func TestParseReasoningContentInMessages(t *testing.T) {
	// empty / null reasoning
	req, err := ParseRequest([]byte(`{
		"model":"m","messages":[
			{"role":"assistant","content":"hi","reasoning_content":null}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range req.Messages[0].Content {
		if b.Type == canonical.BlockThinking {
			t.Fatalf("null should not produce thinking: %+v", b)
		}
	}
	// empty string
	req, err = ParseRequest([]byte(`{
		"model":"m","messages":[
			{"role":"assistant","content":"hi","reasoning_content":""}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range req.Messages[0].Content {
		if b.Type == canonical.BlockThinking {
			t.Fatalf("empty should not produce thinking: %+v", b)
		}
	}
	// non-string reasoning
	req, err = ParseRequest([]byte(`{
		"model":"m","messages":[
			{"role":"assistant","content":"hi","reasoning_content":{"x":1}}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, b := range req.Messages[0].Content {
		if b.Type == canonical.BlockThinking && b.Text == `{"x":1}` {
			found = true
		}
	}
	if !found {
		t.Fatalf("%+v", req.Messages[0].Content)
	}
}

func TestParseFilePartDataURLErrorsInBody(t *testing.T) {
	_, err := ParseRequest([]byte(`{"model":"m","messages":[{"role":"user","content":[
		{"type":"file","file":{"file_data":"data:application/pdf;base64"}}
	]}]}`))
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("%v", err)
	}
	_, err = ParseRequest([]byte(`{"model":"m","messages":[{"role":"user","content":[
		{"type":"file","file":{"file_data":"data:text/plain,hi"}}
	]}]}`))
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("%v", err)
	}
	// raw base64 file_data
	req, err := ParseRequest([]byte(`{"model":"m","messages":[{"role":"user","content":[
		{"type":"file","file":{"file_data":"QQ==","filename":"x.bin"}}
	]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	d := req.Messages[0].Content[0].Document
	if d == nil || d.Kind != "base64" || d.MediaType != "application/octet-stream" {
		t.Fatalf("%+v", d)
	}
}
