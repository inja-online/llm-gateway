package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

func TestImagesGenerationsPassthrough(t *testing.T) {
	var gotPath, gotAuth, gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		gotModel, _ = body["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"created":1,"data":[{"b64_json":"abc","revised_prompt":"a cat"}]}`)
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
  google_openai:
    kind: openai_compat
    base_url: %q
    capabilities:
      text: true
      image_gen: true
      video_gen: true
defaults:
  openai_dialect: openai
`, upstream.URL, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/images/generations",
		strings.NewReader(`{"model":"google_openai/gemini-2.5-flash-image","prompt":"a cat","n":2,"size":"1024x1024"}`))
	req.Header.Set("Authorization", "Bearer sk-img")
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
	if gotAuth != "Bearer sk-img" {
		t.Fatalf("auth %q", gotAuth)
	}
	if gotModel != "gemini-2.5-flash-image" {
		t.Fatalf("model %q", gotModel)
	}
	if !strings.Contains(string(body), "b64_json") {
		t.Fatalf("%s", body)
	}
	ev := col.one(t)
	if ev.Provider != "google_openai" || ev.Status != hooks.StatusOK || !ev.Estimated {
		t.Fatalf("%+v", ev)
	}
	if ev.Modality != config.ModalityImageGen {
		t.Fatalf("modality %q", ev.Modality)
	}
	if ev.Transport != hooks.TransportHTTP {
		t.Fatalf("transport %q", ev.Transport)
	}
	if ev.Media == nil || ev.Media.UnitKind != hooks.MediaUnitImage || ev.Media.Units != 2 || ev.Media.Size != "1024x1024" {
		t.Fatalf("media %+v", ev.Media)
	}
}

func TestImagesCompatWithoutCapabilitiesDenied(t *testing.T) {
	var called int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		t.Error("must not call upstream when capability denied")
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  deepseek: { kind: openai_compat, base_url: %q }
defaults:
  openai_dialect: deepseek
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	resp, err := http.Post(gw.URL+"/v1/images/generations", "application/json",
		strings.NewReader(`{"model":"deepseek/deepseek-chat","prompt":"x","n":1}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d %s", resp.StatusCode, body)
	}
	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	errObj, _ := env["error"].(map[string]any)
	if errObj["type"] != "unsupported_provider_capability" {
		t.Fatalf("error type %v body %s", errObj["type"], body)
	}
	if called != 0 {
		t.Fatalf("upstream calls %d", called)
	}
	ev := col.one(t)
	if ev.Modality != config.ModalityImageGen || ev.Transport != hooks.TransportHTTP {
		t.Fatalf("%+v", ev)
	}
	if ev.Status != hooks.StatusBadRequest || ev.Provider != "deepseek" {
		t.Fatalf("%+v", ev)
	}
	if ev.Media == nil || ev.Media.UnitKind != hooks.MediaUnitImage || ev.Media.Units != 1 {
		t.Fatalf("media %+v", ev.Media)
	}
}

