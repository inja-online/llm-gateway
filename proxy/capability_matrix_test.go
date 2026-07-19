package proxy

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
)

// Capability matrix e2e (design §12.6 / issue #79).
//
// Table-driven dialect × provider kind × modality cells:
//   - Deny cells: fake upstream Fatals on any request (zero upstream calls).
//   - Allow cells: optional smoke with a cooperative 200 fake.
// Offline, race-clean under go test -count=1 / -race.

type matrixExpect int

const (
	matrixDeny  matrixExpect = iota // 4xx capability / unsupported; no upstream
	matrixAllow                     // 2xx with cooperative upstream
)

type matrixCell struct {
	name     string
	dialect  string // openai | anthropic | google | realtime
	modality string
	// model is the public model id (provider/rest) sent to the gateway.
	model string
	// path overrides default path for the modality when non-empty.
	path string
	// body is the JSON body (or multipart model field when multipart=true).
	body      string
	multipart bool
	// extra headers (e.g. anthropic-version).
	headers map[string]string
	// method defaults to POST.
	method string
	expect matrixExpect
	// wantStatus when non-zero is the exact HTTP status expected.
	wantStatus int
	// wantSubstr must appear in the response body (deny/allow).
	wantSubstr string
}

func TestCapabilityMatrix(t *testing.T) {
	// Shared deny upstream: any hit fails the test hard.
	denyUp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("deny-cell upstream must not be called: %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(denyUp.Close)

	// Shared allow upstream: returns modality-shaped 200s.
	allowUp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/chat/completions"):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"c","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
		case strings.Contains(path, "/messages"):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"msg","type":"message","role":"assistant","model":"m","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
		case strings.Contains(path, "generateContent"):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1}}`)
		case strings.Contains(path, "/images") || strings.Contains(path, "predict") || strings.Contains(path, "generateImages"):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"created":1,"data":[{"b64_json":"YQ=="}],"predictions":[{"bytesBase64Encoded":"YQ=="}]}`)
		case strings.Contains(path, "/videos") || strings.Contains(path, "generateVideos") || strings.Contains(path, "predictLongRunning"):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"video_1","status":"processing","name":"operations/op1","done":false}`)
		case strings.Contains(path, "/audio/speech"):
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Write([]byte("ID3"))
		case strings.Contains(path, "/audio/transcriptions") || strings.Contains(path, "/audio/translations"):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"text":"ok"}`)
		default:
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{}`)
		}
	}))
	t.Cleanup(allowUp.Close)

	denyCfgYAML := fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
  anthropic: { kind: anthropic, base_url: %q }
  google: { kind: google, base_url: %q }
  compat: { kind: openai_compat, base_url: %q }
  media_compat:
    kind: openai_compat
    base_url: %q
    capabilities:
      text: true
      image_gen: true
      video_gen: true
      audio_speech: true
      audio_transcribe: true
      realtime: true
defaults:
  openai_dialect: openai
  anthropic_dialect: anthropic
  google_dialect: google
`, denyUp.URL, denyUp.URL, denyUp.URL, denyUp.URL, denyUp.URL)

	allowCfgYAML := fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
  anthropic: { kind: anthropic, base_url: %q }
  google: { kind: google, base_url: %q }
  compat: { kind: openai_compat, base_url: %q }
  media_compat:
    kind: openai_compat
    base_url: %q
    capabilities:
      text: true
      image_gen: true
      video_gen: true
      audio_speech: true
      audio_transcribe: true
      realtime: true
defaults:
  openai_dialect: openai
  anthropic_dialect: anthropic
  google_dialect: google
