package canonical

import "encoding/json"

// ImageMode selects image generation vs edit/variation.
const (
	ImageModeGenerate  = "generate"
	ImageModeEdit      = "edit"
	ImageModeVariation = "variation"
)

// ImageResponseFormat is the desired image payload shape.
const (
	ImageFormatURL    = "url"
	ImageFormatB64    = "b64"
	ImageFormatBase64 = "base64" // Anthropic-gateway synonym for b64
)

// ImageGenRequest is the dialect-neutral image generation/edit request.
type ImageGenRequest struct {
	Model          string
	Prompt         string
	N              int
	Size           string
	Quality        string
	Style          string
	ResponseFormat string // url | b64 | base64
	Mode           string // generate | edit | variation
	// Source images for edit/variation (base64 or URL).
	Images []ImageSource
	Mask   *ImageSource
	Seed   *int64
	// Extra carries vendor-only hints for same-family passthrough.
	Extra map[string]json.RawMessage
}

// GeneratedImage is one output image.
type GeneratedImage struct {
	B64JSON       string
	URL           string
	MediaType     string
	RevisedPrompt string
}

// ImageGenResponse is the dialect-neutral image generation response.
type ImageGenResponse struct {
	ID     string
	Model  string
	Images []GeneratedImage
	Usage  Usage
	// Created is OpenAI's unix timestamp when present.
	Created int64
}
