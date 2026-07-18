package google

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
	googleegress "github.com/inja-online/llm-gateway/egress/google"
)

func TestParseFileDataImage(t *testing.T) {
	body := []byte(`{
		"model":"gemini-2.0-flash",
		"contents":[{
			"role":"user",
			"parts":[
				{"text":"what is this?"},
				{"file_data":{"mime_type":"image/png","file_uri":"https://cdn.example.com/cat.png"}}
			]
		}]
	}`)
	req, err := ParseRequest(body, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Messages) != 1 || len(req.Messages[0].Content) != 2 {
		t.Fatalf("%+v", req.Messages)
	}
	img := req.Messages[0].Content[1]
	if img.Type != canonical.BlockImage || img.Image == nil {
		t.Fatalf("%+v", img)
	}
	if img.Image.Kind != "url" || img.Image.MediaType != "image/png" {
		t.Fatalf("%+v", img.Image)
	}
	if img.Image.Data != "https://cdn.example.com/cat.png" {
		t.Fatalf("uri: %s", img.Image.Data)
	}

	// Round-trip: Google ingress → canonical → Google egress keeps file_data.
	out, err := googleegress.BuildRequest(req, "gemini-2.0-flash")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "[image:") {
		t.Fatalf("placeholder: %s", out)
	}
	if !strings.Contains(string(out), "file_data") || !strings.Contains(string(out), "cat.png") {
		t.Fatalf("want file_data pass-through: %s", out)
	}
}

func TestParseFileDataPDF(t *testing.T) {
	// Temporary policy: PDF mime maps to BlockImage until BlockDocument exists.
	body := []byte(`{
		"model":"g",
		"contents":[{"parts":[
			{"file_data":{"mime_type":"application/pdf","file_uri":"https://generativelanguage.googleapis.com/v1beta/files/pdf1"}}
		]}]
	}`)
	req, err := ParseRequest(body, "")
	if err != nil {
		t.Fatal(err)
	}
	b := req.Messages[0].Content[0]
	if b.Type != canonical.BlockImage || b.Image == nil {
		t.Fatalf("%+v", b)
	}
	if b.Image.MediaType != "application/pdf" {
		t.Fatalf("mime: %s", b.Image.MediaType)
	}
	if !strings.Contains(b.Image.Data, "files/pdf1") {
		t.Fatalf("uri: %s", b.Image.Data)
	}

	out, err := googleegress.BuildRequest(req, "g")
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(out, &wire); err != nil {
		t.Fatal(err)
	}
	contents := wire["contents"].([]any)
	parts := contents[0].(map[string]any)["parts"].([]any)
	fd := parts[0].(map[string]any)["file_data"].(map[string]any)
	if fd["mime_type"] != "application/pdf" {
		t.Fatalf("%v", fd)
	}
	if !strings.Contains(fd["file_uri"].(string), "files/pdf1") {
		t.Fatalf("%v", fd)
	}
}

func TestParseInlineDataPDF(t *testing.T) {
	body := []byte(`{
		"model":"g",
		"contents":[{"parts":[
			{"inline_data":{"mime_type":"application/pdf","data":"JVBERi0="}}
		]}]
	}`)
	req, err := ParseRequest(body, "")
	if err != nil {
		t.Fatal(err)
	}
	img := req.Messages[0].Content[0].Image
	if img == nil || img.Kind != "base64" || img.MediaType != "application/pdf" || img.Data != "JVBERi0=" {
		t.Fatalf("%+v", img)
	}
	out, err := googleegress.BuildRequest(req, "g")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "inline_data") || !strings.Contains(string(out), "JVBERi0=") {
		t.Fatalf("%s", out)
	}
}

func TestParseFileDataCamelCase(t *testing.T) {
	body := []byte(`{
		"model":"g",
		"contents":[{"parts":[
			{"fileData":{"mimeType":"image/jpeg","fileUri":"gs://bucket/x.jpg"}}
		]}]
	}`)
	req, err := ParseRequest(body, "")
	if err != nil {
		t.Fatal(err)
	}
	img := req.Messages[0].Content[0].Image
	if img == nil || img.MediaType != "image/jpeg" || img.Data != "gs://bucket/x.jpg" {
		t.Fatalf("%+v", img)
	}
}

func TestParseFileDataEmptyURISkipped(t *testing.T) {
	body := []byte(`{
		"model":"g",
		"contents":[{"parts":[
			{"text":"only"},
			{"file_data":{"mime_type":"image/png","file_uri":""}}
		]}]
	}`)
	req, err := ParseRequest(body, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Messages[0].Content) != 1 || req.Messages[0].Content[0].Text != "only" {
		t.Fatalf("%+v", req.Messages[0].Content)
	}
}
