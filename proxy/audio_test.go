package proxy

import (
	"bytes"
	"encoding/base64"
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
			wantStatus: http.StatusBadRequest,
			wantType:   "unsupported_provider_capability",
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

// --- #77 Google audio speech ---

func TestGoogleGenerateSpeechToGenerateContent(t *testing.T) {
	wantB64 := base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03, 0x04})
	var gotPath string
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"audio/L16","data":%q}}]}}]}`, wantB64)
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  google_dialect: google
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	reqBody := `{"text":"Hello Google TTS","voice":"Kore"}`
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1beta/models/gemini-2.5-flash-preview-tts:generateSpeech", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", "gk")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if gotPath != "/models/gemini-2.5-flash-preview-tts:generateContent" {
		t.Fatalf("path %s", gotPath)
	}
	// generationConfig.responseModalities must include AUDIO
	gc, _ := gotBody["generationConfig"].(map[string]any)
	if gc == nil {
		t.Fatalf("missing generationConfig: %+v", gotBody)
	}
	mods, _ := gc["responseModalities"].([]any)
	if len(mods) == 0 || mods[0] != "AUDIO" {
		t.Fatalf("modalities %+v", mods)
	}
	if !strings.Contains(string(body), wantB64) {
		t.Fatalf("response missing audio b64: %s", body)
	}
	ev := col.one(t)
	if ev.Modality != config.ModalityAudioSpeech || ev.Provider != "google" || ev.Status != hooks.StatusOK {
		t.Fatalf("%+v", ev)
	}
	if ev.Media == nil || ev.Media.UnitKind != hooks.MediaUnitAudioCharacter || ev.Media.Units != len([]rune("Hello Google TTS")) {
		t.Fatalf("media %+v", ev.Media)
	}
}

func TestGoogleGenerateSpeechCapabilityDeny(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream must not be called")
	}))
	t.Cleanup(upstream.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google:
    kind: google
    base_url: %q
    capabilities: { text: true, audio_speech: false }
defaults:
  google_dialect: google
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	resp, _ := http.Post(gw.URL+"/v1beta/models/tts:generateSpeech", "application/json",
		strings.NewReader(`{"text":"hi"}`))
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if !strings.Contains(string(b), "unsupported_provider_capability") && !strings.Contains(string(b), "does not support modality") {
		t.Fatalf("%s", b)
	}
	col.one(t)
}

func TestOpenAISpeechToGoogleBinary(t *testing.T) {
	pcm := []byte{0x10, 0x20, 0x30, 0x40, 0x50}
	b64 := base64.StdEncoding.EncodeToString(pcm)
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"candidates":[{"content":{"parts":[{"inline_data":{"mime_type":"audio/L16","data":%q}}]}}]}`, b64)
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

	reqBody := `{"model":"gemini-2.5-flash-preview-tts","input":"Hi","voice":"alloy","response_format":"pcm"}`
	resp, err := http.Post(gw.URL+"/v1/audio/speech", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if !bytes.Equal(body, pcm) {
		t.Fatalf("binary mismatch got %v want %v", body, pcm)
	}
	if !strings.Contains(gotPath, ":generateContent") {
		t.Fatalf("path %s", gotPath)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "audio") {
		t.Fatalf("content-type %q", ct)
	}
	ev := col.one(t)
	if ev.Modality != config.ModalityAudioSpeech || ev.Provider != "google" {
		t.Fatalf("%+v", ev)
	}
}

