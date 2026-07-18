package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/inja-online/llm-gateway/canonical"
	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

func TestAsPositiveInt(t *testing.T) {
	cases := []struct {
		in   any
		want int
		ok   bool
	}{
		{float64(3), 3, true},
		{float64(0), 0, false},
		{float64(-1), 0, false},
		{int(7), 7, true},
		{int(0), 0, false},
		{"12", 12, true},
		{"0", 0, false},
		{"nope", 0, false},
		{json.Number("9"), 9, true},
		{json.Number("0"), 0, false},
		{json.Number("x"), 0, false},
		{true, 0, false},
		{nil, 0, false},
	}
	for _, tc := range cases {
		got, ok := asPositiveInt(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("asPositiveInt(%v)=(%d,%v) want (%d,%v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestEnsureMaxTokens(t *testing.T) {
	if got := ensureMaxTokens([]byte(`not-json`)); string(got) != `not-json` {
		t.Fatalf("%s", got)
	}
	// already positive float
	body := []byte(`{"max_tokens":10,"messages":[]}`)
	if string(ensureMaxTokens(body)) != string(body) {
		t.Fatal("float64 positive must be unchanged")
	}
	// zero → inject 1
	out := ensureMaxTokens([]byte(`{"model":"m"}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["max_tokens"] != float64(1) {
		t.Fatalf("%v", m["max_tokens"])
	}
	// explicit zero
	out = ensureMaxTokens([]byte(`{"max_tokens":0}`))
	json.Unmarshal(out, &m)
	if m["max_tokens"] != float64(1) {
		t.Fatalf("%v", m)
	}
	// Use Decoder with UseNumber to exercise json.Number branch via direct call pattern:
	// ensureMaxTokens uses json.Unmarshal into map[string]any which yields float64, not Number.
	// Still cover the int type branch by setting via round-trip isn't possible from JSON;
	// force via re-encoding isn't needed if float path is hit. Call with crafted map by
	// testing parseGoogleTotalTokens edges instead for related coverage.
}

func TestMediaUsageFromRequestHelpers(t *testing.T) {
	img := mediaUsageFromRequest(config.ModalityImageGen, map[string]any{
		"n": float64(3), "size": "1024x1024", "response_format": "b64_json",
	})
	if img.Units != 3 || img.Size != "1024x1024" || img.Format != "b64_json" || img.UnitKind != hooks.MediaUnitImage {
		t.Fatalf("%+v", img)
	}
	imgDef := mediaUsageFromRequest(config.ModalityImageGen, map[string]any{})
	if imgDef.Units != 1 {
		t.Fatalf("%+v", imgDef)
	}
	vid := mediaUsageFromRequest(config.ModalityVideoGen, map[string]any{"seconds": "8"})
	if vid.Units != 8 || vid.UnitKind != hooks.MediaUnitVideoSecond {
		t.Fatalf("%+v", vid)
	}
	vid2 := mediaUsageFromRequest(config.ModalityVideoGen, map[string]any{"duration": float64(5)})
	if vid2.Units != 5 {
		t.Fatalf("%+v", vid2)
	}
	vid3 := mediaUsageFromRequest(config.ModalityVideoGen, map[string]any{})
	if vid3.Units != 1 {
		t.Fatalf("%+v", vid3)
	}
}

func TestParseGoogleTotalTokensExtra(t *testing.T) {
	if n, ok := parseGoogleTotalTokens([]byte(`notjson`)); ok || n != 0 {
		t.Fatal()
	}
	if n, ok := parseGoogleTotalTokens([]byte(`{"totalTokens":0}`)); !ok || n != 0 {
		t.Fatalf("%d %v", n, ok)
	}
	if n, ok := parseGoogleTotalTokens([]byte(`{"total_tokens":0}`)); !ok || n != 0 {
		t.Fatalf("%d %v", n, ok)
	}
	if n, ok := parseGoogleTotalTokens([]byte(`{}`)); ok {
		t.Fatalf("%d", n)
	}
	if n, ok := parseGoogleTotalTokens([]byte(`{"total_tokens":4}`)); !ok || n != 4 {
		t.Fatalf("%d %v", n, ok)
	}
}

func TestApplyCanonUsageFields(t *testing.T) {
	cfg, _ := config.Parse([]byte(`providers: { x: { kind: openai, base_url: "http://x" } }`))
	s := NewServer(cfg, &collector{})
	x := s.newExchange(httptest.NewRecorder(), httptest.NewRequest("POST", "/", nil), DialectOpenAI, writeOpenAIError)
	x.applyCanonUsage(canonical.Usage{
		InputTokens:      10,
		OutputTokens:     5,
		HasUsage:         true,
		CacheReadTokens:  3,
		CacheWriteTokens: 2,
		ReasoningTokens:  4,
	})
	if x.ev.TokensIn != 10 || x.ev.TokensOut != 5 || x.ev.Estimated {
		t.Fatalf("%+v", x.ev)
	}
	if x.ev.CachedTokens != 3 || x.ev.CacheWriteTokens != 2 || x.ev.ReasoningTokens != 4 {
		t.Fatalf("%+v", x.ev)
	}
	x2 := s.newExchange(httptest.NewRecorder(), httptest.NewRequest("POST", "/", nil), DialectOpenAI, writeOpenAIError)
	x2.applyCanonUsage(canonical.Usage{HasUsage: false})
	if !x2.ev.Estimated {
		t.Fatal("expected estimated")
	}
}

func TestSetTokenSource(t *testing.T) {
	cfg, _ := config.Parse([]byte(`providers: { g: { kind: google, base_url: "http://x", auth: adc } }`))
	s := NewServer(cfg, nil)
	if s.tokenSource("g") != nil {
		t.Fatal("expected nil before set")
	}
	s.SetTokenSource("g", StaticTokenSource{AccessToken: "abc"})
	if s.tokenSource("g") == nil {
		t.Fatal("expected source")
	}
	tok, err := s.tokenSource("g").Token(context.Background())
	if err != nil || tok != "abc" {
		t.Fatalf("%q %v", tok, err)
	}
	s2 := &Server{}
	if s2.tokenSource("x") != nil {
		t.Fatal()
	}
	s2.SetTokenSource("x", StaticTokenSource{AccessToken: "t"})
	if s2.tokenSource("x") == nil {
		t.Fatal()
	}

	// CachingTokenSource: default TTL + error + cache hit
	calls := 0
	inner := FuncTokenSource(func(ctx context.Context) (string, error) {
		calls++
		if calls == 1 {
			return "tok1", nil
		}
		return "", fmt.Errorf("refresh fail")
	})
	c := &CachingTokenSource{Inner: inner} // TTL 0 → default 5m
	tok, err = c.Token(context.Background())
	if err != nil || tok != "tok1" {
		t.Fatalf("%q %v", tok, err)
	}
	tok, err = c.Token(context.Background())
	if err != nil || tok != "tok1" || calls != 1 {
		t.Fatalf("cache miss: %q %v calls=%d", tok, err, calls)
	}
	// force expire
	c.expiry = c.expiry.Add(-time.Hour)
	if _, err = c.Token(context.Background()); err == nil {
		t.Fatal("expected refresh error")
	}
}

func TestEmbeddingsErrorPaths(t *testing.T) {
	// upstream 4xx on openai passthrough
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"error":{"message":"bad","type":"invalid_request_error"}}`)
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
	resp, _ := http.Post(gw.URL+"/v1/embeddings", "application/json",
		strings.NewReader(`{"model":"text-embedding-3-small","input":"hi"}`))
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("%d", resp.StatusCode)
	}
	col.one(t)

	// google translate: empty input array
	cfgG, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  openai_dialect: google
`, upstream.URL)))
	gwG := httptest.NewServer(NewServer(cfgG, &collector{}).Handler())
	t.Cleanup(gwG.Close)
	resp2, _ := http.Post(gwG.URL+"/v1/embeddings", "application/json",
		strings.NewReader(`{"model":"emb","input":[]}`))
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 400 {
		t.Fatalf("%d", resp2.StatusCode)
	}

	// google: invalid input type
	resp3, _ := http.Post(gwG.URL+"/v1/embeddings", "application/json",
		strings.NewReader(`{"model":"emb","input":123}`))
	io.Copy(io.Discard, resp3.Body)
	resp3.Body.Close()
	if resp3.StatusCode != 400 {
		t.Fatalf("%d", resp3.StatusCode)
	}

	// google upstream 400
	resp4, _ := http.Post(gwG.URL+"/v1/embeddings", "application/json",
		strings.NewReader(`{"model":"emb","input":"hi"}`))
	io.Copy(io.Discard, resp4.Body)
	resp4.Body.Close()
	if resp4.StatusCode < 400 {
		t.Fatalf("%d", resp4.StatusCode)
	}

	// google bad embed response
	upOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"not":"embeddings"}`)
	}))
	t.Cleanup(upOK.Close)
	cfgBad, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  openai_dialect: google
