package google

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestBuildImageURLFileDataPassThrough(t *testing.T) {
	// OpenAI/Anthropic image_url → Google: emit file_data, never [image: …] text.
	req := &canonical.Request{
		Messages: []canonical.Message{{Role: canonical.RoleUser, Content: []canonical.Block{
			{Type: canonical.BlockText, Text: "describe"},
			{Type: canonical.BlockImage, Image: &canonical.ImageSource{
				Kind: "url", Data: "https://cdn.example.com/photo.png",
			}},
		}}},
	}
	body, err := BuildRequest(req, "gemini-2.0-flash")
	if err != nil {
		t.Fatal(err)
	}
	raw := string(body)
	if strings.Contains(raw, "[image:") {
		t.Fatalf("must not degrade to text placeholder: %s", raw)
	}
	var wire generateRequest
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.Contents) != 1 || len(wire.Contents[0].Parts) != 2 {
		t.Fatalf("parts: %+v", wire.Contents)
	}
	p := wire.Contents[0].Parts[1]
	if p.FileData == nil {
		t.Fatalf("want file_data, got %+v", p)
	}
	if p.FileData.FileURI != "https://cdn.example.com/photo.png" {
		t.Fatalf("file_uri: %s", p.FileData.FileURI)
	}
	if p.FileData.MIMEType != "image/png" {
		t.Fatalf("mime from extension: %q", p.FileData.MIMEType)
	}
	if p.InlineData != nil || p.Text != "" {
		t.Fatalf("unexpected inline/text: %+v", p)
	}
}

func TestBuildImageURLWithExplicitMIME(t *testing.T) {
	req := &canonical.Request{
		Messages: []canonical.Message{{Role: canonical.RoleUser, Content: []canonical.Block{
			{Type: canonical.BlockImage, Image: &canonical.ImageSource{
				Kind: "url", MediaType: "image/webp", Data: "https://x/a",
			}},
		}}},
	}
	body, err := BuildRequest(req, "m")
	if err != nil {
		t.Fatal(err)
	}
	var wire generateRequest
	json.Unmarshal(body, &wire)
	fd := wire.Contents[0].Parts[0].FileData
	if fd == nil || fd.MIMEType != "image/webp" || fd.FileURI != "https://x/a" {
		t.Fatalf("%+v", fd)
	}
}

func TestBuildImageBase64InlineData(t *testing.T) {
	req := &canonical.Request{
		Messages: []canonical.Message{{Role: canonical.RoleUser, Content: []canonical.Block{
			{Type: canonical.BlockImage, Image: &canonical.ImageSource{
				Kind: "base64", MediaType: "image/jpeg", Data: "QUJD",
			}},
		}}},
	}
	body, err := BuildRequest(req, "m")
	if err != nil {
		t.Fatal(err)
	}
	var wire generateRequest
	json.Unmarshal(body, &wire)
	id := wire.Contents[0].Parts[0].InlineData
	if id == nil || id.MIMEType != "image/jpeg" || id.Data != "QUJD" {
		t.Fatalf("%+v", id)
	}
	if wire.Contents[0].Parts[0].FileData != nil {
		t.Fatal("base64 must not emit file_data")
	}
}

func TestBuildImageDataURLNormalizedToInline(t *testing.T) {
	// data: URL carried as Kind url (OpenAI-style residual) → inline_data, no fetch.
	req := &canonical.Request{
		Messages: []canonical.Message{{Role: canonical.RoleUser, Content: []canonical.Block{
			{Type: canonical.BlockImage, Image: &canonical.ImageSource{
				Kind: "url", Data: "data:image/png;base64,iVBOR",
			}},
		}}},
	}
	body, err := BuildRequest(req, "m")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "file_data") {
		t.Fatalf("data URL must not use file_data: %s", body)
	}
	var wire generateRequest
	json.Unmarshal(body, &wire)
	id := wire.Contents[0].Parts[0].InlineData
	if id == nil || id.MIMEType != "image/png" || id.Data != "iVBOR" {
		t.Fatalf("%+v", id)
	}
}

func TestBuildImagePDFFileData(t *testing.T) {
	// Temporary policy: PDF as BlockImage → file_data with application/pdf.
	req := &canonical.Request{
		Messages: []canonical.Message{{Role: canonical.RoleUser, Content: []canonical.Block{
			{Type: canonical.BlockImage, Image: &canonical.ImageSource{
				Kind: "url", MediaType: "application/pdf",
				Data: "https://generativelanguage.googleapis.com/v1beta/files/abc123",
			}},
		}}},
	}
	body, err := BuildRequest(req, "m")
	if err != nil {
		t.Fatal(err)
	}
	var wire generateRequest
	json.Unmarshal(body, &wire)
	fd := wire.Contents[0].Parts[0].FileData
	if fd == nil || fd.MIMEType != "application/pdf" {
		t.Fatalf("%+v", fd)
	}
	if !strings.Contains(fd.FileURI, "files/abc123") {
		t.Fatalf("uri: %s", fd.FileURI)
	}
}

func TestBuildImageEmptyOmittedNoPlaceholder(t *testing.T) {
	req := &canonical.Request{
		Messages: []canonical.Message{{Role: canonical.RoleUser, Content: []canonical.Block{
			{Type: canonical.BlockText, Text: "hi"},
			{Type: canonical.BlockImage, Image: nil},
			{Type: canonical.BlockImage, Image: &canonical.ImageSource{Kind: "url", Data: ""}},
		}}},
	}
	body, err := BuildRequest(req, "m")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "[image:") {
		t.Fatalf("placeholder leaked: %s", body)
	}
	var wire generateRequest
	json.Unmarshal(body, &wire)
	if len(wire.Contents[0].Parts) != 1 || wire.Contents[0].Parts[0].Text != "hi" {
		t.Fatalf("empty images must be omitted: %+v", wire.Contents[0].Parts)
	}
}

func TestParseDataURLAndGuessMIME(t *testing.T) {
	mt, data, ok := parseDataURL("data:image/jpeg;base64,Zm9v")
	if !ok || mt != "image/jpeg" || data != "Zm9v" {
		t.Fatalf("%q %q %v", mt, data, ok)
	}
	if _, _, ok := parseDataURL("https://x/a.png"); ok {
		t.Fatal("http must not parse as data URL")
	}
	if _, _, ok := parseDataURL("data:text/plain,hello"); ok {
		t.Fatal("non-base64 data URL rejected")
	}
	if guessMIMEFromURI("https://cdn/x.WEBP?w=1") != "image/webp" {
		t.Fatal(guessMIMEFromURI("https://cdn/x.WEBP?w=1"))
	}
	if guessMIMEFromURI("https://cdn/doc.PDF") != "application/pdf" {
		t.Fatal()
	}
	if guessMIMEFromURI("https://cdn/noext") != "" {
		t.Fatal()
	}
}
