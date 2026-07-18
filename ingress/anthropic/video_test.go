package anthropic

import (
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestParseSerializeAnthropicVideo(t *testing.T) {
	req, err := ParseVideoCreateRequest([]byte(`{"model":"sora","prompt":"rain","duration":3,"aspect_ratio":"16:9"}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Model != "sora" || req.Aspect != "16:9" {
		t.Fatalf("%+v", req)
	}
	out, err := SerializeVideoResponse(&canonical.VideoGenResponse{
		ID:     "video_1",
		Status: canonical.VideoStatusProcessing,
		Model:  "sora",
		Result: &canonical.VideoResult{URL: "https://x/v.mp4", MediaType: "video/mp4"},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{`"type":"video_generation"`, `"status":"processing"`, `"url":"https://x/v.mp4"`} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %s in %s", want, s)
		}
	}
	fail, _ := SerializeVideoResponse(&canonical.VideoGenResponse{
		ID: "v", Status: canonical.VideoStatusFailed, Error: "nope",
	})
	if !strings.Contains(string(fail), "nope") {
		t.Fatalf("%s", fail)
	}
	if _, err := ParseVideoCreateRequest([]byte(`{"prompt":"x"}`)); err == nil {
		t.Fatal()
	}
}
