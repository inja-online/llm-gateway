package google

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestBuildImagePredictRequest(t *testing.T) {
	body, err := BuildImagePredictRequest(&canonical.ImageGenRequest{
		Prompt: "robot",
		N:      2,
		Style:  "16:9",
		Size:   "1K",
	})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		t.Fatal()
	}
	params := m["parameters"].(map[string]any)
	if params["sampleCount"].(float64) != 2 {
		t.Fatalf("%v", params)
	}
	if params["aspectRatio"] != "16:9" {
		t.Fatalf("%v", params)
	}
	if ImagePredictPath("imagen-4") != "/models/imagen-4:predict" {
		t.Fatal(ImagePredictPath("imagen-4"))
	}
}

func TestParseImageResponsePredictions(t *testing.T) {
	resp, err := ParseImageResponse([]byte(`{"predictions":[{"bytesBase64Encoded":"YQ==","mimeType":"image/png"}]}`), "imagen")
	if err != nil || len(resp.Images) != 1 || resp.Images[0].B64JSON != "YQ==" {
		t.Fatalf("%+v %v", resp, err)
	}
	if !strings.HasPrefix(ImageGenerateImagesPath("m"), "/models/m:") {
		t.Fatal()
	}
	snake, err := ParseImageResponse([]byte(`{"predictions":[{"bytes_base64_encoded":"Yg==","mime_type":"image/jpeg","url":"https://x"}]}`), "m")
	if err != nil || snake.Images[0].B64JSON != "Yg==" {
		t.Fatalf("%+v %v", snake, err)
	}
	gen, err := ParseImageResponse([]byte(`{"generatedImages":[{"image":{"image_bytes":"Yw==","mimeType":"image/webp"},"url":"https://y"}]}`), "m")
	if err != nil || len(gen.Images) != 1 {
		t.Fatalf("%+v %v", gen, err)
	}
	if !looksLikeAspect("9:16") || looksLikeAspect("vivid") {
		t.Fatal()
	}
}
