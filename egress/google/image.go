package google

import (
	"encoding/json"

	"github.com/inja-online/llm-gateway/canonical"
)

// BuildImagePredictRequest builds an Imagen :predict body from canonical.
func BuildImagePredictRequest(req *canonical.ImageGenRequest) ([]byte, error) {
	n := req.N
	if n <= 0 {
		n = 1
	}
	params := map[string]any{
		"sampleCount": n,
	}
	// Style may carry aspect ratio from Google ingress; Size may be 1K/2K.
	if req.Style != "" && looksLikeAspect(req.Style) {
		params["aspectRatio"] = req.Style
	}
	if req.Size != "" {
		params["imageSize"] = req.Size
	}
	out := map[string]any{
		"instances": []map[string]any{
			{"prompt": req.Prompt},
		},
		"parameters": params,
	}
	return json.Marshal(out)
}

// ImagePredictPath is the Imagen predict path (real Generative Language API).
func ImagePredictPath(model string) string {
	return "/models/" + model + ":predict"
}

// ImageGenerateImagesPath is the gateway-facing Google image path alias.
func ImageGenerateImagesPath(model string) string {
	return "/models/" + model + ":generateImages"
}

// ParseImageResponse parses Imagen predict / generateImages responses.
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
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, err
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
	return resp, nil
}

func looksLikeAspect(s string) bool {
	switch s {
	case "1:1", "3:4", "4:3", "9:16", "16:9":
		return true
	default:
		return false
	}
}