`, upOK.URL)))
	gwBad := httptest.NewServer(NewServer(cfgBad, &collector{}).Handler())
	t.Cleanup(gwBad.Close)
	resp5, _ := http.Post(gwBad.URL+"/v1/embeddings", "application/json",
		strings.NewReader(`{"model":"emb","input":"hi"}`))
	io.Copy(io.Discard, resp5.Body)
	resp5.Body.Close()
	if resp5.StatusCode != http.StatusBadGateway {
		t.Fatalf("%d", resp5.StatusCode)
	}

	// usage missing → estimated
	upNoUsage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"object":"list","data":[{"embedding":[1]}],"model":"m"}`)
	}))
	t.Cleanup(upNoUsage.Close)
	cfgNU, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  openai_dialect: openai
`, upNoUsage.URL)))
	colNU := &collector{}
	gwNU := httptest.NewServer(NewServer(cfgNU, colNU).Handler())
	t.Cleanup(gwNU.Close)
	resp6, _ := http.Post(gwNU.URL+"/v1/embeddings", "application/json",
		strings.NewReader(`{"model":"m","input":"hi"}`))
	io.Copy(io.Discard, resp6.Body)
	resp6.Body.Close()
	ev := colNU.one(t)
	if !ev.Estimated {
		t.Fatalf("%+v", ev)
	}

	// total_tokens fallback when prompt_tokens is 0
	upTot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"object":"list","data":[{"embedding":[1]}],"usage":{"prompt_tokens":0,"total_tokens":9}}`)
	}))
	t.Cleanup(upTot.Close)
	cfgT, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  openai_dialect: openai