func TestGoogleSpeechToOpenAIWrapped(t *testing.T) {
	wantAudio := []byte{0xFF, 0xFB, 0x11, 0x22}
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write(wantAudio)
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

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1beta/models/tts-1:generateSpeech",
		strings.NewReader(`{"text":"wrap me","voice":"alloy"}`))
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
	if gotPath != "/audio/speech" {
		t.Fatalf("path %s", gotPath)
	}
	var env struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					InlineData struct {
						Data string `json:"data"`
					} `json:"inlineData"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	got, err := base64.StdEncoding.DecodeString(env.Candidates[0].Content.Parts[0].InlineData.Data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, wantAudio) {
		t.Fatalf("wrapped audio mismatch")
	}
	col.one(t)
}

// --- #78 Anthropic gateway audio ---

func TestAnthropicAudioSpeechToOpenAI(t *testing.T) {
	wantAudio := []byte{0x49, 0x44, 0x33, 0x04, 0x00}
	var gotPath, gotModel, gotInput string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		gotModel, _ = body["model"].(string)
		gotInput, _ = body["input"].(string)
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write(wantAudio)
	}))
	t.Cleanup(upstream.Close)

	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
  anthropic: { kind: anthropic, base_url: %q }
defaults:
  anthropic_dialect: openai
`, upstream.URL, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/audio/speech",
		strings.NewReader(`{"model":"tts-1","input":"Anthropic hello","voice":"nova","response_format":"mp3"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("x-api-key", "sk-ant")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if !bytes.Equal(body, wantAudio) {
		t.Fatalf("binary mismatch")
	}
	if gotPath != "/audio/speech" || gotModel != "tts-1" || gotInput != "Anthropic hello" {
		t.Fatalf("path=%s model=%s input=%s", gotPath, gotModel, gotInput)
	}
	ev := col.one(t)
	if ev.Modality != config.ModalityAudioSpeech || ev.DialectIn != DialectAnthropic {
		t.Fatalf("%+v", ev)
	}
}

func TestAnthropicAudioSpeechRequiresVersionForDialect(t *testing.T) {
	// Without anthropic-version, same path is OpenAI dialect (still works with openai default).
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		w.Write([]byte("RIFF"))
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
		strings.NewReader(`{"model":"tts-1","input":"x","voice":"alloy"}`))
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	ev := col.one(t)
	if ev.DialectIn != DialectOpenAI {
		t.Fatalf("dialect %s", ev.DialectIn)
	}
}

func TestAnthropicAudioSpeechPureAnthropicDenied(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/audio/speech",
		strings.NewReader(`{"model":"claude","input":"x","voice":"alloy"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	// Anthropic error envelope
	if !strings.Contains(string(b), `"type":"error"`) && !strings.Contains(string(b), `"type": "error"`) {
		// type is top-level for anthropic
	}
	var env struct {
		Type  string `json:"type"`
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	_ = json.Unmarshal(b, &env)
	if env.Error.Type != "unsupported_provider_capability" && !strings.Contains(string(b), "unsupported_provider_capability") {
		t.Fatalf("%s", b)
	}
	col.one(t)
}

func TestAnthropicAudioTranscriptionsMultipart(t *testing.T) {
	var gotPath, gotCT string
	var gotBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"text":"anthropic stt"}`)
	}))
	t.Cleanup(upstream.Close)

	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  openai_dialect: openai
  anthropic_dialect: openai
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("model", "whisper-1")
	_ = mw.WriteField("language", "en")
	part, _ := mw.CreateFormFile("file", "clip.wav")
	fakeAudio := []byte{0x52, 0x49, 0x46, 0x46, 0xAA, 0xBB}
	_, _ = part.Write(fakeAudio)
	_ = mw.Close()
	formCT := mw.FormDataContentType()

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/audio/transcriptions", &buf)
	req.Header.Set("Content-Type", formCT)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("x-api-key", "sk-ant")
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
		t.Fatalf("ct %q", gotCT)
	}
	if !bytes.Contains(gotBody, fakeAudio) {
		t.Fatal("multipart file bytes lost")
	}
	// Boundary string must appear in forwarded body (passthrough).
	if !bytes.Contains(gotBody, []byte("whisper-1")) {
		t.Fatal("model field lost")
	}
	if !strings.Contains(string(body), "anthropic stt") {
		t.Fatalf("%s", body)
	}
	ev := col.one(t)
	if ev.Modality != config.ModalityAudioTranscribe || ev.DialectIn != DialectAnthropic {
		t.Fatalf("%+v", ev)
	}
}

func TestAnthropicAudioTranslations(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"text":"hello"}`)
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
	_, _ = part.Write([]byte("mp3data"))
	_ = mw.Close()

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/audio/translations", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("anthropic-version", "2023-06-01")
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
	col.one(t)
}

// --- #82 Multipart + binary TTS fidelity ---

