package proxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
	antingress "github.com/inja-online/llm-gateway/ingress/anthropic"
	googleingress "github.com/inja-online/llm-gateway/ingress/google"
	oaingress "github.com/inja-online/llm-gateway/ingress/openai"
)

func mediaFixtureDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "testdata", "fixtures", "media")
}

func readMediaFixture(t *testing.T, parts ...string) []byte {
	t.Helper()
	p := filepath.Join(append([]string{mediaFixtureDir(t)}, parts...)...)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	if strings.Contains(string(b), "iVBORw0KGgo") || len(b) > 8*1024 {
		t.Fatalf("fixture %s looks oversized or has non-tiny media; use YQ== only", p)
	}
	return b
}

// TestMediaFixtures loads golden media-contract samples and asserts parse /
// serialize shape stability (offline; tiny base64 YQ== only).
func TestMediaFixtures(t *testing.T) {
	t.Run("openai_image_gen", func(t *testing.T) {
		reqBody := readMediaFixture(t, "openai", "openai_image_gen_request.json")
		req, err := oaingress.ParseImageRequest(reqBody, canonical.ImageModeGenerate)
		if err != nil {
			t.Fatal(err)
		}
		if req.Model != "dall-e-3" || req.Prompt == "" || req.N != 1 {
			t.Fatalf("%+v", req)
		}
		respBody := readMediaFixture(t, "openai", "openai_image_gen_response.json")
		resp, err := oaingress.ParseImageResponse(respBody, req.Model)
		if err != nil {
			t.Fatal(err)
		}
		if len(resp.Images) != 1 || resp.Images[0].B64JSON != "YQ==" {
			t.Fatalf("%+v", resp)
		}
		out, err := oaingress.SerializeImageResponse(resp)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), "YQ==") || !strings.Contains(string(out), "b64_json") {
			t.Fatalf("serialize shape: %s", out)
		}
	})

	t.Run("anthropic_image_gen", func(t *testing.T) {
		reqBody := readMediaFixture(t, "anthropic", "anthropic_image_gen_request.json")
		req, err := antingress.ParseImageRequest(reqBody, canonical.ImageModeGenerate)
		if err != nil {
			t.Fatal(err)
		}
		if req.Model == "" || req.Prompt == "" {
			t.Fatalf("%+v", req)
		}
		respBody := readMediaFixture(t, "anthropic", "anthropic_image_gen_response.json")
		var wire map[string]any
		if err := json.Unmarshal(respBody, &wire); err != nil {
			t.Fatal(err)
		}
		if wire["type"] != "image_generation" {
			t.Fatalf("type %v", wire["type"])
		}
		// Round-trip via canonical + serialize
		canon := &canonical.ImageGenResponse{
			ID:    "img_01",
			Model: "dall-e-3",
			Images: []canonical.GeneratedImage{
				{B64JSON: "YQ==", MediaType: "image/png"},
			},
		}
		out, err := antingress.SerializeImageResponse(canon)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`"type":"image_generation"`, `"b64_json":"YQ=="`, `"id":"img_01"`} {
			if !strings.Contains(string(out), want) {
				t.Fatalf("missing %s in %s", want, out)
			}
		}
	})

	t.Run("google_image_gen", func(t *testing.T) {
		reqBody := readMediaFixture(t, "google", "google_image_gen_request.json")
		req, err := googleingress.ParseImageRequest(reqBody, "imagen-4")
		if err != nil {
			t.Fatal(err)
		}
		if req.Prompt != "a red cube" || req.N != 1 {
			t.Fatalf("%+v", req)
		}
		respBody := readMediaFixture(t, "google", "google_image_gen_response.json")
		resp, err := googleingress.ParseImageResponse(respBody, "imagen-4")
		if err != nil {
			t.Fatal(err)
		}
		if len(resp.Images) != 1 || resp.Images[0].B64JSON != "YQ==" {
			t.Fatalf("%+v", resp)
		}
		out, err := googleingress.SerializeImageResponse(resp)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), "predictions") || !strings.Contains(string(out), "YQ==") {
			t.Fatalf("%s", out)
		}
	})

	t.Run("openai_video_gen", func(t *testing.T) {
		reqBody := readMediaFixture(t, "openai", "openai_video_gen_create_request.json")
		req, err := oaingress.ParseVideoCreateRequest(reqBody)
		if err != nil {
			t.Fatal(err)
		}
		if req.Model != "sora" || req.Duration != 4 {
			t.Fatalf("%+v", req)
		}
		respBody := readMediaFixture(t, "openai", "openai_video_gen_create_response.json")
		resp, err := oaingress.ParseVideoResponse(respBody)
		if err != nil {
			t.Fatal(err)
		}
		if resp.ID != "video_1" || resp.Status != canonical.VideoStatusProcessing {
			t.Fatalf("%+v", resp)
		}
	})

	t.Run("anthropic_video_gen", func(t *testing.T) {
		reqBody := readMediaFixture(t, "anthropic", "anthropic_video_gen_create_request.json")
		req, err := antingress.ParseVideoCreateRequest(reqBody)
		if err != nil {
			t.Fatal(err)
		}
		if req.Model == "" || req.Aspect != "16:9" {
			t.Fatalf("%+v", req)
		}
		respBody := readMediaFixture(t, "anthropic", "anthropic_video_gen_create_response.json")
		var wire map[string]any
		if err := json.Unmarshal(respBody, &wire); err != nil {
			t.Fatal(err)
		}
		if wire["type"] != "video_generation" || wire["status"] != "processing" {
			t.Fatalf("%v", wire)
		}
		out, err := antingress.SerializeVideoResponse(&canonical.VideoGenResponse{
			ID:     "video_1",
			Status: canonical.VideoStatusProcessing,
			Model:  "sora",
			Result: &canonical.VideoResult{URL: "https://example.invalid/v.mp4", MediaType: "video/mp4"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), `"type":"video_generation"`) {
			t.Fatalf("%s", out)
		}
	})

	t.Run("google_video_gen", func(t *testing.T) {
		reqBody := readMediaFixture(t, "google", "google_video_gen_create_request.json")
		req, err := googleingress.ParseVideoCreateRequest(reqBody, "veo-3")
		if err != nil {
			t.Fatal(err)
		}
		if req.Prompt != "waterfall" || req.Duration != 6 {
			t.Fatalf("%+v", req)
		}
		respBody := readMediaFixture(t, "google", "google_video_gen_create_response.json")
		resp, err := googleingress.ParseVideoResponse(respBody)
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != canonical.VideoStatusProcessing {
			t.Fatalf("%+v", resp)
		}
	})

	t.Run("openai_audio_speech", func(t *testing.T) {
		// Speech is JSON passthrough (no canonical IR yet); lock request shape.
		body := readMediaFixture(t, "openai", "openai_audio_speech_request.json")
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatal(err)
		}
		if req["model"] != "tts-1" || req["input"] != "hello" || req["voice"] != "alloy" {
			t.Fatalf("%v", req)
		}
		if req["response_format"] != "mp3" {
			t.Fatalf("format %v", req["response_format"])
		}
	})

	t.Run("error_envelopes", func(t *testing.T) {
		oa := readMediaFixture(t, "errors", "openai_error_unsupported_capability.json")
		var oaEnv map[string]any
		if err := json.Unmarshal(oa, &oaEnv); err != nil {
			t.Fatal(err)
		}
		errObj, _ := oaEnv["error"].(map[string]any)
		if errObj["type"] != "unsupported_provider_capability" {
			t.Fatalf("openai envelope: %v", errObj)
		}

		ant := readMediaFixture(t, "errors", "anthropic_error_unsupported_capability.json")
		var antEnv map[string]any
		if err := json.Unmarshal(ant, &antEnv); err != nil {
			t.Fatal(err)
		}
		if antEnv["type"] != "error" {
			t.Fatalf("anthropic top type: %v", antEnv["type"])
		}
		antErr, _ := antEnv["error"].(map[string]any)
		if antErr["type"] != "unsupported_provider_capability" {
			t.Fatalf("anthropic error type: %v", antErr)
		}

		g := readMediaFixture(t, "errors", "google_error_unsupported_capability.json")
		var gEnv map[string]any
		if err := json.Unmarshal(g, &gEnv); err != nil {
			t.Fatal(err)
		}
		gErr, _ := gEnv["error"].(map[string]any)
		if gErr["status"] != "INVALID_ARGUMENT" {
			t.Fatalf("google envelope: %v", gErr)
		}
		msg, _ := gErr["message"].(string)
		if !strings.Contains(msg, "does not support modality") {
			t.Fatalf("google message: %s", msg)
		}
	})
}

// TestMediaFixtureNamingPolicy ensures committed basenames follow
// {dialect}_{modality}_{case}.json (errors/ may use dialect_error_*).
func TestMediaFixtureNamingPolicy(t *testing.T) {
	root := mediaFixtureDir(t)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".json") {
			return nil
		}
		name := info.Name()
		// Must start with a known dialect prefix.
		if !(strings.HasPrefix(name, "openai_") ||
			strings.HasPrefix(name, "anthropic_") ||
			strings.HasPrefix(name, "google_")) {
			t.Errorf("fixture %s does not start with dialect_", name)
		}
		// No huge payloads.
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(b), "iVBORw0KGgo") {
			t.Errorf("fixture %s contains non-tiny PNG base64", name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