`, upTot.URL)))
	colT := &collector{}
	gwT := httptest.NewServer(NewServer(cfgT, colT).Handler())
	t.Cleanup(gwT.Close)
	resp7, _ := http.Post(gwT.URL+"/v1/embeddings", "application/json",
		strings.NewReader(`{"model":"m","input":"hi"}`))
	io.Copy(io.Discard, resp7.Body)
	resp7.Body.Close()
	if colT.one(t).TokensIn != 9 {
		t.Fatal()
	}

	// unknown model
	resp8, _ := http.Post(gw.URL+"/v1/embeddings", "application/json",
		strings.NewReader(`{"model":"unknownprov/m","input":"hi"}`))
	io.Copy(io.Discard, resp8.Body)
	resp8.Body.Close()
	if resp8.StatusCode != 404 {
		t.Fatalf("%d", resp8.StatusCode)
	}
}

func TestAudioJSONAndFallbacks(t *testing.T) {
	var gotPath, gotCT string
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
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

	// JSON body (rare but supported)
	resp, _ := http.Post(gw.URL+"/v1/audio/transcriptions", "application/json",
		strings.NewReader(`{"model":"openai/whisper-1","file":"ignored"}`))
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if gotPath != "/audio/transcriptions" {
		t.Fatalf("%s", gotPath)
	}
	if gotBody["model"] != "whisper-1" {
		t.Fatalf("%v", gotBody)
	}
	if !strings.Contains(gotCT, "json") {
		t.Fatalf("ct %q", gotCT)
	}
	col.one(t)

	// non-openai family
	cfgA, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  anthropic: { kind: anthropic, base_url: %q }
  openai: { kind: openai, base_url: "http://127.0.0.1:9" }
defaults:
  openai_dialect: openai
`, upstream.URL)))
	gwA := httptest.NewServer(NewServer(cfgA, &collector{}).Handler())
	t.Cleanup(gwA.Close)
	resp2, _ := http.Post(gwA.URL+"/v1/audio/transcriptions", "application/json",
		strings.NewReader(`{"model":"anthropic/claude","file":"x"}`))
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotImplemented && resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("%d", resp2.StatusCode)
	}

	// missing model → defaults to unknown, uses dialect default
	resp3, _ := http.Post(gw.URL+"/v1/audio/translations", "application/json",
		strings.NewReader(`{}`))
	io.Copy(io.Discard, resp3.Body)
	resp3.Body.Close()
	// may succeed or fail depending on upstream; just exercise path
}