func TestImagesOpenAIKindAllowedByDefault(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"created":1,"data":[{"url":"https://example/i.png"}]}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  openai_dialect: openai
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	resp, err := http.Post(gw.URL+"/v1/images/generations", "application/json",
		strings.NewReader(`{"model":"dall-e-3","prompt":"x","n":1}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	ev := col.one(t)
	if ev.Modality != config.ModalityImageGen || ev.Media == nil || ev.Media.Units != 1 {
		t.Fatalf("%+v", ev)
	}
}

func TestImagesRejectsAnthropicProvider(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not call")
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  anthropic: { kind: anthropic, base_url: %q }
  openai: { kind: openai, base_url: "https://api.openai.com/v1" }
defaults:
  openai_dialect: openai
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	resp, err := http.Post(gw.URL+"/v1/images/generations", "application/json",
		strings.NewReader(`{"model":"anthropic/claude","prompt":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	// anthropic kind defaults deny image_gen → capability error (fail closed)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "unsupported_provider_capability") {
		t.Fatalf("body %s", body)
	}
	ev := col.one(t)
	if ev.Modality != config.ModalityImageGen || ev.Status != hooks.StatusBadRequest {
		t.Fatalf("%+v", ev)
	}
}

func TestVideosCreateAndGet(t *testing.T) {
	var posts, gets int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/videos":
			posts++
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["model"] != "veo-3.1-generate-preview" {
				t.Errorf("model %v", body["model"])
			}
			fmt.Fprint(w, `{"id":"video_1","status":"processing"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/videos/video_1":
			gets++
			fmt.Fprint(w, `{"id":"video_1","status":"completed","url":"https://example/v.mp4"}`)
		default:
			t.Errorf("%s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  google_openai:
    kind: openai_compat
    base_url: %q
    capabilities:
      text: true
      image_gen: true
      video_gen: true
defaults:
  openai_dialect: google_openai
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	// create
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/videos",
		strings.NewReader(`{"model":"google_openai/veo-3.1-generate-preview","prompt":"waterfall","seconds":"8"}`))
	req.Header.Set("Authorization", "Bearer gk")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "processing") {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	evCreate := col.one(t)
	if evCreate.Modality != config.ModalityVideoGen || evCreate.Transport != hooks.TransportHTTP {
		t.Fatalf("create event %+v", evCreate)
	}
	if evCreate.Media == nil || evCreate.Media.UnitKind != hooks.MediaUnitVideoSecond || evCreate.Media.Units != 8 {
		t.Fatalf("create media %+v", evCreate.Media)
	}

	// poll with provider query
	col2 := &collector{}
	gw2 := httptest.NewServer(NewServer(cfg, col2).Handler())
	t.Cleanup(gw2.Close)
	resp2, err := http.Get(gw2.URL + "/v1/videos/video_1?provider=google_openai")
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 200 || !strings.Contains(string(b2), "completed") {
		t.Fatalf("%d %s", resp2.StatusCode, b2)
	}
	if posts != 1 || gets != 1 {
		t.Fatalf("posts=%d gets=%d", posts, gets)
	}
	evPoll := col2.one(t)
	if evPoll.Modality != config.ModalityVideoGen || evPoll.Transport != hooks.TransportHTTP {
		t.Fatalf("poll event %+v", evPoll)
	}
	// poll is operational: zero media units
	if evPoll.Media == nil || evPoll.Media.UnitKind != hooks.MediaUnitVideoSecond || evPoll.Media.Units != 0 {
		t.Fatalf("poll media %+v", evPoll.Media)
	}
}

func TestVideosCompatWithoutCapabilitiesDenied(t *testing.T) {
	var called int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		t.Error("must not call upstream")
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  openrouter: { kind: openai_compat, base_url: %q }
defaults:
  openai_dialect: openrouter
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	resp, err := http.Post(gw.URL+"/v1/videos", "application/json",
		strings.NewReader(`{"model":"openrouter/veo","prompt":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "unsupported_provider_capability") {
		t.Fatalf("%s", body)
	}
	if called != 0 {
		t.Fatalf("upstream calls %d", called)
	}
	ev := col.one(t)
	if ev.Modality != config.ModalityVideoGen || ev.Media == nil || ev.Media.UnitKind != hooks.MediaUnitVideoSecond {
		t.Fatalf("%+v", ev)
	}

	// GET poll also denied
	col2 := &collector{}
	gw2 := httptest.NewServer(NewServer(cfg, col2).Handler())
	t.Cleanup(gw2.Close)
	resp2, _ := http.Get(gw2.URL + "/v1/videos/vid_1?provider=openrouter")
	b2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest || !strings.Contains(string(b2), "unsupported_provider_capability") {
		t.Fatalf("%d %s", resp2.StatusCode, b2)
	}
	if called != 0 {
		t.Fatalf("upstream calls after poll %d", called)
	}
	col2.one(t)
}

func TestImagesMultipartEdits(t *testing.T) {
	var gotCT string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/edits" {
			t.Errorf("path %s", r.URL.Path)
		}
		gotCT = r.Header.Get("Content-Type")
		fmt.Fprint(w, `{"created":1,"data":[{"url":"https://example/i.png"}]}`)
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  openai_dialect: openai
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("model", "dall-e-2")
	_ = mw.WriteField("prompt", "add a hat")
	part, _ := mw.CreateFormFile("image", "x.png")
	_, _ = part.Write([]byte("fakepng"))
	_ = mw.Close()

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/images/edits", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer sk")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if !strings.Contains(gotCT, "multipart/") {
		t.Fatalf("content-type %q", gotCT)
	}
	ev := col.one(t)
	if ev.Modality != config.ModalityImageGen || ev.Media == nil || ev.Media.Units != 1 {
		t.Fatalf("%+v", ev)
	}
}

