package google

import (
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestParseImageRequestGenerateImagesShape(t *testing.T) {
	req, err := ParseImageRequest([]byte(`{
		"prompt":"robot",
		"numberOfImages":3,
		"aspectRatio":"16:9",
		"imageSize":"1K"
	}`), "imagen-4")
	if err != nil {
		t.Fatal(err)
	}
	if req.Prompt != "robot" || req.N != 3 || req.Style != "16:9" || req.Size != "1K" {
		t.Fatalf("%+v", req)
	}
	if req.Model != "imagen-4" {
		t.Fatalf("model %s", req.Model)
	}
}

func TestParseImageRequestPredictShape(t *testing.T) {
	req, err := ParseImageRequest([]byte(`{
		"instances":[{"prompt":"cat"}],
		"parameters":{"sampleCount":2,"aspect_ratio":"1:1"}
	}`), "imagen")
	if err != nil {
		t.Fatal(err)
	}
	if req.Prompt != "cat" || req.N != 2 || req.Style != "1:1" {
		t.Fatalf("%+v", req)
	}
}

func TestParseImageRequestMissingPrompt(t *testing.T) {
	_, err := ParseImageRequest([]byte(`{}`), "m")
	if err == nil {
		t.Fatal()
	}
}

func TestSerializeParseImageResponse(t *testing.T) {
	out, err := SerializeImageResponse(&canonical.ImageGenResponse{
		Images: []canonical.GeneratedImage{
			{B64JSON: "YQ==", MediaType: "image/png"},
			{URL: "https://x/i.png"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "predictions") {
		t.Fatalf("%s", out)
	}
	back, err := ParseImageResponse(out, "m")
	if err != nil || len(back.Images) < 1 {
		t.Fatalf("%+v %v", back, err)
	}
	// generatedImages form
	resp, err := ParseImageResponse([]byte(`{
		"generatedImages":[{"image":{"imageBytes":"YQ==","mimeType":"image/jpeg"}}]
	}`), "m")
	if err != nil || len(resp.Images) != 1 || resp.Images[0].B64JSON != "YQ==" {
		t.Fatalf("%+v %v", resp, err)
	}
	resp2, err := ParseImageResponse([]byte(`{
		"generated_images":[{"image":{"image_bytes":"Yg=="}}]
	}`), "m")
	if err != nil || len(resp2.Images) != 1 {
		t.Fatalf("%+v %v", resp2, err)
	}
}
