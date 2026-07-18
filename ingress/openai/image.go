package openai

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/inja-online/llm-gateway/canonical"
)

// ParseImageRequest parses an OpenAI Images API JSON body into canonical form.
func ParseImageRequest(body []byte, mode string) (*canonical.ImageGenRequest, error) {
	var wire struct {
		Model          string `json:"model"`
		Prompt         string `json:"prompt"`
		N              int    `json:"n"`
		Size           string `json:"size"`
		Quality        string `json:"quality"`
		Style          string `json:"style"`
		ResponseFormat string `json:"response_format"`
		// edit: image may be string (b64) in JSON path; multipart handled elsewhere
		Image string `json:"image"`
		Mask  string `json:"mask"`
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
	if rf == "b64_json" {
		rf = canonical.ImageFormatB64
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
	}
	if wire.Image != "" {
		req.Images = []canonical.ImageSource{{Kind: "base64", Data: wire.Image, MediaType: "image/png"}}
	}
	if wire.Mask != "" {
		req.Mask = &canonical.ImageSource{Kind: "base64", Data: wire.Mask, MediaType: "image/png"}
	}
	return req, nil
}

// SerializeImageResponse writes an OpenAI Images API response body.
func SerializeImageResponse(resp *canonical.ImageGenResponse) ([]byte, error) {
	type item struct {
		URL           string `json:"url,omitempty"`
		B64JSON       string `json:"b64_json,omitempty"`
		RevisedPrompt string `json:"revised_prompt,omitempty"`
	}
	out := struct {
		Created int64  `json:"created"`
		Data    []item `json:"data"`
	}{
		Created: resp.Created,
		Data:    make([]item, 0, len(resp.Images)),
	}
	if out.Created == 0 {
		out.Created = time.Now().Unix()
	}
	for _, img := range resp.Images {
		out.Data = append(out.Data, item{
			URL:           img.URL,
			B64JSON:       img.B64JSON,
			RevisedPrompt: img.RevisedPrompt,
		})
	}
	return json.Marshal(out)
}

// ParseImageResponse parses an OpenAI Images API response into canonical form.
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
		return nil, fmt.Errorf("invalid image response JSON: %w", err)
	}
	resp := &canonical.ImageGenResponse{
		Model:   model,
		Created: wire.Created,
		Images:  make([]canonical.GeneratedImage, 0, len(wire.Data)),
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
