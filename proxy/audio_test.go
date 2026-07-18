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

func TestAudioSpeechPassthroughBinary(t *testing.T) {
	wantAudio := []byte{0xFF, 0xFB, 0x90, 0x00, 'I', 'D', '3', 0x01, 0x02, 0x03}
	var gotPath, gotAuth, gotModel, gotInput string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		gotModel, _ = body["model"].(string)
		gotInput, _ = body["input"].(string)
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write(wantAudio)
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
  tts_host:
    kind: openai_compat
    base_url: %q
    capabilities: { text: true, audio_speech: true }
defaults:
  openai_dialect: openai
`, upstream.URL, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	reqBody := `{"model":"tts_host/tts-1","input":"Hello world","voice":"alloy","response_format":"mp3"}`
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/audio/speech", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer sk-tts")
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
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "audio/mpeg") {
		t.Fatalf("content-type %q", ct)
	}
	if !bytes.Equal(body, wantAudio) {
		t.Fatalf("binary mismatch: got %v want %v", body, wantAudio)
	}
	if gotPath != "/audio/speech" {
		t.Fatalf("path %s", gotPath)
	}
	if gotAuth != "Bearer sk-tts" {
		t.Fatalf("auth %q", gotAuth)
	}
	if gotModel != "tts-1" {
		t.Fatalf("model rewrite %q", gotModel)
	}
	if gotInput != "Hello world" {
		t.Fatalf("input %q", gotInput)
	}
	ev := col.one(t)
	if ev.Modality != config.ModalityAudioSpeech || ev.Provider != "tts_host" || ev.Status != hooks.StatusOK {
		t.Fatalf("%+v", ev)
	}
	if !ev.Estimated {
		t.Fatal("expected estimated")
	}
	if ev.Media == nil || ev.Media.UnitKind != hooks.MediaUnitAudioCharacter || ev.Media.Units != len([]rune("Hello world")) {
		t.Fatalf("media %+v", ev.Media)
	}
	if ev.Media.Format != "mp3" {
		t.Fatalf("format %q", ev.Media.Format)
	}
}

func TestAudioSpeechNativeOpenAIDefaultCaps(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		w.Write([]byte("RIFFxxxxWAVEfmt "))
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
	resp, err := http.Post(gw.URL+"/v1/audio/speech", "application/json",
		strings.NewReader(`{"model":"tts-1-hd","input":"hi","voice":"nova"}`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if string(b) != "RIFFxxxxWAVEfmt " {
		t.Fatalf("%q", b)
	}
	ev := col.one(t)
	if ev.Modality != config.ModalityAudioSpeech || ev.Provider != "openai" {
		t.Fatalf("%+v", ev)
	}
}

func TestAudioTranscriptionsMultipart(t *testing.T) {
	var gotPath, gotCT string
	var gotBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"text":"hello from audio"}`)
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
	_ = mw.WriteField("model", "whisper-1")
	_ = mw.WriteField("language", "en")
	part, _ := mw.CreateFormFile("file", "clip.wav")
	fakeAudio := []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x01, 0x00, 0x00}
	_, _ = part.Write(fakeAudio)
	_ = mw.Close()
	formCT := mw.FormDataContentType()

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/audio/transcriptions", &buf)
	req.Header.Set("Content-Type", formCT)
	req.Header.Set("Authorization", "Bearer sk-stt")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if gotPath != "/audio/transcriptions" {
		t.Fatalf("path %s", gotPath)
	}
	if !strings.Contains(gotCT, "multipart/") {
		t.Fatalf("content-type %q", gotCT)
	}
	if !bytes.Contains(gotBody, fakeAudio) {
		t.Fatalf("multipart body lost file bytes")
	}
	if !bytes.Contains(gotBody, []byte("whisper-1")) {
		t.Fatalf("multipart body lost model field")
	}
	if !strings.Contains(string(body), "hello from audio") {
		t.Fatalf("%s", body)
	}
	ev := col.one(t)
	if ev.Modality != config.ModalityAudioTranscribe || ev.Model != "whisper-1" || ev.Status != hooks.StatusOK {
		t.Fatalf("%+v", ev)
	}
	if ev.Media == nil || ev.Media.UnitKind != hooks.MediaUnitAudioMinute {
		t.Fatalf("media %+v", ev.Media)
	}
}

