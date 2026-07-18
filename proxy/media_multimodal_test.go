package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

func TestAnthropicImagesGenerateToOpenAI(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"created":1,"data":[{"b64_json":"YQ==","revised_prompt":"cube"}]}`)
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  anthropic_dialect: openai
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/images",
		strings.NewReader(`{"model":"dall-e-3","prompt":"a red cube","n":1,"response_format":"base64"}`))
	req.Header.Set("x-api-key", "sk-ant-test")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if gotPath != "/images/generations" {
		t.Fatalf("path %s", gotPath)
	}
	if gotAuth != "Bearer sk-ant-test" {
		t.Fatalf("auth %q", gotAuth)
	}
	if gotBody["model"] != "dall-e-3" {
		t.Fatalf("model %v", gotBody["model"])
	}
	var out map[string]any
	if json.Unmarshal(body, &out) != nil {
		t.Fatalf("bad json %s", body)
	}
	if out["type"] != "image_generation" {
		t.Fatalf("type %v", out["type"])
	}
	data, _ := out["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("data %v", out["data"])
	}
	ev := col.one(t)
	if ev.DialectIn != DialectAnthropic || ev.Modality != config.ModalityImageGen {
		t.Fatalf("%+v", ev)
	}
	if ev.Media == nil || ev.Media.UnitKind != hooks.MediaUnitImage {
		t.Fatalf("media %+v", ev.Media)
	}
}

func TestAnthropicImagesRequiresVersion(t *testing.T) {
	cfg, _ := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "http://127.0.0.1:9" }
`))
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)
	resp, _ := http.Post(gw.URL+"/v1/images", "application/json",
		strings.NewReader(`{"model":"x","prompt":"y"}`))
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 400 && resp.StatusCode != 501 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if !strings.Contains(string(b), "anthropic-version") {
		t.Fatalf("%s", b)
	}
	// Anthropic error envelope
	if !strings.Contains(string(b), `"type":"error"`) {
		t.Fatalf("want anthropic envelope: %s", b)
	}
}

func TestAnthropicImagesCapabilityDeny(t *testing.T) {
	called := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Error("must not call upstream")
	}))
	t.Cleanup(upstream.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  anthropic: { kind: anthropic, base_url: %q }
defaults:
  anthropic_dialect: anthropic
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/images",
		strings.NewReader(`{"model":"claude","prompt":"x"}`))
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented && resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if called {
		t.Fatal("upstream called")
	}
	if !strings.Contains(string(b), "unsupported_provider_capability") {
		t.Fatalf("%s", b)
	}
	col.one(t)
}

func TestAnthropicImagesEditsToOpenAI(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"created":1,"data":[{"b64_json":"YQ=="}]}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/images/edits",
		strings.NewReader(`{"model":"openai/dall-e-2","prompt":"add hat","image":"YQ=="}`))
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if gotPath != "/images/edits" {
		t.Fatalf("path %s", gotPath)
	}
	if !strings.Contains(string(b), "image_generation") {
		t.Fatalf("%s", b)
	}
	col.one(t)
}

func TestOpenAIImagesRejectsAnthropicVersion(t *testing.T) {
	cfg, _ := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "http://127.0.0.1:9" }
