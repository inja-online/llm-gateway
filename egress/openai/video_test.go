package openai

import (
	"encoding/json"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestBuildParseVideo(t *testing.T) {
	body, err := BuildVideoCreateRequest(&canonical.VideoGenRequest{
		Prompt: "rain", Duration: 4, Resolution: "720p", Aspect: "16:9",
	}, "sora")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal(body, &m)
	if m["model"] != "sora" || m["prompt"] != "rain" {
		t.Fatalf("%v", m)
	}
	if VideoCreatePath() != "/videos" || VideoGetPath("id1") != "/videos/id1" {
		t.Fatal()
	}
	resp, err := ParseVideoResponse([]byte(`{"id":"v1","status":"completed","url":"https://x","error":null}`))
	if err != nil || resp.Status != canonical.VideoStatusCompleted || resp.Result.URL == "" {
		t.Fatalf("%+v %v", resp, err)
	}
	fail, _ := ParseVideoResponse([]byte(`{"id":"v2","status":"failed","error":{"message":"bad"}}`))
	if fail.Error != "bad" {
		t.Fatalf("%+v", fail)
	}
	for in, want := range map[string]string{
		"queued": canonical.VideoStatusQueued, "pending": canonical.VideoStatusQueued,
		"in_progress": canonical.VideoStatusProcessing, "running": canonical.VideoStatusProcessing,
		"processing": canonical.VideoStatusProcessing,
		"completed": canonical.VideoStatusCompleted, "succeeded": canonical.VideoStatusCompleted,
		"failed": canonical.VideoStatusFailed, "error": canonical.VideoStatusFailed,
		"": canonical.VideoStatusProcessing, "custom": "custom",
	} {
		if mapVideoStatus(in) != want {
			t.Fatalf("%q -> %q want %q", in, mapVideoStatus(in), want)
		}
	}
}
