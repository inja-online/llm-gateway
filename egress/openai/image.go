package openai

import (
	"encoding/json"

	"github.com/inja-online/llm-gateway/canonical"
)

// BuildImageRequest converts canonical image gen to OpenAI Images JSON.
func BuildImageRequest(req *canonical.ImageGenRequest, model string) ([]byte, error) {
	out := map[string]any{
		"model":  model,
		"prompt": req.Prompt,
	}
	n := req.N
	if n <= 0 {
		n = 1
	}
	out["n"] = n
	if req.Size != "" {
		out["size"] = req.Size
	}
	if req.Quality != "" {
		out["quality"] = req.Quality
	}
	if req.Style != "" && req.Mode == canonical.ImageModeGenerate {
		// OpenAI style is vivid/natural; skip aspect ratios from Google.
		switch req.Style {
		case "vivid", "natural":
			out["style"] = req.Style
		}
	}
	rf := req.ResponseFormat
	switch rf {
	case canonical.ImageFormatB64, canonical.ImageFormatBase64, "b64_json":
		out["response_format"] = "b64_json"
	case canonical.ImageFormatURL:
		out["response_format"] = "url"
	case "":
		// default upstream
	default:
		out["response_format"] = rf
	}
	if req.Mode == canonical.ImageModeEdit || req.Mode == canonical.ImageModeVariation {
		if len(req.Images) > 0 {
			out["image"] = req.Images[0].Data
		}
		if req.Mask != nil {
			out["mask"] = req.Mask.Data
		}
	}
	return json.Marshal(out)
}

// ImagePath returns the OpenAI Images API path for the mode.
func ImagePath(mode string) string {
	switch mode {
	case canonical.ImageModeEdit:
		return "/images/edits"
	case canonical.ImageModeVariation:
		return "/images/variations"
	default:
		return "/images/generations"
	}
}

// ParseImageResponse parses OpenAI Images API JSON into canonical form.
func ParseImageResponse(body []byte, model string) (*canonical.ImageGenResponse, error) {
	var wire struct {
		Created int64 `json:"created"`
		Data    []struct {
			URL           string `json:"url"`
			B64JSON       string `json:"b64_json"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, err
	}
	resp := &canonical.ImageGenResponse{
		Model:   model,
		Created: wire.Created,
	}
	for _, d := range wire.Data {
		resp.Images = append(resp.Images, canonical.GeneratedImage{
			URL:           d.URL,
			B64JSON:       d.B64JSON,
			MediaType:     "image/png",
			RevisedPrompt: d.RevisedPrompt,
		})
	}
	return resp, nil
}
