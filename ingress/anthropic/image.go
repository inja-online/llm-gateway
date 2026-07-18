package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/inja-online/llm-gateway/canonical"
)

// ParseImageRequest parses Gateway Media Contract v1 Anthropic image generate/edit.
func ParseImageRequest(body []byte, mode string) (*canonical.ImageGenRequest, error) {
	var wire struct {
		Model          string `json:"model"`
		Prompt         string `json:"prompt"`
		N              int    `json:"n"`
		Size           string `json:"size"`
		Quality        string `json:"quality"`
		Style          string `json:"style"`
		ResponseFormat string `json:"response_format"`
		// edit sources
		Image     string `json:"image"`
		ImageB64  string `json:"image_b64"`
		MediaType string `json:"media_type"`
		Mask      string `json:"mask"`
		Seed      *int64 `json:"seed"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("invalid image request JSON: %w", err)
	}
	if wire.Model == "" {
		return nil, &ValidationError{Msg: "missing or invalid required field: model"}
	}
	if mode == "" {
		mode = canonical.ImageModeGenerate
	}
	if mode == canonical.ImageModeGenerate && wire.Prompt == "" {
		return nil, &ValidationError{Msg: "missing or invalid required field: prompt"}
	}
	n := wire.N
	if n <= 0 {
		n = 1
	}
	rf := wire.ResponseFormat
	switch rf {
	case "", "base64", "b64", "b64_json":
		if rf == "" {
			rf = canonical.ImageFormatBase64
		}
	case "url":
		// ok
	}
	req := &canonical.ImageGenRequest{
		Model:          wire.Model,
		Prompt:         wire.Prompt,
		N:              n,
		Size:           wire.Size,
		Quality:        wire.Quality,
		Style:          wire.Style,
		ResponseFormat: rf,
		Mode:           mode,
		Seed:           wire.Seed,
	}
	img := wire.Image
	if img == "" {
		img = wire.ImageB64
	}
	if img != "" {
		mt := wire.MediaType
		if mt == "" {
			mt = "image/png"
		}
		req.Images = []canonical.ImageSource{{Kind: "base64", Data: img, MediaType: mt}}
	}
	if wire.Mask != "" {
		req.Mask = &canonical.ImageSource{Kind: "base64", Data: wire.Mask, MediaType: "image/png"}
	}
	return req, nil
}

// SerializeImageResponse writes Anthropic-gateway image_generation response.
func SerializeImageResponse(resp *canonical.ImageGenResponse) ([]byte, error) {
	type item struct {
		B64JSON       string `json:"b64_json,omitempty"`
		URL           string `json:"url,omitempty"`
		MediaType     string `json:"media_type,omitempty"`
		RevisedPrompt string `json:"revised_prompt,omitempty"`
	}
	out := struct {
		ID    string `json:"id"`
		Type  string `json:"type"`
		Model string `json:"model"`
		Data  []item `json:"data"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}{
		ID:    resp.ID,
		Type:  "image_generation",
		Model: resp.Model,
		Data:  make([]item, 0, len(resp.Images)),
	}
	if out.ID == "" {
		out.ID = "img_gateway"
	}
	for _, img := range resp.Images {
		mt := img.MediaType
		if mt == "" {
			mt = "image/png"
		}
		out.Data = append(out.Data, item{
			B64JSON:       img.B64JSON,
			URL:           img.URL,
			MediaType:     mt,
			RevisedPrompt: img.RevisedPrompt,
		})
	}
	out.Usage.InputTokens = resp.Usage.InputTokens
	out.Usage.OutputTokens = resp.Usage.OutputTokens
	return json.Marshal(out)
}