`))
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/images/generations",
		strings.NewReader(`{"model":"dall-e-3","prompt":"x"}`))
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 400 || !strings.Contains(string(b), "POST /v1/images") {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
}

func TestOpenAICompatImageGenOptIn(t *testing.T) {
	called := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	t.Cleanup(upstream.Close)
	// no capabilities → image_gen denied for openai_compat
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  vllm: { kind: openai_compat, base_url: %q }
defaults:
  openai_dialect: vllm
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	resp, _ := http.Post(gw.URL+"/v1/images/generations", "application/json",
		strings.NewReader(`{"model":"sdxl","prompt":"x"}`))
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented && resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if called {
		t.Fatal("must not call")
	}
	col.one(t)
}

func TestGoogleGenerateImagesPassthroughPredict(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		fmt.Fprint(w, `{"predictions":[{"bytesBase64Encoded":"YQ==","mimeType":"image/png"}]}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  google_dialect: google
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost,
		gw.URL+"/v1beta/models/imagen-4.0-generate-001:generateImages",
		strings.NewReader(`{"prompt":"a robot","numberOfImages":2}`))
	req.Header.Set("x-goog-api-key", "gk")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if gotPath != "/models/imagen-4.0-generate-001:predict" {
		t.Fatalf("path %s", gotPath)
	}
	// rewritten to instances/parameters
	if _, ok := gotBody["instances"]; !ok {
		t.Fatalf("body %v", gotBody)
	}
	if !strings.Contains(string(b), "bytesBase64Encoded") {
		t.Fatalf("%s", b)
	}
	ev := col.one(t)
	if ev.Modality != config.ModalityImageGen || ev.DialectIn != DialectGoogle {
		t.Fatalf("%+v", ev)
	}
}

func TestGooglePredictNativePassthrough(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"predictions":[{"bytesBase64Encoded":"YQ=="}]}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  google_dialect: google
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	body := `{"instances":[{"prompt":"hi"}],"parameters":{"sampleCount":1}}`
	req, _ := http.NewRequest(http.MethodPost,
		gw.URL+"/v1beta/models/imagen-4.0-generate-001:predict",
		strings.NewReader(body))
	req.Header.Set("x-goog-api-key", "gk")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d", resp.StatusCode)
	}
	if gotPath != "/models/imagen-4.0-generate-001:predict" {
		t.Fatalf("%s", gotPath)
	}
	col.one(t)
}