func TestAudioTranslationsMultipart(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"text":"translated"}`)
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

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("model", "whisper-1")
	part, _ := mw.CreateFormFile("file", "fr.mp3")
	_, _ = part.Write([]byte("fake-mp3"))
	_ = mw.Close()

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/audio/translations", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer sk")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if gotPath != "/audio/translations" {
		t.Fatalf("path %s", gotPath)
	}
	ev := col.one(t)
	if ev.Modality != config.ModalityAudioTranscribe {
		t.Fatalf("%+v", ev)
	}
}

func TestAudioCapabilityDenyTable(t *testing.T) {
	// Fake upstream must never be hit on deny paths.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("upstream must not be called: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(500)
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
  deepseek: { kind: openai_compat, base_url: %q }
  stt_only:
    kind: openai_compat
    base_url: %q
    capabilities: { text: true, audio_transcribe: true }
  tts_only:
    kind: openai_compat
    base_url: %q
    capabilities: { text: true, audio_speech: true }
  anthropic: { kind: anthropic, base_url: %q }
defaults:
  openai_dialect: openai
`, upstream.URL, upstream.URL, upstream.URL, upstream.URL, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name       string
		path       string
		body       string
		multipart  bool
		wantStatus int
		wantType   string
	}{
		{
			name:       "speech openai_compat no cap",
			path:       "/v1/audio/speech",
			body:       `{"model":"deepseek/tts","input":"x","voice":"alloy"}`,
			wantStatus: http.StatusBadRequest,
			wantType:   "unsupported_provider_capability",
		},
		{
			name:       "speech stt-only host denied",
			path:       "/v1/audio/speech",
			body:       `{"model":"stt_only/tts","input":"x","voice":"alloy"}`,
			wantStatus: http.StatusBadRequest,
			wantType:   "unsupported_provider_capability",
		},
		{
			name:       "speech anthropic family denied",
			path:       "/v1/audio/speech",
			body:       `{"model":"anthropic/claude","input":"x","voice":"alloy"}`,
			wantStatus: http.StatusNotImplemented,
			wantType:   "invalid_request_error",
		},
		{
			name:       "transcriptions openai_compat no cap",
			path:       "/v1/audio/transcriptions",
			multipart:  true,
			body:       "deepseek/whisper",
			wantStatus: http.StatusBadRequest,
			wantType:   "unsupported_provider_capability",
		},
		{
			name:       "transcriptions tts-only host denied",
			path:       "/v1/audio/transcriptions",
			multipart:  true,
			body:       "tts_only/whisper",
			wantStatus: http.StatusBadRequest,
			wantType:   "unsupported_provider_capability",
		},
		{
			name:       "translations openai_compat no cap",
			path:       "/v1/audio/translations",
			multipart:  true,
			body:       "deepseek/whisper",
			wantStatus: http.StatusBadRequest,
			wantType:   "unsupported_provider_capability",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			col := &collector{}
			gw := httptest.NewServer(NewServer(cfg, col).Handler())
			t.Cleanup(gw.Close)

			var req *http.Request
			if tc.multipart {
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
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status %d want %d body %s", resp.StatusCode, tc.wantStatus, b)
			}
			var env struct {
				Error struct {
					Type string `json:"type"`
				} `json:"error"`
			}
			if json.Unmarshal(b, &env) != nil || env.Error.Type != tc.wantType {
				t.Fatalf("error type %q want %q body %s", env.Error.Type, tc.wantType, b)
			}
			col.one(t)
		})
	}
}

func TestAudioSpeechMissingModel(t *testing.T) {
	cfg, _ := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "https://api.openai.com/v1" }
defaults:
  openai_dialect: openai
`))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	resp, _ := http.Post(gw.URL+"/v1/audio/speech", "application/json",
		strings.NewReader(`{"input":"hi","voice":"alloy"}`))
	if resp.StatusCode != 400 {
		t.Fatalf("%d", resp.StatusCode)
	}
	resp.Body.Close()
	col.one(t)
}

func TestAudioSpeechUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"error":{"message":"bad voice","type":"invalid_request_error"}}`)
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
	resp, _ := http.Post(gw.URL+"/v1/audio/speech", "application/json",
		strings.NewReader(`{"model":"tts-1","input":"x","voice":"nope"}`))
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 400 || !strings.Contains(string(b), "bad voice") {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	ev := col.one(t)
	if ev.Status != hooks.StatusUpstreamError || ev.Modality != config.ModalityAudioSpeech {
		t.Fatalf("%+v", ev)
	}
}

func TestAudioCompatOptInTranscriptions(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/transcriptions" {
			t.Errorf("path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"text":"ok"}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  groq:
    kind: openai_compat
    base_url: %q
    capabilities: { text: true, audio_transcribe: true }
defaults:
  openai_dialect: groq
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("model", "groq/whisper-large-v3")
	part, _ := mw.CreateFormFile("file", "a.wav")
	_, _ = part.Write([]byte("wav"))
	_ = mw.Close()

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/audio/transcriptions", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	ev := col.one(t)
	if ev.Provider != "groq" || ev.UpstreamModel != "whisper-large-v3" {
		t.Fatalf("%+v", ev)
	}
}

func TestImageCapabilityDenyCompat(t *testing.T) {
	// Shared helper: openai_compat without image_gen must fail closed.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not call upstream")
	}))
	t.Cleanup(upstream.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  deepseek: { kind: openai_compat, base_url: %q }
defaults:
  openai_dialect: deepseek
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	resp, _ := http.Post(gw.URL+"/v1/images/generations", "application/json",
		strings.NewReader(`{"model":"dall-e-3","prompt":"x"}`))
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if !strings.Contains(string(b), "unsupported_provider_capability") {
		t.Fatalf("%s", b)
	}
	col.one(t)
}
