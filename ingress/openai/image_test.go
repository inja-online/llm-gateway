package openai

import (
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestParseSerializeImageRoundTrip(t *testing.T) {
	body := []byte(`{"model":"dall-e-3","prompt":"a cat","n":2,"size":"1024x1024","response_format":"b64_json"}`)
	req, err := ParseImageRequest(body, canonical.ImageModeGenerate)
	if err != nil {
		t.Fatal(err)
	}
	if req.Model != "dall-e-3" || req.N != 2 || req.ResponseFormat != canonical.ImageFormatB64 {
		t.Fatalf("%+v", req)
	}
	resp := &canonical.ImageGenResponse{
		Created: 1,
		Images:  []canonical.GeneratedImage{{B64JSON: "YQ==", RevisedPrompt: "cat"}},
	}
	out, err := SerializeImageResponse(resp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "b64_json") {
		t.Fatalf("%s", out)
	}
	back, err := ParseImageResponse(out, "dall-e-3")
	if err != nil || len(back.Images) != 1 || back.Images[0].B64JSON != "YQ==" {
		t.Fatalf("%+v %v", back, err)
	}
}

func TestParseImageMissingModel(t *testing.T) {
	_, err := ParseImageRequest([]byte(`{"prompt":"x"}`), canonical.ImageModeGenerate)
	if err == nil {
		t.Fatal()
	}
	_, err = ParseImageRequest([]byte(`{"model":"m"}`), canonical.ImageModeGenerate)
	if err == nil {
		t.Fatal("missing prompt")
	}
	edit, err := ParseImageRequest([]byte(`{"model":"m","image":"YQ==","mask":"Yg=="}`), canonical.ImageModeEdit)
	if err != nil || len(edit.Images) != 1 || edit.Mask == nil {
		t.Fatalf("%+v %v", edit, err)
	}
	// invalid json
	if _, err := ParseImageRequest([]byte(`{`), canonical.ImageModeGenerate); err == nil {
		t.Fatal()
	}
	// serialize empty created uses now
	out, _ := SerializeImageResponse(&canonical.ImageGenResponse{
		Images: []canonical.GeneratedImage{{URL: "https://x"}},
	})
	if !strings.Contains(string(out), "https://x") {
		t.Fatalf("%s", out)
	}
}