func TestGoogleImageToOpenAITranslate(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		fmt.Fprint(w, `{"created":1,"data":[{"b64_json":"YQ=="}]}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  google_dialect: openai
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost,
		gw.URL+"/v1beta/models/dall-e-3:generateImages",
		strings.NewReader(`{"prompt":"a cat","numberOfImages":1}`))
	req.Header.Set("x-goog-api-key", "k")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if gotPath != "/images/generations" {
		t.Fatalf("path %s", gotPath)
	}
	if gotBody["prompt"] != "a cat" {
		t.Fatalf("%v", gotBody)
	}
	// Google response shape
	if !strings.Contains(string(b), "predictions") {
		t.Fatalf("%s", b)
	}
	col.one(t)
}

func TestOpenAIImageToGoogleTranslate(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"predictions":[{"bytesBase64Encoded":"YQ==","mimeType":"image/png"}]}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  openai_dialect: google
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	resp, _ := http.Post(gw.URL+"/v1/images/generations", "application/json",
		strings.NewReader(`{"model":"imagen-4.0-generate-001","prompt":"sheep","n":1,"response_format":"b64_json"}`))
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if gotPath != "/models/imagen-4.0-generate-001:predict" {
		t.Fatalf("%s", gotPath)
	}
	if !strings.Contains(string(b), "b64_json") {
		t.Fatalf("%s", b)
	}
	col.one(t)
}

func TestAnthropicImageToGoogleTranslate(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, ":predict") {
			t.Errorf("path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"predictions":[{"bytesBase64Encoded":"YQ=="}]}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  anthropic_dialect: google
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/images",
		strings.NewReader(`{"model":"imagen-4.0-generate-001","prompt":"cube"}`))
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if !strings.Contains(string(b), "image_generation") || !strings.Contains(string(b), "b64_json") {
		t.Fatalf("%s", b)
	}
	col.one(t)
}

func TestGoogleVideoGenerateAndPoll(t *testing.T) {
	var posts, gets int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":generateVideos"):
			posts++
			fmt.Fprint(w, `{"name":"operations/op1","done":false}`)
		case r.Method == http.MethodGet && r.URL.Path == "/videos/op1":
			gets++
			fmt.Fprint(w, `{"name":"operations/op1","done":true,"response":{"generatedVideos":[{"video":{"uri":"https://example/v.mp4"}}]}}`)
		default:
			t.Errorf("%s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(upstream.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  google_dialect: google
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost,
		gw.URL+"/v1beta/models/veo-3.1-generate-preview:generateVideos",
		strings.NewReader(`{"prompt":"waterfall","durationSeconds":4}`))
	req.Header.Set("x-goog-api-key", "gk")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(b), "operations/op1") {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	col.one(t)

	col2 := &collector{}
	gw2 := httptest.NewServer(NewServer(cfg, col2).Handler())
	t.Cleanup(gw2.Close)
	resp2, _ := http.Get(gw2.URL + "/v1beta/videos/op1?provider=google")
	b2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 200 || !strings.Contains(string(b2), `"done":true`) {
		t.Fatalf("%d %s", resp2.StatusCode, b2)
	}
	if posts != 1 || gets != 1 {
		t.Fatalf("posts=%d gets=%d", posts, gets)
	}
	col2.one(t)
}

func TestOpenAIVideoToGoogleTranslate(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, ":generateVideos") {
			t.Errorf("path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"name":"operations/v1","done":false}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  openai_dialect: google
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	resp, _ := http.Post(gw.URL+"/v1/videos", "application/json",
		strings.NewReader(`{"model":"veo-3","prompt":"waves"}`))
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	// OpenAI-shaped create response from canonical
	if !strings.Contains(string(b), "operations/v1") && !strings.Contains(string(b), "processing") {
		// SerializeVideoResponse uses id + status
		if !strings.Contains(string(b), `"id"`) {
			t.Fatalf("%s", b)
		}
	}
	col.one(t)
}

func TestAnthropicVideoCreateToOpenAI(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"id":"video_9","status":"processing"}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  anthropic_dialect: openai
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/videos",
		strings.NewReader(`{"model":"sora","prompt":"rain"}`))
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if gotPath != "/videos" {
		t.Fatalf("%s", gotPath)
	}
	if !strings.Contains(string(b), "video_generation") {
		t.Fatalf("%s", b)
	}
	col.one(t)
}

func TestResolveForModality(t *testing.T) {
	cfg, _ := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "https://api.openai.com/v1" }
  anthropic: { kind: anthropic, base_url: "https://api.anthropic.com" }
  vllm: { kind: openai_compat, base_url: "http://127.0.0.1:8000/v1" }
  gemini_media:
    kind: openai_compat
    base_url: "http://127.0.0.1:9"
    capabilities: { text: true, image_gen: true }
defaults:
  openai_dialect: openai
`))
	if _, err := ResolveForModality(cfg, DialectOpenAI, "dall-e-3", config.ModalityImageGen); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveForModality(cfg, DialectOpenAI, "anthropic/claude", config.ModalityImageGen); err == nil {
		t.Fatal("want capability error")
	} else if _, ok := err.(*CapabilityError); !ok {
		t.Fatalf("want CapabilityError, got %T %v", err, err)
	}
	if _, err := ResolveForModality(cfg, DialectOpenAI, "vllm/x", config.ModalityImageGen); err == nil {
		t.Fatal("openai_compat default denies image_gen")
	}
	if _, err := ResolveForModality(cfg, DialectOpenAI, "gemini_media/img", config.ModalityImageGen); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveProvider(cfg, "missing"); err == nil {
		t.Fatal()
	}
}

func TestAnthropicVideosGetOpenAI(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/videos/video_9" {
			t.Errorf("path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"id":"video_9","status":"completed","url":"https://example/v.mp4"}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  anthropic_dialect: openai
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodGet, gw.URL+"/v1/videos/video_9?provider=openai", nil)
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, _ := http.DefaultClient.Do(req)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if !strings.Contains(string(b), "video_generation") || !strings.Contains(string(b), "completed") {
		t.Fatalf("%s", b)
	}
	col.one(t)
}

func TestAnthropicVideoUpstreamErrorTranslated(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"error":{"message":"bad video","type":"invalid_request_error"}}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  anthropic_dialect: openai
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/videos",
		strings.NewReader(`{"model":"sora","prompt":"x"}`))
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 400 && resp.StatusCode != 501 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if !strings.Contains(string(b), `"type":"error"`) || !strings.Contains(string(b), "bad video") {
		t.Fatalf("%s", b)
	}
	col.one(t)
}

