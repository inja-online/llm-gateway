package google

import (
	"encoding/json"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestBuildParseGoogleVideo(t *testing.T) {
	body, err := BuildVideoCreateRequest(&canonical.VideoGenRequest{
		Prompt: "waves", Duration: 5, Aspect: "16:9", Resolution: "720p",
	})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal(body, &m)
	if _, ok := m["instances"]; !ok {
		t.Fatalf("%v", m)
	}
	if VideoGeneratePath("veo") != "/models/veo:generateVideos" {
		t.Fatal()
	}
	if VideoPredictLongRunningPath("veo") != "/models/veo:predictLongRunning" {
		t.Fatal()
	}
	if VideoPollPath("op1") != "/videos/op1" {
		t.Fatal(VideoPollPath("op1"))
	}
	if VideoPollPath("operations/op1") != "/operations/op1" {
		t.Fatal()
	}
	resp, err := ParseVideoResponse([]byte(`{
		"name":"operations/z",
		"done":true,
		"response":{"generated_videos":[{"video":{"uri":"https://x"}}]}
	}`))
	if err != nil || resp.Result == nil {
		t.Fatalf("%+v %v", resp, err)
	}
	for in, want := range map[string]string{
		"queued": canonical.VideoStatusQueued, "pending": canonical.VideoStatusQueued,
		"processing": canonical.VideoStatusProcessing, "RUNNING": canonical.VideoStatusProcessing,
		"completed": canonical.VideoStatusCompleted, "done": canonical.VideoStatusCompleted,
		"failed": canonical.VideoStatusFailed, "ERROR": canonical.VideoStatusFailed,
		"other": "other",
	} {
		if mapGoogleVideoStatus(in) != want {
			t.Fatalf("%q -> %q want %q", in, mapGoogleVideoStatus(in), want)
		}
	}
	// more parse branches
	proc, _ := ParseVideoResponse([]byte(`{"id":"v1","status":"processing","url":"https://u"}`))
	if proc.Status != canonical.VideoStatusProcessing || proc.Result == nil {
		t.Fatalf("%+v", proc)
	}
	snake, _ := ParseVideoResponse([]byte(`{"name":"n","done":true,"response":{"generatedVideos":[{"video":{"url":"https://a","bytesBase64Encoded":"YQ=="}}]}}`))
	if snake.Result == nil || snake.Result.URL != "https://a" {
		t.Fatalf("%+v", snake)
	}
}
