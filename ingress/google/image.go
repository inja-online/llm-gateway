package google

import (
	"encoding/json"
	"fmt"

	"github.com/inja-online/llm-gateway/canonical"
)

// ParseImageRequest parses Google-shaped image generate bodies.
// Accepts gateway generateImages shape and Imagen :predict instances/parameters.
func ParseImageRequest(body []byte, pathModel string) (*canonical.ImageGenRequest, error) {
	// Gateway generateImages shape first.
	var gen struct {
		Model            string `json:"model"`
		Prompt           string `json:"prompt"`
		NumberOfImages   int    `json:"numberOfImages"`
		NumberOfImagesS  int    `json:"number_of_images"`
		AspectRatio      string `json:"aspectRatio"`
		AspectRatioS     string `json:"aspect_ratio"`
		ImageSize        string `json:"imageSize"`
		ImageSizeS       string `json:"image_size"`
	}
	_ = json.Unmarshal(body, &gen)

	// Imagen predict shape.
	var pred struct {
		Instances []struct {
			Prompt string `json:"prompt"`
		} `json:"instances"`
		Parameters *struct {
			SampleCount  int    `json:"sampleCount"`
			SampleCountS int    `json:"sample_count"`
			AspectRatio  string `json:"aspectRatio"`
			AspectRatioS string `json:"aspect_ratio"`
			ImageSize    string `json:"imageSize"`
			ImageSizeS   string `json:"image_size"`
		} `json:"parameters"`
	}
	_ = json.Unmarshal(body, &pred)

	model := gen.Model
	if model == "" {
		model = pathModel
	}
	if model == "" {
		return nil, &ValidationError{Msg: "missing model"}
	}

	prompt := gen.Prompt
	if prompt == "" && len(pred.Instances) > 0 {
		prompt = pred.Instances[0].Prompt
	}
	if prompt == "" {
		return nil, &ValidationError{Msg: "missing prompt"}
	}

	n := gen.NumberOfImages
	if n <= 0 {
		n = gen.NumberOfImagesS
	}
	if n <= 0 && pred.Parameters != nil {
		n = pred.Parameters.SampleCount
		if n <= 0 {
			n = pred.Parameters.SampleCountS
		}
	}
	if n <= 0 {
		n = 1
	}

	aspect := gen.AspectRatio
	if aspect == "" {
		aspect = gen.AspectRatioS
	}
	size := gen.ImageSize
	if size == "" {
		size = gen.ImageSizeS
	}
	if pred.Parameters != nil {
		if aspect == "" {
			aspect = pred.Parameters.AspectRatio
			if aspect == "" {
				aspect = pred.Parameters.AspectRatioS
			}
		}
		if size == "" {
			size = pred.Parameters.ImageSize
			if size == "" {
				size = pred.Parameters.ImageSizeS
			}
		}
	}

	return &canonical.ImageGenRequest{
		Model:          model,
		Prompt:         prompt,
		N:              n,
		Size:           size,
		Style:          aspect, // aspect stored in Style for Google mapping
		ResponseFormat: canonical.ImageFormatB64,
		Mode:           canonical.ImageModeGenerate,
	}, nil
}

// SerializeImageResponse writes a Google-shaped image response (predictions).
func SerializeImageResponse(resp *canonical.ImageGenResponse) ([]byte, error) {
	type pred struct {
		BytesBase64Encoded string `json:"bytesBase64Encoded,omitempty"`
		MIMEType           string `json:"mimeType,omitempty"`
		// also emit generateImages-style nested form via parallel fields when URL only
		URL string `json:"url,omitempty"`
	}
	out := struct {
		Predictions []pred `json:"predictions"`
	}{Predictions: make([]pred, 0, len(resp.Images))}
	for _, img := range resp.Images {
		mt := img.MediaType
		if mt == "" {
			mt = "image/png"
		}
		out.Predictions = append(out.Predictions, pred{
			BytesBase64Encoded: img.B64JSON,
			MIMEType:           mt,
			URL:                img.URL,
		})
	}
	return json.Marshal(out)
}

// ParseImageResponse parses Google Imagen predict / generateImages responses.
func ParseImageResponse(body []byte, model string) (*canonical.ImageGenResponse, error) {
	var wire struct {
		Predictions []struct {
			BytesBase64Encoded string `json:"bytesBase64Encoded"`
			BytesB64Snake      string `json:"bytes_base64_encoded"`
			MIMEType           string `json:"mimeType"`
			MIMETypeSnake      string `json:"mime_type"`
			URL                string `json:"url"`
		} `json:"predictions"`
		GeneratedImages []struct {
			Image *struct {
				ImageBytes string `json:"imageBytes"`
				ImageB64   string `json:"image_bytes"`
				MIMEType   string `json:"mimeType"`
			} `json:"image"`
			URL string `json:"url"`
		} `json:"generatedImages"`
		GeneratedImagesSnake []struct {
			Image *struct {
				ImageBytes string `json:"imageBytes"`
				ImageB64   string `json:"image_bytes"`
			} `json:"image"`
		} `json:"generated_images"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("invalid google image response: %w", err)
	}
	resp := &canonical.ImageGenResponse{Model: model}
	for _, p := range wire.Predictions {
		b64 := p.BytesBase64Encoded
		if b64 == "" {
			b64 = p.BytesB64Snake
		}
		mt := p.MIMEType
		if mt == "" {
			mt = p.MIMETypeSnake
		}
		if mt == "" {
			mt = "image/png"
		}
		resp.Images = append(resp.Images, canonical.GeneratedImage{
			B64JSON: b64, URL: p.URL, MediaType: mt,
		})
	}
	for _, g := range wire.GeneratedImages {
		img := canonical.GeneratedImage{URL: g.URL, MediaType: "image/png"}
		if g.Image != nil {
			img.B64JSON = g.Image.ImageBytes
			if img.B64JSON == "" {
				img.B64JSON = g.Image.ImageB64
			}
			if g.Image.MIMEType != "" {
				img.MediaType = g.Image.MIMEType
			}
		}
		resp.Images = append(resp.Images, img)
	}
	for _, g := range wire.GeneratedImagesSnake {
		img := canonical.GeneratedImage{MediaType: "image/png"}
		if g.Image != nil {
			img.B64JSON = g.Image.ImageBytes
			if img.B64JSON == "" {
				img.B64JSON = g.Image.ImageB64
			}
		}
		resp.Images = append(resp.Images, img)
	}
	return resp, nil
}