func TestGoogleVideoPredictLongRunning(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"name":"operations/lr1","done":false}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  google_dialect: google
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost,
		gw.URL+"/v1beta/models/veo:predictLongRunning",
		strings.NewReader(`{"instances":[{"prompt":"storm"}]}`))
	req.Header.Set("x-goog-api-key", "gk")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d", resp.StatusCode)
	}
	if gotPath != "/models/veo:predictLongRunning" {
		t.Fatalf("%s", gotPath)
	}
	col.one(t)
}

func TestGoogleVideoToOpenAITranslate(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/videos" {
			t.Errorf("path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"id":"video_g","status":"processing"}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  google_dialect: openai
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost,
		gw.URL+"/v1beta/models/sora:generateVideos",
		strings.NewReader(`{"prompt":"fog"}`))
	req.Header.Set("x-goog-api-key", "k")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if !strings.Contains(string(b), "name") { // LRO-shaped google response
		t.Fatalf("%s", b)
	}
	col.one(t)
}

func TestGoogleVideoPollToOpenAI(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/videos/vid1" {
			t.Errorf("path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"id":"vid1","status":"completed","url":"https://example/v.mp4"}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  google_dialect: openai
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	resp, _ := http.Get(gw.URL + "/v1beta/videos/vid1?provider=openai")
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if !strings.Contains(string(b), `"done":true`) {
		t.Fatalf("%s", b)
	}
	col.one(t)
}

func TestImagesVariationsPassthrough(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"created":1,"data":[{"url":"https://example/i.png"}]}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  openai_dialect: openai
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	resp, _ := http.Post(gw.URL+"/v1/images/variations", "application/json",
		strings.NewReader(`{"model":"dall-e-2","n":1}`))
	// variation may require image; our parse may fail on empty prompt for generate mode only
	// ImageModeVariation does not require prompt in ParseImageRequest
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	// Without image field, still may succeed parse for variation mode
	if resp.StatusCode != 200 {
		// if 400 because of validation, try with image
		resp2, _ := http.Post(gw.URL+"/v1/images/variations", "application/json",
			strings.NewReader(`{"model":"dall-e-2","image":"YQ==","n":1}`))
		b2, _ := io.ReadAll(resp2.Body)
		resp2.Body.Close()
		if resp2.StatusCode != 200 {
			t.Fatalf("%d %s / %d %s", resp.StatusCode, b, resp2.StatusCode, b2)
		}
		if gotPath != "/images/variations" {
			t.Fatalf("%s", gotPath)
		}
		col.one(t)
		return
	}
	if gotPath != "/images/variations" {
		t.Fatalf("%s", gotPath)
	}
	col.one(t)
}

func TestOpenAIVideoGetGoogleTranslate(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/videos/op1" {
			t.Errorf("%s", r.URL.Path)
		}
		fmt.Fprint(w, `{"name":"operations/op1","done":true,"response":{"generatedVideos":[{"video":{"uri":"https://x/v.mp4"}}]}}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  openai_dialect: google
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	resp, _ := http.Get(gw.URL + "/v1/videos/op1?provider=google")
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if !strings.Contains(string(b), "completed") || !strings.Contains(string(b), "https://x/v.mp4") {
		t.Fatalf("%s", b)
	}
	col.one(t)
}

func TestAnthropicImagesEditMissingVersionOnDedicatedPath(t *testing.T) {
	// When anthropic-version is missing, /v1/images/edits is OpenAI dialect.
	// Missing model should be 400 openai-shaped, not anthropic.
	cfg, _ := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "http://127.0.0.1:9" }
defaults:
  openai_dialect: openai
`))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	resp, _ := http.Post(gw.URL+"/v1/images/edits", "application/json",
		strings.NewReader(`{"prompt":"x"}`))
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 400 && resp.StatusCode != 501 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	// OpenAI envelope
	if !strings.Contains(string(b), `"message"`) {
		t.Fatalf("%s", b)
	}
	col.one(t)
}