`, allowUp.URL, allowUp.URL, allowUp.URL, allowUp.URL, allowUp.URL)

	denyCfg, err := config.Parse([]byte(denyCfgYAML))
	if err != nil {
		t.Fatal(err)
	}
	allowCfg, err := config.Parse([]byte(allowCfgYAML))
	if err != nil {
		t.Fatal(err)
	}

	cells := []matrixCell{
		// --- text ---
		{
			name: "text/openai→openai", dialect: DialectOpenAI, modality: config.ModalityText,
			path: "/v1/chat/completions", model: "openai/gpt-test",
			body:   `{"model":"openai/gpt-test","messages":[{"role":"user","content":"hi"}]}`,
			expect: matrixAllow, wantStatus: 200,
		},
		{
			name: "text/openai→compat", dialect: DialectOpenAI, modality: config.ModalityText,
			path: "/v1/chat/completions", model: "compat/chat",
			body:   `{"model":"compat/chat","messages":[{"role":"user","content":"hi"}]}`,
			expect: matrixAllow, wantStatus: 200,
		},
		{
			name: "text/anthropic→anthropic", dialect: DialectAnthropic, modality: config.ModalityText,
			path: "/v1/messages", model: "anthropic/claude",
			body:   `{"model":"anthropic/claude","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`,
			expect: matrixAllow, wantStatus: 200,
		},
		{
			name: "text/google→google", dialect: DialectGoogle, modality: config.ModalityText,
			// Bare path model; routes via defaults.google_dialect (slash in path breaks {action}).
			path: "/v1beta/models/gemini-test:generateContent", model: "gemini-test",
			body:   `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`,
			expect: matrixAllow, wantStatus: 200,
		},

		// --- image_gen deny ---
		{
			name: "image_gen/openai→anthropic deny", dialect: DialectOpenAI, modality: config.ModalityImageGen,
			path: "/v1/images/generations", model: "anthropic/claude",
			body: `{"model":"anthropic/claude","prompt":"x","n":1}`,
			expect: matrixDeny, wantStatus: http.StatusBadRequest,
			wantSubstr: "unsupported_provider_capability",
		},
		{
			name: "image_gen/openai→compat off deny", dialect: DialectOpenAI, modality: config.ModalityImageGen,
			path: "/v1/images/generations", model: "compat/img",
			body: `{"model":"compat/img","prompt":"x","n":1}`,
			expect: matrixDeny, wantStatus: http.StatusBadRequest,
			wantSubstr: "unsupported_provider_capability",
		},
		{
			name: "image_gen/anthropic→anthropic deny", dialect: DialectAnthropic, modality: config.ModalityImageGen,
			path: "/v1/images", model: "anthropic/claude",
			body: `{"model":"anthropic/claude","prompt":"x"}`,
			headers: map[string]string{"anthropic-version": "2023-06-01"},
			expect:  matrixDeny,
			// capability or kind rejection before upstream
			wantSubstr: "unsupported_provider_capability",
		},

		// --- image_gen allow ---
		{
			name: "image_gen/openai→openai", dialect: DialectOpenAI, modality: config.ModalityImageGen,
			path: "/v1/images/generations", model: "openai/dall-e-3",
			body:   `{"model":"openai/dall-e-3","prompt":"a cube","n":1}`,
			expect: matrixAllow, wantStatus: 200, wantSubstr: "YQ==",
		},
		{
			name: "image_gen/openai→media_compat", dialect: DialectOpenAI, modality: config.ModalityImageGen,
			path: "/v1/images/generations", model: "media_compat/img",
			body:   `{"model":"media_compat/img","prompt":"a cube","n":1}`,
			expect: matrixAllow, wantStatus: 200,
		},
		{
			name: "image_gen/anthropic→openai", dialect: DialectAnthropic, modality: config.ModalityImageGen,
			path: "/v1/images", model: "openai/dall-e-3",
			body:    `{"model":"openai/dall-e-3","prompt":"a cube"}`,
			headers: map[string]string{"anthropic-version": "2023-06-01"},
			expect:  matrixAllow, wantStatus: 200, wantSubstr: "image_generation",
		},
		{
			name: "image_gen/google→google", dialect: DialectGoogle, modality: config.ModalityImageGen,
			path: "/v1beta/models/imagen-4:generateImages", model: "imagen-4",
			body:   `{"prompt":"robot","numberOfImages":1}`,
			expect: matrixAllow, wantStatus: 200,
		},

		// --- video_gen deny ---
		{
			name: "video_gen/openai→anthropic deny", dialect: DialectOpenAI, modality: config.ModalityVideoGen,
			path: "/v1/videos", model: "anthropic/claude",
			body: `{"model":"anthropic/claude","prompt":"rain"}`,
			expect: matrixDeny, wantStatus: http.StatusBadRequest,
			wantSubstr: "unsupported_provider_capability",
		},
		{
			name: "video_gen/openai→compat off deny", dialect: DialectOpenAI, modality: config.ModalityVideoGen,
			path: "/v1/videos", model: "compat/veo",
			body: `{"model":"compat/veo","prompt":"rain"}`,
			expect: matrixDeny, wantStatus: http.StatusBadRequest,
			wantSubstr: "unsupported_provider_capability",
		},

		// --- video_gen allow ---
		{
			name: "video_gen/openai→openai", dialect: DialectOpenAI, modality: config.ModalityVideoGen,
			path: "/v1/videos", model: "openai/sora",
			body:   `{"model":"openai/sora","prompt":"rain","seconds":"4"}`,
			expect: matrixAllow, wantStatus: 200,
		},
		{
			name: "video_gen/openai→media_compat", dialect: DialectOpenAI, modality: config.ModalityVideoGen,
			path: "/v1/videos", model: "media_compat/veo",
			body:   `{"model":"media_compat/veo","prompt":"rain"}`,
			expect: matrixAllow, wantStatus: 200,
		},

		// --- audio_speech deny ---
		{
			name: "audio_speech/openai→compat off deny", dialect: DialectOpenAI, modality: config.ModalityAudioSpeech,
			path: "/v1/audio/speech", model: "compat/tts",
			body: `{"model":"compat/tts","input":"hi","voice":"alloy"}`,
			expect: matrixDeny, wantStatus: http.StatusBadRequest,
			wantSubstr: "unsupported_provider_capability",
		},
		{
			name: "audio_speech/openai→anthropic deny", dialect: DialectOpenAI, modality: config.ModalityAudioSpeech,
			path: "/v1/audio/speech", model: "anthropic/claude",
			body: `{"model":"anthropic/claude","input":"hi","voice":"alloy"}`,
			// non-openai-family → 501; capability path for anthropic is also fail-closed
			expect: matrixDeny, wantStatus: http.StatusNotImplemented,
		},

		// --- audio_speech allow ---
		{
			name: "audio_speech/openai→openai", dialect: DialectOpenAI, modality: config.ModalityAudioSpeech,
			path: "/v1/audio/speech", model: "openai/tts-1",
			body:   `{"model":"openai/tts-1","input":"hi","voice":"alloy"}`,
			expect: matrixAllow, wantStatus: 200,
		},
		{
			name: "audio_speech/openai→media_compat", dialect: DialectOpenAI, modality: config.ModalityAudioSpeech,
			path: "/v1/audio/speech", model: "media_compat/tts",
			body:   `{"model":"media_compat/tts","input":"hi","voice":"alloy"}`,
			expect: matrixAllow, wantStatus: 200,
		},

		// --- audio_transcribe deny ---
		{
			name: "audio_transcribe/openai→compat off deny", dialect: DialectOpenAI, modality: config.ModalityAudioTranscribe,
			path: "/v1/audio/transcriptions", model: "compat/whisper",
			body: "compat/whisper", multipart: true,
			expect: matrixDeny, wantStatus: http.StatusBadRequest,
			wantSubstr: "unsupported_provider_capability",
		},

		// --- audio_transcribe allow ---
		{
			name: "audio_transcribe/openai→openai", dialect: DialectOpenAI, modality: config.ModalityAudioTranscribe,
			path: "/v1/audio/transcriptions", model: "openai/whisper-1",
			body: "openai/whisper-1", multipart: true,
			expect: matrixAllow, wantStatus: 200, wantSubstr: "ok",
		},

		// --- realtime subset (deny cells; upgrade not required for capability reject) ---
		{
			name: "realtime/openai→anthropic deny", dialect: "realtime", modality: config.ModalityRealtime,
			path: "/v1/realtime?model=anthropic/claude", model: "anthropic/claude",
			method: http.MethodGet,
			headers: map[string]string{
				"Upgrade":               "websocket",
				"Connection":            "Upgrade",
				"Sec-WebSocket-Key":     "dGhlIHNhbXBsZSBub25jZQ==",
				"Sec-WebSocket-Version": "13",
			},
			expect: matrixDeny, wantStatus: http.StatusNotImplemented,
		},
		{
			name: "realtime/openai→compat off deny", dialect: "realtime", modality: config.ModalityRealtime,
			path: "/v1/realtime?model=compat/rt", model: "compat/rt",
			method: http.MethodGet,
			headers: map[string]string{
				"Upgrade":               "websocket",
				"Connection":            "Upgrade",
				"Sec-WebSocket-Key":     "dGhlIHNhbXBsZSBub25jZQ==",
				"Sec-WebSocket-Version": "13",
			},
			expect: matrixDeny,
			// Realtime deny uses invalid_request_error envelope + capabilities.realtime hint.
			wantSubstr: "does not support realtime",
		},
	}

	for _, cell := range cells {
		cell := cell
		t.Run(cell.name, func(t *testing.T) {
			cfg := allowCfg
			if cell.expect == matrixDeny {
				cfg = denyCfg
			}
			col := &collector{}
			gw := httptest.NewServer(NewServer(cfg, col).Handler())
			t.Cleanup(gw.Close)

			method := cell.method
			if method == "" {
				method = http.MethodPost
			}
			url := gw.URL + cell.path

			var req *http.Request
			if cell.multipart {
				var buf bytes.Buffer
				mw := multipart.NewWriter(&buf)
				_ = mw.WriteField("model", cell.body)
				part, _ := mw.CreateFormFile("file", "a.wav")
				_, _ = part.Write([]byte("RIFF"))
				_ = mw.Close()
				req, _ = http.NewRequest(method, url, &buf)
				req.Header.Set("Content-Type", mw.FormDataContentType())
			} else if method == http.MethodGet {
				req, _ = http.NewRequest(method, url, nil)
			} else {
				req, _ = http.NewRequest(method, url, strings.NewReader(cell.body))
				req.Header.Set("Content-Type", "application/json")
			}
			req.Header.Set("Authorization", "Bearer sk-matrix")
			req.Header.Set("x-goog-api-key", "gk")
			for k, v := range cell.headers {
				req.Header.Set(k, v)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)

			if cell.wantStatus != 0 && resp.StatusCode != cell.wantStatus {
				t.Fatalf("status %d want %d body %s", resp.StatusCode, cell.wantStatus, body)
			}
			if cell.expect == matrixDeny {
				if resp.StatusCode < 400 {
					t.Fatalf("deny cell got %d body %s", resp.StatusCode, body)
				}
			}
			if cell.expect == matrixAllow {
				if resp.StatusCode != 200 {
					t.Fatalf("allow cell got %d body %s", resp.StatusCode, body)
				}
			}
			if cell.wantSubstr != "" && !strings.Contains(string(body), cell.wantSubstr) {
				t.Fatalf("body missing %q: %s", cell.wantSubstr, body)
			}
		})
	}
}