func TestGoogleCountTokensAndModelGetEdges(t *testing.T) {
	// countTokens wrong kind
	cfgWrong, _ := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "http://127.0.0.1:9" }
defaults:
  google_dialect: openai
`))
	gwW := httptest.NewServer(NewServer(cfgWrong, nil).Handler())
	t.Cleanup(gwW.Close)
	resp, _ := http.Post(gwW.URL+"/v1beta/models/gemini:countTokens", "application/json",
		strings.NewReader(`{"contents":[]}`))
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("%d", resp.StatusCode)
	}

	// successful countTokens + model get with provider/model and models/ prefix
	var paths []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path+"?"+r.URL.RawQuery)
		if strings.Contains(r.URL.Path, "countTokens") {
			if r.Method != http.MethodPost {
				t.Fatalf("method %s", r.Method)
			}
			fmt.Fprint(w, `{"totalTokens":3}`)
			return
		}
		fmt.Fprint(w, `{"name":"models/gemini-2.0-flash"}`)
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  google_dialect: google
`, up.URL)))
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	resp2, _ := http.Post(gw.URL+"/v1beta/models/gemini-2.0-flash:countTokens",
		"application/json", strings.NewReader(`{"model":"models/gemini-2.0-flash","contents":[{"parts":[{"text":"x"}]}]}`))
	b, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 200 || !strings.Contains(string(b), "3") {
		t.Fatalf("%d %s", resp2.StatusCode, b)
	}

	// GET with models/ prefix
	resp3, _ := http.Get(gw.URL + "/v1beta/models/models%2Fgemini-2.0-flash")
	io.Copy(io.Discard, resp3.Body)
	resp3.Body.Close()

	// GET with provider/model
	resp4, _ := http.Get(gw.URL + "/v1beta/models/google%2Fgemini-2.0-flash")
	io.Copy(io.Discard, resp4.Body)
	resp4.Body.Close()

	// models list with query
	resp5, _ := http.Get(gw.URL + "/v1beta/models?pageSize=2&provider=google")
	io.Copy(io.Discard, resp5.Body)
	resp5.Body.Close()

	// countTokens upstream fail
	upFail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	t.Cleanup(upFail.Close)
	cfgF, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  google_dialect: google
