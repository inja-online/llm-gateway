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
  google_openai: { kind: openai_compat, base_url: %q }
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
		strings.NewReader(`{"model":"google_openai/gemini-2.5-flash-image","prompt":"a cat","n":1}`))
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
}

func TestImagesRejectsAnthropicProvider(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not call")
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  anthropic: { kind: anthropic, base_url: %q }
defaults:
  openai_dialect: anthropic
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	// openai_dialect pointing at anthropic is weird but valid for bare models;
	// provider/model path is what we test.
	cfg, err = config.Parse([]byte(fmt.Sprintf(`
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
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
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
  google_openai: { kind: openai_compat, base_url: %q }
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
		strings.NewReader(`{"model":"google_openai/veo-3.1-generate-preview","prompt":"waterfall"}`))
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
	col.one(t)

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
	col.one(t)
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
	col.one(t)
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
	col.one(t)
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
}
