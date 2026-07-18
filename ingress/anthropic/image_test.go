package anthropic

import (
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestParseAnthropicImageAndSerialize(t *testing.T) {
	req, err := ParseImageRequest([]byte(`{
		"model":"google/imagen-3",
		"prompt":"a red cube",
		"n":1,
		"size":"1024x1024",
		"response_format":"base64"
	}`), canonical.ImageModeGenerate)
	if err != nil {
		t.Fatal(err)
	}
	if req.Model != "google/imagen-3" || req.Prompt != "a red cube" {
		t.Fatalf("%+v", req)
	}
	out, err := SerializeImageResponse(&canonical.ImageGenResponse{
		ID:    "img_01",
		Model: "imagen-3",
		Images: []canonical.GeneratedImage{
			{B64JSON: "YQ==", MediaType: "image/png"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{`"type":"image_generation"`, `"b64_json":"YQ=="`, `"media_type":"image/png"`, `"id":"img_01"`} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %s in %s", want, s)
		}
	}
}

func TestParseAnthropicImageEdit(t *testing.T) {
	req, err := ParseImageRequest([]byte(`{"model":"dall-e-2","prompt":"hat","image":"YQ=="}`), canonical.ImageModeEdit)
	if err != nil {
		t.Fatal(err)
	}
	if req.Mode != canonical.ImageModeEdit || len(req.Images) != 1 {
		t.Fatalf("%+v", req)
	}
}