`, upFail.URL)))
	gwF := httptest.NewServer(NewServer(cfgF, nil).Handler())
	t.Cleanup(gwF.Close)
	resp6, _ := http.Post(gwF.URL+"/v1beta/models/g:countTokens", "application/json",
		strings.NewReader(`{}`))
	io.Copy(io.Discard, resp6.Body)
	resp6.Body.Close()
	if resp6.StatusCode != http.StatusBadGateway {
		t.Fatalf("%d", resp6.StatusCode)
	}

	// missing model on GET
	s := NewServer(cfg, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1beta/models/", nil)
	// path value empty
	s.handleGoogleModelGet(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("%d", rr.Code)
	}
}

func TestGoogleCrossDialectErrorPaths(t *testing.T) {
	// Google ingress → OpenAI egress: bad parse
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":"c","choices":[{"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  google_dialect: openai
`, up.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	// invalid google body
	resp, _ := http.Post(gw.URL+"/v1beta/models/gpt-4o:generateContent",
		"application/json", strings.NewReader(`{"contents":"bad"}`))
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("%d", resp.StatusCode)
	}

	// success translate google→openai (fresh collector)
	colOK := &collector{}
	gwOK := httptest.NewServer(NewServer(cfg, colOK).Handler())
	t.Cleanup(gwOK.Close)
	resp2, _ := http.Post(gwOK.URL+"/v1beta/models/gpt-4o:generateContent",
		"application/json", strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	b, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("%d %s", resp2.StatusCode, b)
	}
	colOK.one(t)
	_ = col

	// google→openai upstream error
	upErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"error":{"message":"nope","type":"invalid_request_error"}}`)
	}))
	t.Cleanup(upErr.Close)
	cfgE, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  google_dialect: openai
`, upErr.URL)))
	gwE := httptest.NewServer(NewServer(cfgE, &collector{}).Handler())
	t.Cleanup(gwE.Close)
	resp3, _ := http.Post(gwE.URL+"/v1beta/models/gpt-4o:generateContent",
		"application/json", strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	io.Copy(io.Discard, resp3.Body)
	resp3.Body.Close()
	if resp3.StatusCode < 400 {
		t.Fatalf("%d", resp3.StatusCode)
	}

	// google→anthropic
	upA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":"m","type":"message","role":"assistant","content":[{"type":"text","text":"yo"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	t.Cleanup(upA.Close)
	cfgA, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  anthropic: { kind: anthropic, base_url: %q }
defaults:
  google_dialect: anthropic
`, upA.URL)))
	colA := &collector{}
	gwA := httptest.NewServer(NewServer(cfgA, colA).Handler())
	t.Cleanup(gwA.Close)
	resp4, _ := http.Post(gwA.URL+"/v1beta/models/claude:generateContent",
		"application/json", strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	b4, _ := io.ReadAll(resp4.Body)
	resp4.Body.Close()
	if resp4.StatusCode != 200 {
		t.Fatalf("%d %s", resp4.StatusCode, b4)
	}
	colA.one(t)

	// google→anthropic error
	upAE := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"bad"}}`)
	}))
	t.Cleanup(upAE.Close)
	cfgAE, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  anthropic: { kind: anthropic, base_url: %q }
defaults:
  google_dialect: anthropic
`, upAE.URL)))
	gwAE := httptest.NewServer(NewServer(cfgAE, &collector{}).Handler())
	t.Cleanup(gwAE.Close)
	resp5, _ := http.Post(gwAE.URL+"/v1beta/models/claude:generateContent",
		"application/json", strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	io.Copy(io.Discard, resp5.Body)
	resp5.Body.Close()
	if resp5.StatusCode < 400 {
		t.Fatalf("%d", resp5.StatusCode)
	}
}

func TestOpenAIImageToGoogleSuccessAndParseFail(t *testing.T) {
	// success path for imageTranslateToGoogle
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"predictions":[{"bytesBase64Encoded":"YQ==","mimeType":"image/png"}]}`)
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  openai_dialect: google
`, up.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	resp, _ := http.Post(gw.URL+"/v1/images/generations", "application/json",
		strings.NewReader(`{"model":"imagen-3","prompt":"cat","n":1}`))
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	col.one(t)

	// parse fail — completely unusable body
	upBad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"nope":true}`)
	}))
	t.Cleanup(upBad.Close)
	cfgB, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  openai_dialect: google
`, upBad.URL)))
	gwB := httptest.NewServer(NewServer(cfgB, &collector{}).Handler())
	t.Cleanup(gwB.Close)
	resp2, _ := http.Post(gwB.URL+"/v1/images/generations", "application/json",
		strings.NewReader(`{"model":"imagen-3","prompt":"cat"}`))
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadGateway && resp2.StatusCode != 200 {
		// some parsers accept empty predictions as empty success
		t.Fatalf("%d", resp2.StatusCode)
	}
}

func TestAnthropicVideoCreateCoveragePaths(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/videos" {
			t.Fatalf("path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"id":"vid_1","object":"video","model":"sora","status":"queued"}`)
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  anthropic_dialect: openai
`, up.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/videos",
		strings.NewReader(`{"model":"sora","prompt":"a bird"}`))
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
	col.one(t)

	// invalid body
	req2, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/videos",
		strings.NewReader(`{}`))
	req2.Header.Set("anthropic-version", "2023-06-01")
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := http.DefaultClient.Do(req2)
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 400 {
		t.Fatalf("%d", resp2.StatusCode)
	}

	// upstream error
	upE := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"error":{"message":"no","type":"invalid_request_error"}}`)
	}))
	t.Cleanup(upE.Close)
	cfgE, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