func TestImagesMissingModel(t *testing.T) {
	cfg, _ := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "https://api.openai.com/v1" }
defaults:
  openai_dialect: openai
`))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	resp, _ := http.Post(gw.URL+"/v1/images/generations", "application/json",
		strings.NewReader(`{"prompt":"x"}`))
	if resp.StatusCode != 400 {
		t.Fatalf("%d", resp.StatusCode)
	}
	resp.Body.Close()
	ev := col.one(t)
	if ev.Modality != config.ModalityImageGen {
		t.Fatalf("%+v", ev)
	}
}

func TestPeekMultipartModel(t *testing.T) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("model", "dall-e-3")
	_ = mw.WriteField("prompt", "hi")
	_ = mw.Close()
	if got := peekMultipartModel(buf.Bytes()); got != "dall-e-3" {
		t.Fatalf("%q", got)
	}
	if peekMultipartModel([]byte("nope")) != "" {
		t.Fatal()
	}
}

func TestVideosGetRequiresProvider(t *testing.T) {
	cfg, _ := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "https://api.openai.com/v1" }
`))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	resp, _ := http.Get(gw.URL + "/v1/videos/vid_1")
	if resp.StatusCode != 400 {
		t.Fatalf("%d", resp.StatusCode)
	}
	resp.Body.Close()
	ev := col.one(t)
	if ev.Modality != config.ModalityVideoGen {
		t.Fatalf("%+v", ev)
	}
}

func TestImagesUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"error":{"message":"bad prompt","type":"invalid_request_error"}}`)
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
	resp, _ := http.Post(gw.URL+"/v1/images/generations", "application/json",
		strings.NewReader(`{"model":"dall-e-3","prompt":"x"}`))
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 400 || !strings.Contains(string(b), "bad prompt") {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	ev := col.one(t)
	if ev.Status != hooks.StatusUpstreamError {
		t.Fatalf("%+v", ev)
	}
	if ev.Modality != config.ModalityImageGen || ev.Media == nil {
		t.Fatalf("want modality+media on upstream error: %+v", ev)
	}
}

func TestMediaUsageFromRequest(t *testing.T) {
	img := mediaUsageFromRequest(config.ModalityImageGen, map[string]any{
		"n": float64(3), "size": "512x512", "response_format": "b64_json",
	})
	if img.Units != 3 || img.UnitKind != hooks.MediaUnitImage || img.Size != "512x512" || img.Format != "b64_json" {
		t.Fatalf("%+v", img)
	}
	// default n=1
	img2 := mediaUsageFromRequest(config.ModalityImageGen, map[string]any{})
	if img2.Units != 1 {
		t.Fatalf("%+v", img2)
	}
	vid := mediaUsageFromRequest(config.ModalityVideoGen, map[string]any{"seconds": "12"})
	if vid.Units != 12 || vid.UnitKind != hooks.MediaUnitVideoSecond {
		t.Fatalf("%+v", vid)
	}
	vid2 := mediaUsageFromRequest(config.ModalityVideoGen, map[string]any{"duration": float64(4)})
	if vid2.Units != 4 {
		t.Fatalf("%+v", vid2)
	}
}

func TestImagesVariations(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/variations" {
			t.Errorf("path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"created":1,"data":[{"url":"https://example/v.png"}]}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  openai_dialect: openai
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	resp, err := http.Post(gw.URL+"/v1/images/variations", "application/json",
		strings.NewReader(`{"model":"openai/dall-e-2","n":1}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
}
