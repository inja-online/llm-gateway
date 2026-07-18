package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/inja-online/llm-gateway/canonical"
)

// ParseVideoCreateRequest parses Anthropic-gateway video create JSON.
func ParseVideoCreateRequest(body []byte) (*canonical.VideoGenRequest, error) {
	var wire struct {
		Model      string  `json:"model"`
		Prompt     string  `json:"prompt"`
		Duration   float64 `json:"duration"`
		Resolution string  `json:"resolution"`
		Aspect     string  `json:"aspect_ratio"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("invalid video request JSON: %w", err)
	}
	if wire.Model == "" {
		return nil, &ValidationError{Msg: "missing or invalid required field: model"}
	}
	return &canonical.VideoGenRequest{
		Model:      wire.Model,
		Prompt:     wire.Prompt,
		Duration:   wire.Duration,
		Resolution: wire.Resolution,
		Aspect:     wire.Aspect,
		Operation:  canonical.VideoOpCreate,
	}, nil
}

// SerializeVideoResponse writes Anthropic-gateway video job response.
func SerializeVideoResponse(resp *canonical.VideoGenResponse) ([]byte, error) {
	out := map[string]any{
		"id":     resp.ID,
		"type":   "video_generation",
		"status": resp.Status,
	}
	if resp.Model != "" {
		out["model"] = resp.Model
	}
	if resp.Progress > 0 {
		out["progress"] = resp.Progress
	}
	if resp.Result != nil {
		r := map[string]any{}
		if resp.Result.URL != "" {
			r["url"] = resp.Result.URL
		}
		if resp.Result.B64 != "" {
			r["b64_json"] = resp.Result.B64
		}
		if resp.Result.MediaType != "" {
			r["media_type"] = resp.Result.MediaType
		}
		if len(r) > 0 {
			out["result"] = r
		}
	}
	if resp.Error != "" {
		out["error"] = map[string]any{"type": "api_error", "message": resp.Error}
	}
	out["usage"] = map[string]any{
		"input_tokens":  resp.Usage.InputTokens,
		"output_tokens": resp.Usage.OutputTokens,
	}
	return json.Marshal(out)
}