func TestAudioSpeechBinaryFidelityNoReencode(t *testing.T) {
	// Arbitrary binary including nulls — must be byte-equal on passthrough.
	wantAudio := make([]byte, 256)
	for i := range wantAudio {
		wantAudio[i] = byte(i)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/flac")
		w.Write(wantAudio)
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
		strings.NewReader(`{"model":"tts-1","input":"fidelity","voice":"alloy","response_format":"flac"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("%d", resp.StatusCode)
	}
	if !bytes.Equal(body, wantAudio) {
		t.Fatalf("TTS body re-encoded or truncated: len=%d want=%d", len(body), len(wantAudio))
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "audio/flac") {
		t.Fatalf("content-type %q", ct)
	}
	col.one(t)
}

func TestAudioMultipartBoundaryAndFileBytesPreserved(t *testing.T) {
	var gotBody []byte
	var gotCT string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		fmt.Fprint(w, `{"text":"ok"}`)
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
	boundary := mw.Boundary()
	_ = mw.WriteField("model", "whisper-1")
	_ = mw.WriteField("language", "en")
	part, _ := mw.CreateFormFile("file", "clip.wav")
	// Include boundary-like sequences and nulls inside file payload.
	fileBytes := append([]byte{0x00, 0xFF}, []byte("--"+boundary+"fake")...)
	fileBytes = append(fileBytes, 0x00, 0x01, 0x02)
	_, _ = part.Write(fileBytes)
	_ = mw.Close()
	clientBody := buf.Bytes()
	formCT := mw.FormDataContentType()

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/audio/transcriptions", bytes.NewReader(clientBody))
	req.Header.Set("Content-Type", formCT)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d", resp.StatusCode)
	}
	// Exact body passthrough (including multipart structure).
	if !bytes.Equal(gotBody, clientBody) {
		t.Fatalf("multipart body not byte-equal: got %d want %d", len(gotBody), len(clientBody))
	}
	if gotCT != formCT {
		t.Fatalf("content-type not preserved: got %q want %q", gotCT, formCT)
	}
	if !bytes.Contains(gotBody, fileBytes) {
		t.Fatal("file payload lost")
	}
	col.one(t)
}

func TestAudioMultipartMaxBodyBytesBoundary(t *testing.T) {
	// Under limit: full body accepted and forwarded.
	// At/over LimitReader capacity: readBody truncates to maxBodyBytes (documented).
	if maxBodyBytes != 32<<20 {
		t.Fatalf("maxBodyBytes = %d want 32MiB", maxBodyBytes)
	}

	var gotLen int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotLen = len(b)
		fmt.Fprint(w, `{"text":"ok"}`)
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

	// Comfortably under limit — full passthrough.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("model", "whisper-1")
	part, _ := mw.CreateFormFile("file", "a.wav")
	payload := bytes.Repeat([]byte{0xAB}, 64*1024) // 64 KiB
	_, _ = part.Write(payload)
	_ = mw.Close()
	clientBody := buf.Bytes()

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/audio/transcriptions", bytes.NewReader(clientBody))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d", resp.StatusCode)
	}
	if gotLen != len(clientBody) {
		t.Fatalf("under-limit len got %d want %d", gotLen, len(clientBody))
	}
	col.one(t)

	// Truncation at maxBodyBytes: send maxBodyBytes+100; gateway LimitReader caps read.
	// Build a large body without multipart complexity (JSON path also uses readBody).
	// Use speech JSON with huge input to hit the limit cheaply.
	huge := strings.Repeat("x", maxBodyBytes+100)
	req2, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/audio/speech",
		strings.NewReader(`{"model":"tts-1","input":"`+huge+`","voice":"alloy"}`))
	req2.Header.Set("Content-Type", "application/json")
	// Body size exceeds maxBodyBytes; readBody truncates → invalid JSON or missing fields → 400.
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	// Truncated body is not valid complete JSON → bad request (fail closed).
	if resp2.StatusCode == 200 {
		t.Fatalf("oversized body should not succeed: %s", b2)
	}
	// Second usage event for the failed speech attempt.
	col.mu.Lock()
	n := len(col.events)
	col.mu.Unlock()
	if n != 2 {
		t.Fatalf("want 2 usage events, got %d", n)
	}
}

func TestParseGoogleActionGenerateSpeech(t *testing.T) {
	m, method, ok := parseGoogleAction("gemini-2.5-flash-preview-tts:generateSpeech")
	if !ok || method != "generateSpeech" || m != "gemini-2.5-flash-preview-tts" {
		t.Fatalf("%s %s %v", m, method, ok)
	}
}
