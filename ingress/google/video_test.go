package google

import (
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestParseVideoCreateRequest(t *testing.T) {
	req, err := ParseVideoCreateRequest([]byte(`{
		"prompt":"waterfall",
		"durationSeconds":6,
		"aspectRatio":"16:9"
	}`), "veo-3")
	if err != nil {
		t.Fatal(err)
	}
	if req.Model != "veo-3" || req.Duration != 6 || req.Aspect != "16:9" {
		t.Fatalf("%+v", req)
	}
	req2, err := ParseVideoCreateRequest([]byte(`{
		"instances":[{"prompt":"rain"}],
		"parameters":{"durationSeconds":4,"aspectRatio":"9:16"}
	}`), "veo")
	if err != nil || req2.Prompt != "rain" || req2.Duration != 4 {
		t.Fatalf("%+v %v", req2, err)
	}
}

func TestSerializeVideoResponses(t *testing.T) {
	out, err := SerializeVideoCreateResponse(&canonical.VideoGenResponse{
		ID:     "op1",
		Status: canonical.VideoStatusProcessing,
		Model:  "veo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "operations/op1") {
		t.Fatalf("%s", out)
	}
	done, _ := SerializeVideoPollResponse(&canonical.VideoGenResponse{
		ID:     "operations/op2",
		Status: canonical.VideoStatusCompleted,
		Result: &canonical.VideoResult{URL: "https://x/v.mp4"},
	})
	if !strings.Contains(string(done), `"done":true`) {
		t.Fatalf("%s", done)
	}
	fail, _ := SerializeVideoCreateResponse(&canonical.VideoGenResponse{
		ID:     "op3",
		Status: canonical.VideoStatusFailed,
		Error:  "boom",
	})
	if !strings.Contains(string(fail), "boom") {
		t.Fatalf("%s", fail)
	}
}

func TestParseVideoResponseLRO(t *testing.T) {
	resp, err := ParseVideoResponse([]byte(`{
		"name":"operations/abc",
		"done":true,
		"metadata":{"model":"veo"},
		"response":{"generatedVideos":[{"video":{"uri":"https://x/v.mp4"}}]}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != canonical.VideoStatusCompleted || resp.Result == nil || resp.Result.URL == "" {
		t.Fatalf("%+v", resp)
	}
	fail, _ := ParseVideoResponse([]byte(`{"name":"operations/x","error":{"message":"nope"}}`))
	if fail.Status != canonical.VideoStatusFailed || fail.Error != "nope" {
		t.Fatalf("%+v", fail)
	}
	proc, _ := ParseVideoResponse([]byte(`{"name":"operations/y","done":false}`))
	if proc.Status != canonical.VideoStatusProcessing {
		t.Fatalf("%+v", proc)
	}
	for in, want := range map[string]string{
		"queued": canonical.VideoStatusQueued, "pending": canonical.VideoStatusQueued,
		"processing": canonical.VideoStatusProcessing, "running": canonical.VideoStatusProcessing,
		"completed": canonical.VideoStatusCompleted, "SUCCEEDED": canonical.VideoStatusCompleted,
		"failed": canonical.VideoStatusFailed, "ERROR": canonical.VideoStatusFailed,
		"zzz": "zzz",
	} {
		if normalizeGoogleVideoStatus(in) != want {
			t.Fatalf("%q", in)
		}
	}
	oaish, _ := ParseVideoResponse([]byte(`{"id":"v","status":"completed","url":"https://u"}`))
	if oaish.Result == nil || oaish.Result.URL != "https://u" {
		t.Fatalf("%+v", oaish)
	}
}
