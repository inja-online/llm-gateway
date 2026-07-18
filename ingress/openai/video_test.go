package openai

import (
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestParseSerializeVideo(t *testing.T) {
	req, err := ParseVideoCreateRequest([]byte(`{"model":"sora","prompt":"rain","duration":4,"size":"720p"}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Model != "sora" || req.Duration != 4 || req.Resolution != "720p" {
		t.Fatalf("%+v", req)
	}
	out, err := SerializeVideoResponse(&canonical.VideoGenResponse{
		ID:       "video_1",
		Status:   canonical.VideoStatusCompleted,
		Model:    "sora",
		Progress: 100,
		Result:   &canonical.VideoResult{URL: "https://x/v.mp4"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "video_1") || !strings.Contains(string(out), "https://x/v.mp4") {
		t.Fatalf("%s", out)
	}
	back, err := ParseVideoResponse(out)
	if err != nil || back.ID != "video_1" {
		t.Fatalf("%+v %v", back, err)
	}
	errOut, _ := SerializeVideoResponse(&canonical.VideoGenResponse{
		ID: "v2", Status: canonical.VideoStatusFailed, Error: "bad",
	})
	if !strings.Contains(string(errOut), "bad") {
		t.Fatalf("%s", errOut)
	}
	for in, want := range map[string]string{
		"queued": canonical.VideoStatusQueued, "pending": canonical.VideoStatusQueued,
		"in_progress": canonical.VideoStatusProcessing, "running": canonical.VideoStatusProcessing,
		"processing": canonical.VideoStatusProcessing,
		"completed":  canonical.VideoStatusCompleted, "succeeded": canonical.VideoStatusCompleted, "done": canonical.VideoStatusCompleted,
		"failed": canonical.VideoStatusFailed, "error": canonical.VideoStatusFailed, "cancelled": canonical.VideoStatusFailed,
		"": canonical.VideoStatusProcessing, "x": "x",
	} {
		if normalizeVideoStatus(in) != want {
			t.Fatalf("%q", in)
		}
	}
	if _, err := ParseVideoCreateRequest([]byte(`{"prompt":"x"}`)); err == nil {
		t.Fatal("missing model")
	}
	withB64, _ := ParseVideoResponse([]byte(`{"id":"v","status":"completed","b64_json":"YQ==","error":{"message":"e"}}`))
	if withB64.Result == nil || withB64.Result.B64 != "YQ==" || withB64.Error != "e" {
		t.Fatalf("%+v", withB64)
	}
}
