package google

import (
	"encoding/json"
	"strings"

	"github.com/inja-online/llm-gateway/canonical"
)

// BuildVideoCreateRequest builds a Google video generate body from canonical.
// Uses instances/parameters (predictLongRunning-compatible).
func BuildVideoCreateRequest(req *canonical.VideoGenRequest) ([]byte, error) {
	params := map[string]any{}
	if req.Duration > 0 {
		params["durationSeconds"] = req.Duration
	}
	if req.Aspect != "" {
		params["aspectRatio"] = req.Aspect
	}
	if req.Resolution != "" {
		params["resolution"] = req.Resolution
	}
	out := map[string]any{
		"instances": []map[string]any{
			{"prompt": req.Prompt},
		},
	}
	if len(params) > 0 {
		out["parameters"] = params
	}
	return json.Marshal(out)
}

// VideoGeneratePath returns the create path. Prefer generateVideos (gateway contract);
// upstream fakes may implement either generateVideos or predictLongRunning.
func VideoGeneratePath(model string) string {
	return "/models/" + model + ":generateVideos"
}

// VideoPredictLongRunningPath is the alternate LRO create path.
func VideoPredictLongRunningPath(model string) string {
	return "/models/" + model + ":predictLongRunning"
}

// VideoPollPath maps a job name/id to a poll URL under base_url (…/v1beta).
// Accepts bare ids, "operations/…", or "videos/…".
func VideoPollPath(name string) string {
	name = strings.TrimPrefix(name, "/")
	if strings.HasPrefix(name, "operations/") || strings.HasPrefix(name, "videos/") {
		return "/" + name
	}
	// Gateway contract: GET /v1beta/videos/{name}
	return "/videos/" + name
}

// ParseVideoResponse parses Google LRO create/poll JSON.
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
					URI      string `json:"uri"`
					URL      string `json:"url"`
					BytesB64 string `json:"bytesBase64Encoded"`
				} `json:"video"`
			} `json:"generatedVideos"`
			GeneratedVideosSnake []struct {
				Video *struct {
					URI string `json:"uri"`
				} `json:"video"`
			} `json:"generated_videos"`
		} `json:"response"`
		ID     string `json:"id"`
		Status string `json:"status"`
		URL    string `json:"url"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, err
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
		resp.Status = mapGoogleVideoStatus(wire.Status)
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

func mapGoogleVideoStatus(s string) string {
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
