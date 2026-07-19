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

	"github.com/inja-online/llm-gateway/canonical"
	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

// --- pure helpers (audio + exchange) ---

func TestContentTypeForSpeechFormat(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"mp3", "audio/mpeg"},
		{"MP3", "audio/mpeg"},
		{"opus", "audio/opus"},
		{"aac", "audio/aac"},
		{"flac", "audio/flac"},
		{"wav", "audio/wav"},
		{"pcm", "audio/L16"},
		{"ogg", "audio/ogg"},
		{"", "application/octet-stream"},
	}
	for _, tc := range cases {
		if got := contentTypeForSpeechFormat(tc.in); got != tc.want {
			t.Errorf("contentTypeForSpeechFormat(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestSpeechMediaUsage(t *testing.T) {
	if u := speechMediaUsage(nil); u == nil || u.UnitKind != hooks.MediaUnitAudioCharacter || u.Units != 0 {
		t.Fatalf("nil: %+v", u)
	}
	u := speechMediaUsage(&canonical.AudioSpeechRequest{Input: "你好", Format: ""})
	if u.Units != 2 || u.Format != "mp3" {
		t.Fatalf("%+v", u)
	}
	u2 := speechMediaUsage(&canonical.AudioSpeechRequest{Input: "ab", Format: "wav"})
	if u2.Format != "wav" || u2.Units != 2 {
		t.Fatalf("%+v", u2)
	}
}

func TestExchangeBodyLimitAndReadHelpers(t *testing.T) {
	// nil exchange / nil server → package default
	var xNil *exchange
	if xNil.bodyLimit() != maxBodyBytes {
		t.Fatalf("nil exchange bodyLimit=%d", xNil.bodyLimit())
	}
	var sNil *Server
	if sNil.bodyLimit() != maxBodyBytes {
		t.Fatalf("nil server bodyLimit=%d", sNil.bodyLimit())
	}
	s := &Server{} // cfg nil
	if s.bodyLimit() != maxBodyBytes {
		t.Fatalf("nil cfg bodyLimit=%d", s.bodyLimit())
	}

	cfg, err := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "https://api.openai.com/v1" }
max_body_bytes: 128
`))
	if err != nil {
		t.Fatal(err)
	}
	s2 := NewServer(cfg, nil)
	if s2.bodyLimit() != 128 {
		t.Fatalf("configured bodyLimit=%d", s2.bodyLimit())
	}
	x := s2.newExchange(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil), DialectOpenAI, writeOpenAIError)
	if x.bodyLimit() != 128 {
		t.Fatalf("exchange bodyLimit=%d", x.bodyLimit())
	}

	// readAllResp honors exchange limit
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(strings.Repeat("a", 200)))}
	got, err := x.readAllResp(resp)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 128 {
		t.Fatalf("readAllResp len=%d want 128", len(got))
	}

	// readAllLimited uses package default
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hello-limited"))
	b, err := readAllLimited(req)
	if err != nil || string(b) != "hello-limited" {
		t.Fatalf("readAllLimited: %q %v", b, err)
	}

	// readAllLimitedN: positive limit + zero → default
	req2 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("b", 50)))
	b2, err := readAllLimitedN(req2, 10)
	if err != nil || len(b2) != 10 {
		t.Fatalf("readAllLimitedN(10): len=%d err=%v", len(b2), err)
	}
	req3 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("abc"))
	b3, err := readAllLimitedN(req3, 0)
	if err != nil || string(b3) != "abc" {
		t.Fatalf("readAllLimitedN(0): %q %v", b3, err)
	}
	req4 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("xyz"))
	b4, err := readAllLimitedN(req4, -5)
	if err != nil || string(b4) != "xyz" {
		t.Fatalf("readAllLimitedN(-5): %q %v", b4, err)
	}
}

func TestResolveCatalogProviderEdges(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "https://api.openai.com/v1" }
aliases:
  gpt: openai/gpt-4o
`))
	if err != nil {
		t.Fatal(err)
	}
	// bare id without slash → no provider
	if _, ok := resolveCatalogProvider(cfg, "gpt-4o"); ok {
		t.Fatal("expected no provider for bare id")
	}
	// empty provider prefix
	if _, ok := resolveCatalogProvider(cfg, "/model"); ok {
		t.Fatal("expected no provider for empty prefix")
	}
	// known provider/model
	if p, ok := resolveCatalogProvider(cfg, "openai/gpt-4o"); !ok || p.Kind != config.KindOpenAI {
		t.Fatalf("want openai provider, ok=%v kind=%s", ok, p.Kind)
	}
	// alias → target provider
	if p, ok := resolveCatalogProvider(cfg, "gpt"); !ok || p.Kind != config.KindOpenAI {
		t.Fatalf("alias resolve: ok=%v kind=%s", ok, p.Kind)
	}
	// empty target is skipped in catalog (no second entry)
	cfg.Aliases["empty"] = ""
	catalog := buildModelsCatalog(cfg)
	var sawEmpty bool
	for _, m := range catalog {
		if m.ID == "empty" {
			sawEmpty = true
			if m.Capabilities != nil {
				t.Errorf("empty alias should omit capabilities")
			}
		}
	}
	if !sawEmpty {
		t.Fatal("expected empty alias key in catalog")
	}
}

