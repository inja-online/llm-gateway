package openai

import (
	"encoding/json"
	"fmt"

	"github.com/inja-online/llm-gateway/canonical"
)

// ParseVideoCreateRequest parses OpenAI-style video create JSON.
func ParseVideoCreateRequest(body []byte) (*canonical.VideoGenRequest, error) {
	var wire struct {
		Model      string  `json:"model"`
		Prompt     string  `json:"prompt"`
		Duration   float64 `json:"duration"`
		Resolution string  `json:"resolution"`
		Size       string  `json:"size"`
		Aspect     string  `json:"aspect_ratio"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("invalid video request JSON: %w", err)
	}
	if wire.Model == "" {
		return nil, &ValidationError{Msg: "missing or invalid required field: model"}
	}
	res := wire.Resolution
	if res == "" {
		res = wire.Size
	}
	return &canonical.VideoGenRequest{
		Model:      wire.Model,
		Prompt:     wire.Prompt,
		Duration:   wire.Duration,
		Resolution: res,
		Aspect:     wire.Aspect,
		Operation:  canonical.VideoOpCreate,
	}, nil
}

// SerializeVideoResponse writes an OpenAI-style video job response.
func SerializeVideoResponse(resp *canonical.VideoGenResponse) ([]byte, error) {
	out := map[string]any{
		"id":     resp.ID,
		"status": resp.Status,
	}
	if resp.Model != "" {
		out["model"] = resp.Model
	}
	if resp.Progress > 0 {
		out["progress"] = resp.Progress
	}
	if resp.Result != nil {
		if resp.Result.URL != "" {
			out["url"] = resp.Result.URL
		}
		if resp.Result.B64 != "" {
			out["b64_json"] = resp.Result.B64
		}
	}
	if resp.Error != "" {
		out["error"] = map[string]any{"message": resp.Error}
	}
	return json.Marshal(out)
}

// ParseVideoResponse parses an OpenAI-style video job response.
func ParseVideoResponse(body []byte) (*canonical.VideoGenResponse, error) {
	var wire struct {
		ID       string  `json:"id"`
		Status   string  `json:"status"`
		Model    string  `json:"model"`
		Progress float64 `json:"progress"`
		URL      string  `json:"url"`
		B64JSON  string  `json:"b64_json"`
		Error    *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("invalid video response JSON: %w", err)
	}
	resp := &canonical.VideoGenResponse{
		ID:       wire.ID,
		Status:   normalizeVideoStatus(wire.Status),
		Model:    wire.Model,
		Progress: wire.Progress,
	}
	if wire.URL != "" || wire.B64JSON != "" {
		resp.Result = &canonical.VideoResult{URL: wire.URL, B64: wire.B64JSON, MediaType: "video/mp4"}
	}
	if wire.Error != nil {
		resp.Error = wire.Error.Message
	}
	return resp, nil
}

func normalizeVideoStatus(s string) string {
	switch s {
	case "queued", "pending":
		return canonical.VideoStatusQueued
	case "in_progress", "running", "processing":
		return canonical.VideoStatusProcessing
	case "completed", "succeeded", "done":
		return canonical.VideoStatusCompleted
	case "failed", "error", "cancelled", "canceled":
		return canonical.VideoStatusFailed
	default:
		if s == "" {
			return canonical.VideoStatusProcessing
		}
		return s
	}
}