// TestCapabilityMatrixCompatMediaDefaultOff locks openai_compat kind defaults:
// media/realtime off unless opted in — zero upstream calls.
func TestCapabilityMatrixCompatMediaDefaultOff(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("openai_compat media default-off must not call upstream: %s", r.URL.Path)
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

	cases := []struct {
		name string
		path string
		body string
		mp   bool
	}{
		{"image_gen", "/v1/images/generations", `{"model":"deepseek/x","prompt":"p"}`, false},
		{"video_gen", "/v1/videos", `{"model":"deepseek/x","prompt":"p"}`, false},
		{"audio_speech", "/v1/audio/speech", `{"model":"deepseek/x","input":"hi","voice":"alloy"}`, false},
		{"audio_transcribe", "/v1/audio/transcriptions", "deepseek/whisper", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			col := &collector{}
			gw := httptest.NewServer(NewServer(cfg, col).Handler())
			t.Cleanup(gw.Close)

			var req *http.Request
			if tc.mp {
				var buf bytes.Buffer
				mw := multipart.NewWriter(&buf)
				_ = mw.WriteField("model", tc.body)
				part, _ := mw.CreateFormFile("file", "a.wav")
				_, _ = part.Write([]byte("x"))
				_ = mw.Close()
				req, _ = http.NewRequest(http.MethodPost, gw.URL+tc.path, &buf)
				req.Header.Set("Content-Type", mw.FormDataContentType())
			} else {
				req, _ = http.NewRequest(http.MethodPost, gw.URL+tc.path, strings.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
			}
			req.Header.Set("Authorization", "Bearer sk")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status %d %s", resp.StatusCode, b)
			}
			if !strings.Contains(string(b), "unsupported_provider_capability") {
				t.Fatalf("%s", b)
			}
			col.one(t)
		})
	}
}