// --- audio path error / edge coverage ---

func TestOpenAISpeechToGoogleEmptyMIMEDefaultsL16(t *testing.T) {
	// Empty mimeType in wire → egress defaults to audio/L16 (parser); binary still returned.
	pcm := []byte{0x01, 0x02}
	b64 := base64.StdEncoding.EncodeToString(pcm)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"","data":%q}}]}}]}`, b64)
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

	resp, err := http.Post(gw.URL+"/v1/audio/speech", "application/json",
		strings.NewReader(`{"model":"gemini-tts","input":"Hi","voice":"alloy","response_format":"wav"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if !bytes.Equal(body, pcm) {
		t.Fatalf("audio mismatch")
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "audio") {
		t.Fatalf("content-type %q", ct)
	}
	col.one(t)
}

func TestOpenAISpeechToGoogleUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"code":400,"message":"bad voice","status":"INVALID_ARGUMENT"}}`)
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

	resp, err := http.Post(gw.URL+"/v1/audio/speech", "application/json",
		strings.NewReader(`{"model":"gemini-tts","input":"Hi","voice":"alloy"}`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Fatalf("want error status, got %d %s", resp.StatusCode, b)
	}
	col.one(t)
}

func TestOpenAISpeechToGoogleParseFail(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"candidates":[]}`)
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

	resp, err := http.Post(gw.URL+"/v1/audio/speech", "application/json",
		strings.NewReader(`{"model":"gemini-tts","input":"Hi","voice":"alloy"}`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	col.one(t)
}

func TestGoogleSpeechToOpenAIStripsContentTypeParams(t *testing.T) {
	// Content-Type with parameters → mimeType field strips ";..."
	wantAudio := []byte{0xAA, 0xBB}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg; charset=binary")
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
		strings.NewReader(`{"text":"wrap","voice":"alloy","format":"opus"}`))
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
	var env struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					InlineData struct {
						MIMEType string `json:"mimeType"`
						Data     string `json:"data"`
					} `json:"inlineData"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	if env.Candidates[0].Content.Parts[0].InlineData.MIMEType != "audio/mpeg" {
		t.Fatalf("mimeType %q want audio/mpeg (params stripped)", env.Candidates[0].Content.Parts[0].InlineData.MIMEType)
	}
	got, _ := base64.StdEncoding.DecodeString(env.Candidates[0].Content.Parts[0].InlineData.Data)
	if !bytes.Equal(got, wantAudio) {
		t.Fatal("audio mismatch")
	}
	col.one(t)
}

func TestGoogleSpeechToOpenAIUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"bad key","type":"invalid_request_error"}}`)
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
		strings.NewReader(`{"text":"x","voice":"alloy"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	col.one(t)
}

func TestGoogleSpeechInvalidJSON(t *testing.T) {
	cfg, _ := config.Parse([]byte(`
providers:
  google: { kind: google, base_url: "https://example.invalid" }
defaults:
  google_dialect: google
`))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1beta/models/tts:generateSpeech",
		strings.NewReader(`{not-json`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	col.one(t)
}

func TestGoogleSpeechMissingText(t *testing.T) {
	cfg, _ := config.Parse([]byte(`
providers:
  google: { kind: google, base_url: "https://example.invalid" }
defaults:
  google_dialect: google
`))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1beta/models/tts:generateSpeech",
		strings.NewReader(`{"voice":"Kore"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	col.one(t)
}

func TestGoogleSpeechAnthropicProviderDenied(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not call")
	}))
	t.Cleanup(upstream.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  anthropic: { kind: anthropic, base_url: %q }
defaults:
  google_dialect: anthropic
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1beta/models/claude:generateSpeech",
		strings.NewReader(`{"text":"hi","voice":"Kore"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	col.one(t)
}

func TestAnthropicSpeechToOpenAIUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"voice invalid","type":"invalid_request_error"}}`)
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

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/audio/speech",
		strings.NewReader(`{"model":"tts-1","input":"x","voice":"alloy"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	// Anthropic-shaped error
	if !strings.Contains(string(b), "error") {
		t.Fatalf("%s", b)
	}
	col.one(t)
}

func TestAnthropicSpeechInvalidBody(t *testing.T) {
	cfg, _ := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "https://example.invalid" }
defaults:
  anthropic_dialect: openai
`))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/audio/speech",
		strings.NewReader(`{"input":"no model"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	col.one(t)
}

func TestAnthropicAudioJSONTranscribe(t *testing.T) {
	var gotPath, gotCT, gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		var m map[string]any
		json.NewDecoder(r.Body).Decode(&m)
		gotModel, _ = m["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"text":"json stt"}`)
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

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/audio/transcriptions",
		strings.NewReader(`{"model":"whisper-1","language":"en","prompt":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("x-api-key", "sk-ant")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if gotPath != "/audio/transcriptions" {
		t.Fatalf("path %s", gotPath)
	}
	if !strings.Contains(gotCT, "json") {
		t.Fatalf("ct %q", gotCT)
	}
	if gotModel != "whisper-1" {
		t.Fatalf("model %q", gotModel)
	}
	if !strings.Contains(string(b), "json stt") {
		t.Fatalf("%s", b)
	}
	ev := col.one(t)
	if ev.DialectIn != DialectAnthropic || ev.Modality != config.ModalityAudioTranscribe {
		t.Fatalf("%+v", ev)
	}
}

func TestAnthropicAudioJSONTranscribeInvalid(t *testing.T) {
	cfg, _ := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "https://example.invalid" }
defaults:
  anthropic_dialect: openai
`))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/audio/transcriptions",
		strings.NewReader(`{"language":"en"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	col.one(t)
}

func TestAnthropicAudioMultipartUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"bad file","type":"invalid_request_error"}}`)
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

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("model", "whisper-1")
	part, _ := mw.CreateFormFile("file", "x.wav")
	_, _ = part.Write([]byte("RIFF"))
	_ = mw.Close()

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/audio/transcriptions", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	col.one(t)
}