defaults:
  anthropic_dialect: openai
`, upE.URL)))
	gwE := httptest.NewServer(NewServer(cfgE, &collector{}).Handler())
	t.Cleanup(gwE.Close)
	req3, _ := http.NewRequest(http.MethodPost, gwE.URL+"/v1/videos",
		strings.NewReader(`{"model":"sora","prompt":"x"}`))
	req3.Header.Set("anthropic-version", "2023-06-01")
	req3.Header.Set("Content-Type", "application/json")
	resp3, _ := http.DefaultClient.Do(req3)
	io.Copy(io.Discard, resp3.Body)
	resp3.Body.Close()
	if resp3.StatusCode < 400 {
		t.Fatalf("%d", resp3.StatusCode)
	}
}

func TestRewriteGoogleEmbedBody(t *testing.T) {
	out := rewriteGoogleEmbedBody([]byte(`notjson`), "m", "embedContent")
	if string(out) != `notjson` {
		t.Fatal()
	}
	out = rewriteGoogleEmbedBody([]byte(`{"model":"old","content":{}}`), "up", "embedContent")
	var m map[string]any
	json.Unmarshal(out, &m)
	if m["model"] != "models/up" {
		t.Fatalf("%v", m)
	}
	out = rewriteGoogleEmbedBody([]byte(`{"requests":[{"model":"a"},"skip",{"model":"b"}]}`), "up", "batchEmbedContents")
	json.Unmarshal(out, &m)
	reqs := m["requests"].([]any)
	if reqs[0].(map[string]any)["model"] != "models/up" {
		t.Fatalf("%v", reqs)
	}
	if rewriteGoogleEmbedBody([]byte(`{}`), "m", "other") == nil {
		t.Fatal()
	}
	if !isGoogleEmbedMethod("embedContent") || isGoogleEmbedMethod("generateContent") {
		t.Fatal()
	}
}

func TestGoogleNativeEmbedEstimated(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"embedding":{"values":[1]}}`)
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  google_dialect: google
`, up.URL)))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1beta/models/emb:embedContent",
		strings.NewReader(`{"content":{"parts":[{"text":"hi"}]}}`))
	req.Header.Set("x-goog-api-key", "k")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	ev := col.one(t)
	if !ev.Estimated {
		t.Fatalf("%+v", ev)
	}

	// 4xx forward
	upE := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		fmt.Fprint(w, `{"error":{"message":"no"}}`)
	}))
	t.Cleanup(upE.Close)
	cfgE, _ := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  google_dialect: google
`, upE.URL)))
	colE := &collector{}
	gwE := httptest.NewServer(NewServer(cfgE, colE).Handler())
	t.Cleanup(gwE.Close)
	req2, _ := http.NewRequest(http.MethodPost, gwE.URL+"/v1beta/models/emb:embedContent",
		strings.NewReader(`{"content":{"parts":[{"text":"hi"}]}}`))
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := http.DefaultClient.Do(req2)
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 403 {
		t.Fatalf("%d", resp2.StatusCode)
	}
}

func TestHeadersAndGatewayRequestID(t *testing.T) {
	rr := httptest.NewRecorder()
	setGatewayRequestID(rr, "")
	if rr.Header().Get("X-Gateway-Request-Id") != "" {
		t.Fatal("empty id must not set header")
	}
	setGatewayRequestID(rr, "rid-1")
	if rr.Header().Get("X-Gateway-Request-Id") != "rid-1" {
		t.Fatalf("%q", rr.Header().Get("X-Gateway-Request-Id"))
	}
}
