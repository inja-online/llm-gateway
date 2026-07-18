package openai

import (
	"encoding/json"

	"github.com/inja-online/llm-gateway/canonical"
)

// BuildVideoCreateRequest converts canonical video create to OpenAI-style JSON.
func BuildVideoCreateRequest(req *canonical.VideoGenRequest, model string) ([]byte, error) {
	out := map[string]any{
		"model":  model,
		"prompt": req.Prompt,
	}
	if req.Duration > 0 {
		out["duration"] = req.Duration
	}
	if req.Resolution != "" {
		out["size"] = req.Resolution
	}
	if req.Aspect != "" {
		out["aspect_ratio"] = req.Aspect
	}
	return json.Marshal(out)
}

// VideoCreatePath is the OpenAI videos create path.
func VideoCreatePath() string { return "/videos" }

// VideoGetPath is the OpenAI videos poll path.
func VideoGetPath(id string) string { return "/videos/" + id }

// ParseVideoResponse parses OpenAI-style video job JSON.
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
		return nil, err
	}
	resp := &canonical.VideoGenResponse{
		ID:       wire.ID,
		Status:   mapVideoStatus(wire.Status),
		Model:    wire.Model,
		Progress: wire.Progress,
	}
	if wire.URL != "" || wire.B64JSON != "" {
		resp.Result = &canonical.VideoResult{URL: wire.URL, B64: wire.B64JSON, MediaType: "video/mp4"}
	}
	if wire.Error != nil {
		resp.Error = wire.Error.Message
		if resp.Status == "" {
			resp.Status = canonical.VideoStatusFailed
		}
	}
	return resp, nil
}

func mapVideoStatus(s string) string {
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
