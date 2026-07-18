package openai

import (
	"encoding/json"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestMediaTypeToAudioFormatAll(t *testing.T) {
	cases := map[string]string{
		"audio/wav":   "wav",
		"audio/wave":  "wav",
		"audio/x-wav": "wav",
		"audio/mpeg":  "mp3",
		"audio/mp3":   "mp3",
		"audio/flac":  "flac",
		"audio/opus":  "opus",
		"audio/pcm":   "pcm16",
		"audio/ogg":   "",
		"":            "",
		"video/mp4":   "",
	}
	for in, want := range cases {
		if got := mediaTypeToAudioFormat(in); got != want {
			t.Fatalf("%q → %q want %q", in, got, want)
		}
	}
}

func TestAudioMediaTypeFormatAll(t *testing.T) {
	cases := map[string]string{
		"audio/wav":   "wav",
		"audio/x-wav": "wav",
		"wav":         "wav",
		"audio/mpeg":  "mp3",
		"audio/mp3":   "mp3",
		"mp3":         "mp3",
		"audio/mp4":   "mp4",
		"audio/m4a":   "mp4",
		"mp4":         "mp4",
		"m4a":         "mp4",
		"audio/webm":  "webm",
		"webm":        "webm",
		"audio/ogg":   "ogg",
		"ogg":         "ogg",
		"audio/flac":  "flac",
		"flac":        "flac",
		"":            "",
		"aac":         "aac", // short format name passthrough
		"audio/unknown-long-type": "wav", // long unknown with slash → wav default
	}
	for in, want := range cases {
		if got := audioMediaTypeFormat(in); got != want {
			t.Fatalf("%q → %q want %q", in, got, want)
		}
	}
}

func TestContainsSlash(t *testing.T) {
	if !containsSlash("audio/wav") {
		t.Fatal("want true")
	}
	if containsSlash("wav") {
		t.Fatal("want false")
	}
	if containsSlash("") {
		t.Fatal("empty")
	}
	if !containsSlash("/") {
		t.Fatal("slash only")
	}
}

func TestDocumentToFileBranches(t *testing.T) {
	// file_id
	f := documentToFile(&canonical.DocumentSource{Kind: "file_id", Data: "file-1", Filename: "a.pdf"})
	if f.FileID != "file-1" || f.Filename != "a.pdf" {
		t.Fatalf("%+v", f)
	}
	// url
	f = documentToFile(&canonical.DocumentSource{Kind: "url", Data: "https://x/a.pdf", Filename: "a.pdf"})
	if f.FileData != "https://x/a.pdf" || f.Filename != "a.pdf" {
		t.Fatalf("%+v", f)
	}
	// base64 with media
	f = documentToFile(&canonical.DocumentSource{Kind: "base64", Data: "QQ==", MediaType: "application/pdf"})
	if f.FileData != "data:application/pdf;base64,QQ==" {
		t.Fatalf("%+v", f)
	}
	// base64 without media → octet-stream
	f = documentToFile(&canonical.DocumentSource{Kind: "base64", Data: "QQ=="})
	if f.FileData != "data:application/octet-stream;base64,QQ==" {
		t.Fatalf("%+v", f)
	}
	// default kind treated as base64
	f = documentToFile(&canonical.DocumentSource{Kind: "other", Data: "ZZ", MediaType: "text/plain"})
	if f.FileData != "data:text/plain;base64,ZZ" {
		t.Fatalf("%+v", f)
	}
}

func TestBuildResponseFormatAll(t *testing.T) {
	if buildResponseFormat(nil) != nil {
		t.Fatal("nil")
	}
	if buildResponseFormat(&canonical.ResponseFormat{Kind: "xml"}) != nil {
		t.Fatal("unknown")
	}
	rf := buildResponseFormat(&canonical.ResponseFormat{Kind: canonical.ResponseFormatText})
	if rf == nil || rf.Type != "text" {
		t.Fatalf("%+v", rf)
	}
	rf = buildResponseFormat(&canonical.ResponseFormat{Kind: canonical.ResponseFormatJSONObject})
	if rf == nil || rf.Type != "json_object" {
		t.Fatalf("%+v", rf)
	}
	strict := true
	rf = buildResponseFormat(&canonical.ResponseFormat{
		Kind:        canonical.ResponseFormatJSONSchema,
		Name:        "n",
		Description: "d",
		Schema:      json.RawMessage(`{"type":"object"}`),
		Strict:      &strict,
	})
	if rf == nil || rf.Type != "json_schema" || rf.JSONSchema == nil || rf.JSONSchema.Name != "n" {
		t.Fatalf("%+v", rf)
	}
}

func TestBuildAudioAndDocumentMessages(t *testing.T) {
	body, err := BuildRequest(&canonical.Request{
		Messages: []canonical.Message{{
			Role: canonical.RoleUser,
			Content: []canonical.Block{
				{Type: canonical.BlockAudio, Audio: &canonical.AudioSource{
					Kind: "base64", Data: "QUJD", MediaType: "audio/mpeg",
				}},
				{Type: canonical.BlockAudio, Audio: &canonical.AudioSource{
					Kind: "base64", Data: "WAV", MediaType: "wav",
				}},
				{Type: canonical.BlockAudio, Audio: &canonical.AudioSource{
					Kind: "base64", Data: "X", MediaType: "audio/weird-long-type-name",
				}},
				{Type: canonical.BlockDocument, Document: &canonical.DocumentSource{
					Kind: "file_id", Data: "file-9", Filename: "a.pdf",
				}},
				{Type: canonical.BlockDocument, Document: &canonical.DocumentSource{
					Kind: "url", Data: "https://x/a.pdf",
				}},
				{Type: canonical.BlockDocument, Document: &canonical.DocumentSource{
					Kind: "base64", Data: "QQ==", MediaType: "application/pdf",
				}},
				{Type: canonical.BlockDocument, Document: &canonical.DocumentSource{
					Kind: "base64", Data: "YY",
				}},
			},
		}},
	}, "m")
	if err != nil {
		t.Fatal(err)
	}
	var out chatRequest
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Messages) != 1 {
		t.Fatalf("%d msgs", len(out.Messages))
	}
	raw := string(out.Messages[0].Content)
	for _, want := range []string{
		`"input_audio"`,
		`"format":"mp3"`,
		`"format":"wav"`,
		`"file"`,
		`"file_id":"file-9"`,
		`"file_data"`,
	} {
		if !containsSubstring(raw, want) {
			t.Errorf("missing %s in %s", want, raw)
		}
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}

func TestBuildAudioEmptyMediaTypeDefaultsWav(t *testing.T) {
	body, err := BuildRequest(&canonical.Request{
		Messages: []canonical.Message{{
			Role: canonical.RoleUser,
			Content: []canonical.Block{
				{Type: canonical.BlockAudio, Audio: &canonical.AudioSource{Data: "AA", MediaType: ""}},
			},
		}},
	}, "m")
	if err != nil {
		t.Fatal(err)
	}
	if !containsSubstring(string(body), `"format":"wav"`) {
		t.Fatalf("%s", body)
	}
}
