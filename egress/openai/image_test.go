package openai

import (
	"encoding/json"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestBuildImageRequest(t *testing.T) {
	body, err := BuildImageRequest(&canonical.ImageGenRequest{
		Prompt:         "hi",
		N:              1,
		Size:           "1024x1024",
		ResponseFormat: canonical.ImageFormatBase64,
		Mode:           canonical.ImageModeGenerate,
		Style:          "vivid",
		Quality:        "hd",
	}, "dall-e-3")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal(body, &m)
	if m["model"] != "dall-e-3" || m["response_format"] != "b64_json" {
		t.Fatalf("%v", m)
	}
	if m["style"] != "vivid" || m["quality"] != "hd" {
		t.Fatalf("%v", m)
	}
	if ImagePath(canonical.ImageModeEdit) != "/images/edits" {
		t.Fatal()
	}
	if ImagePath(canonical.ImageModeVariation) != "/images/variations" {
		t.Fatal()
	}
	if ImagePath(canonical.ImageModeGenerate) != "/images/generations" {
		t.Fatal()
	}
	if ImagePath("") != "/images/generations" {
		t.Fatal()
	}
	edit, _ := BuildImageRequest(&canonical.ImageGenRequest{
		Mode:   canonical.ImageModeEdit,
		Prompt: "hat",
		Images: []canonical.ImageSource{{Data: "YQ=="}},
		Mask:   &canonical.ImageSource{Data: "Yg=="},
	}, "dall-e-2")
	var em map[string]any
	json.Unmarshal(edit, &em)
	if em["image"] != "YQ==" || em["mask"] != "Yg==" {
		t.Fatalf("%v", em)
	}
	urlBody, _ := BuildImageRequest(&canonical.ImageGenRequest{
		Prompt: "x", ResponseFormat: canonical.ImageFormatURL,
	}, "dall-e-3")
	var um map[string]any
	json.Unmarshal(urlBody, &um)
	if um["response_format"] != "url" {
		t.Fatalf("%v", um)
	}
}

func TestParseImageResponse(t *testing.T) {
	resp, err := ParseImageResponse([]byte(`{"created":9,"data":[{"b64_json":"YQ==","revised_prompt":"c"},{"url":"https://x"}]}`), "dall-e-3")
	if err != nil || len(resp.Images) != 2 {
		t.Fatalf("%+v %v", resp, err)
	}
	if resp.Created != 9 || resp.Images[0].RevisedPrompt != "c" {
		t.Fatalf("%+v", resp)
	}
}
