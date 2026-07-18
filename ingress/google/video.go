package google

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/inja-online/llm-gateway/canonical"
)

// ParseVideoCreateRequest parses Google generateVideos / predictLongRunning bodies.
func ParseVideoCreateRequest(body []byte, pathModel string) (*canonical.VideoGenRequest, error) {
	var gen struct {
		Model      string  `json:"model"`
		Prompt     string  `json:"prompt"`
		Duration   float64 `json:"durationSeconds"`
		DurationS  float64 `json:"duration_seconds"`
		Aspect     string  `json:"aspectRatio"`
		AspectS    string  `json:"aspect_ratio"`
		Resolution string  `json:"resolution"`
	}
	_ = json.Unmarshal(body, &gen)

	var pred struct {
		Instances []struct {
			Prompt string `json:"prompt"`
		} `json:"instances"`
		Parameters *struct {
			DurationSeconds float64 `json:"durationSeconds"`
			AspectRatio     string  `json:"aspectRatio"`
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
	dur := gen.Duration
	if dur <= 0 {
		dur = gen.DurationS
	}
	if dur <= 0 && pred.Parameters != nil {
		dur = pred.Parameters.DurationSeconds
	}
	aspect := gen.Aspect
	if aspect == "" {
		aspect = gen.AspectS
	}
	if aspect == "" && pred.Parameters != nil {
		aspect = pred.Parameters.AspectRatio
	}
	return &canonical.VideoGenRequest{
		Model:      model,
		Prompt:     prompt,
		Duration:   dur,
		Aspect:     aspect,
		Resolution: gen.Resolution,
		Operation:  canonical.VideoOpCreate,
	}, nil
}

// SerializeVideoCreateResponse writes a Google LRO-style create response.
func SerializeVideoCreateResponse(resp *canonical.VideoGenResponse) ([]byte, error) {
	name := resp.ID
	if name != "" && !strings.Contains(name, "/") {
		name = "operations/" + name
	}
	out := map[string]any{
		"name": name,
		"done": resp.Status == canonical.VideoStatusCompleted,
	}
	if resp.Status == canonical.VideoStatusFailed && resp.Error != "" {
		out["error"] = map[string]any{"message": resp.Error}
	}
	if resp.Status == canonical.VideoStatusCompleted && resp.Result != nil {
		out["response"] = map[string]any{
			"generatedVideos": []map[string]any{{
				"video": map[string]any{
					"uri": resp.Result.URL,
				},
			}},
		}
		if resp.Result.URL != "" {
			// also top-level convenience
		}
	}
	if resp.Model != "" {
		out["metadata"] = map[string]any{"model": resp.Model}
	}
	return json.Marshal(out)
}

// SerializeVideoPollResponse writes Google operation poll shape.
func SerializeVideoPollResponse(resp *canonical.VideoGenResponse) ([]byte, error) {
	return SerializeVideoCreateResponse(resp)
}

// ParseVideoResponse parses Google LRO create/poll responses.
func ParseVideoResponse(body []byte) (*canonical.VideoGenResponse, error) {
	var wire struct {
		Name     string `json:"name"`
		Done     bool   `json:"done"`
		Metadata *struct {
			Model string `json:"model"`
		} `json:"metadata"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Response *struct {
			GeneratedVideos []struct {
				Video *struct {
					URI     string `json:"uri"`
					URL     string `json:"url"`
					BytesB64 string `json:"bytesBase64Encoded"`
				} `json:"video"`
			} `json:"generatedVideos"`
			GeneratedVideosSnake []struct {
				Video *struct {
					URI string `json:"uri"`
				} `json:"video"`
			} `json:"generated_videos"`
		} `json:"response"`
		// OpenAI-compat-ish fields sometimes appear via proxies
		ID     string `json:"id"`
		Status string `json:"status"`
		URL    string `json:"url"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("invalid google video response: %w", err)
	}
	resp := &canonical.VideoGenResponse{}
	resp.ID = wire.Name
	if resp.ID == "" {
		resp.ID = wire.ID
	}
	if wire.Metadata != nil {
		resp.Model = wire.Metadata.Model
	}
	switch {
	case wire.Error != nil:
		resp.Status = canonical.VideoStatusFailed
		resp.Error = wire.Error.Message
	case wire.Done:
		resp.Status = canonical.VideoStatusCompleted
	case wire.Status != "":
		resp.Status = normalizeGoogleVideoStatus(wire.Status)
	default:
		resp.Status = canonical.VideoStatusProcessing
	}
	if wire.Response != nil {
		for _, g := range wire.Response.GeneratedVideos {
			if g.Video != nil {
				u := g.Video.URI
				if u == "" {
					u = g.Video.URL
				}
				resp.Result = &canonical.VideoResult{URL: u, B64: g.Video.BytesB64, MediaType: "video/mp4"}
				break
			}
		}
		if resp.Result == nil {
			for _, g := range wire.Response.GeneratedVideosSnake {
				if g.Video != nil {
					resp.Result = &canonical.VideoResult{URL: g.Video.URI, MediaType: "video/mp4"}
					break
				}
			}
		}
	}
	if wire.URL != "" && resp.Result == nil {
		resp.Result = &canonical.VideoResult{URL: wire.URL, MediaType: "video/mp4"}
	}
	return resp, nil
}

func normalizeGoogleVideoStatus(s string) string {
	switch strings.ToLower(s) {
	case "queued", "pending":
		return canonical.VideoStatusQueued
	case "processing", "running", "in_progress":
		return canonical.VideoStatusProcessing
	case "completed", "succeeded", "done":
		return canonical.VideoStatusCompleted
	case "failed", "error":
		return canonical.VideoStatusFailed
	default:
		return s
	}
}