func TestAnthropicAudioMultipartNonOpenAIDenied(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not call")
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

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("model", "whisper-1")
	part, _ := mw.CreateFormFile("file", "x.wav")
	_, _ = part.Write([]byte("RIFF"))
	_ = mw.Close()

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/audio/transcriptions", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	col.one(t)
}

func TestOpenAIAudioMultipartJSON(t *testing.T) {
	var gotModel, gotCT string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		var m map[string]any
		json.NewDecoder(r.Body).Decode(&m)
		gotModel, _ = m["model"].(string)
		fmt.Fprint(w, `{"text":"json openai stt"}`)
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

	resp, err := http.Post(gw.URL+"/v1/audio/transcriptions", "application/json",
		strings.NewReader(`{"model":"openai/whisper-1","language":"en"}`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if gotModel != "whisper-1" {
		t.Fatalf("model rewrite %q", gotModel)
	}
	if !strings.Contains(gotCT, "json") {
		t.Fatalf("ct %q", gotCT)
	}
	col.one(t)
}

func TestOpenAISpeechUnsupportedProviderKind(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not call")
	}))
	t.Cleanup(upstream.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  anthropic: { kind: anthropic, base_url: %q }
defaults:
  openai_dialect: anthropic
`, upstream.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	resp, err := http.Post(gw.URL+"/v1/audio/speech", "application/json",
		strings.NewReader(`{"model":"claude","input":"x","voice":"alloy"}`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	// capability deny (400) or not implemented depending on check order
	if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	col.one(t)
}

func TestOpenAIAudioSpeechInvalidJSON(t *testing.T) {
	cfg, _ := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "https://example.invalid" }
defaults:
  openai_dialect: openai
`))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	resp, err := http.Post(gw.URL+"/v1/audio/speech", "application/json",
		strings.NewReader(`{"model":"tts-1"}`)) // missing input
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	col.one(t)
}

func TestAnthropicSpeechToGoogleBinary(t *testing.T) {
	pcm := []byte{9, 8, 7}
	b64 := base64.StdEncoding.EncodeToString(pcm)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"audio/L16","data":%q}}]}}]}`, b64)
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

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/audio/speech",
		strings.NewReader(`{"model":"gemini-tts","input":"hi","voice":"Kore"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if !bytes.Equal(body, pcm) {
		t.Fatalf("pcm mismatch")
	}
	ev := col.one(t)
	if ev.DialectIn != DialectAnthropic || ev.Provider != "google" {
		t.Fatalf("%+v", ev)
	}
}

func TestCompletionsMissingModelAndUnresolved(t *testing.T) {
	cfg, _ := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "https://example.invalid" }
defaults:
  openai_dialect: openai
`))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	resp, err := http.Post(gw.URL+"/v1/completions", "application/json",
		strings.NewReader(`{"prompt":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing model: %d %s", resp.StatusCode, b)
	}
	col.one(t)

	resp2, err := http.Post(gw.URL+"/v1/completions", "application/json",
		strings.NewReader(`{"model":"no-such-provider/m","prompt":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("unresolved: %d %s", resp2.StatusCode, b2)
	}
	// second event
	col.mu.Lock()
	n := len(col.events)
	col.mu.Unlock()
	if n != 2 {
		t.Fatalf("events=%d", n)
	}
}

func TestCompletionsOversize413(t *testing.T) {
	const limit = 64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not call")
	}))
	t.Cleanup(upstream.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  openai_dialect: openai
max_body_bytes: %d
`, upstream.URL, limit)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	payload := `{"model":"gpt-3.5-turbo-instruct","prompt":"` + strings.Repeat("p", limit) + `"}`
	resp, err := http.Post(gw.URL+"/v1/completions", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	col.one(t)
}
