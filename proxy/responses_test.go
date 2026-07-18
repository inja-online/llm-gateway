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

func TestResponsesCreatePassthrough(t *testing.T) {
	var gotPath, gotAuth, gotModel, gotOrg, gotProj string
	var gotRL bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotOrg = r.Header.Get("OpenAI-Organization")
		gotProj = r.Header.Get("OpenAI-Project")
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		gotModel, _ = body["model"].(string)
		// Preserve unknown fields
		if _, ok := body["input"]; !ok {
			t.Error("missing input field")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-ratelimit-remaining-requests", "99")
		w.Header().Set("x-request-id", "up_req_1")
		gotRL = true
		fmt.Fprint(w, `{"id":"resp_1","object":"response","model":"gpt-4o","usage":{"input_tokens":10,"output_tokens":4},"output":[]}`)
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

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/responses",
		strings.NewReader(`{"model":"openai/gpt-4o","input":"hi","temperature":0.2}`))
	req.Header.Set("Authorization", "Bearer sk-resp")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OpenAI-Organization", "org-abc")
	req.Header.Set("OpenAI-Project", "proj-xyz")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if gotPath != "/responses" || gotModel != "gpt-4o" || gotAuth != "Bearer sk-resp" {
		t.Fatalf("path=%s model=%s auth=%s", gotPath, gotModel, gotAuth)
	}
	if gotOrg != "org-abc" || gotProj != "proj-xyz" {
		t.Fatalf("org/project not forwarded: %q %q", gotOrg, gotProj)
	}
	if !gotRL || resp.Header.Get("X-Ratelimit-Remaining-Requests") != "99" {
		t.Fatalf("rate-limit header not passed: %v", resp.Header)
	}
	if resp.Header.Get("X-Request-Id") != "up_req_1" {
		t.Fatalf("x-request-id: %q", resp.Header.Get("X-Request-Id"))
	}
	if resp.Header.Get("X-Gateway-Request-Id") == "" {
		t.Fatal("missing X-Gateway-Request-Id")
	}
	if !strings.Contains(string(body), "resp_1") {
		t.Fatalf("%s", body)
	}
	ev := col.one(t)
	if ev.TokensIn != 10 || ev.TokensOut != 4 || ev.Estimated || ev.Status != hooks.StatusOK {
		t.Fatalf("%+v", ev)
	}
	if ev.Provider != "openai" || ev.UpstreamModel != "gpt-4o" {
		t.Fatalf("%+v", ev)
	}
}

func TestResponsesMissingModel(t *testing.T) {
	cfg, _ := config.Parse([]byte(`providers: { openai: { kind: openai, base_url: "http://x" } }
defaults: { openai_dialect: openai }`))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	resp, err := http.Post(gw.URL+"/v1/responses", "application/json", strings.NewReader(`{"input":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
}

func TestResponsesRejectsAnthropic(t *testing.T) {
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
	resp, err := http.Post(gw.URL+"/v1/responses", "application/json",
		strings.NewReader(`{"model":"anthropic/claude","input":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
}

func TestResponsesUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-ratelimit-remaining-requests", "0")
		w.WriteHeader(429)
		fmt.Fprint(w, `{"error":{"message":"slow down","type":"rate_limit_error"}}`)
	}))
	gw, col := newTestGateway(t, upstream)
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/responses",
		strings.NewReader(`{"model":"up/m","input":"x"}`))
	req.Header.Set("Authorization", "Bearer k")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 429 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if resp.Header.Get("X-Ratelimit-Remaining-Requests") != "0" {
		t.Fatalf("headers %v", resp.Header)
	}
	ev := col.one(t)
	if ev.Status != hooks.StatusUpstreamError {
		t.Fatalf("%+v", ev)
	}
}

func TestResponsesStream(t *testing.T) {
	sseBody := "" +
		"event: response.created\n" +
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_s\"}}\n\n" +
		"event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\n" +
		"event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":3,\"output_tokens\":2}}}\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["stream"] != true {
			t.Errorf("stream flag %v", body["stream"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("x-ratelimit-limit-requests", "500")
		fmt.Fprint(w, sseBody)
	}))
	gw, col := newTestGateway(t, upstream)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/responses",
		strings.NewReader(`{"model":"up/m","input":"x","stream":true}`))
	req.Header.Set("Authorization", "Bearer k")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, got)
	}
	if string(got) != sseBody {
		t.Fatalf("framing mismatch:\n got %q\nwant %q", got, sseBody)
	}
	if resp.Header.Get("X-Ratelimit-Limit-Requests") != "500" {
		t.Fatalf("rl header %v", resp.Header)
	}
	ev := col.one(t)
	if ev.TokensIn != 3 || ev.TokensOut != 2 || ev.Estimated || !ev.Stream {
		t.Fatalf("%+v", ev)
	}
}

func TestResponsesGetDelete(t *testing.T) {
	var gets, dels int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/responses/resp_9":
			gets++
			fmt.Fprint(w, `{"id":"resp_9","status":"completed"}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/responses/resp_9":
			dels++
			fmt.Fprint(w, `{"id":"resp_9","deleted":true}`)
		default:
			t.Errorf("%s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
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

	resp, err := http.Get(gw.URL + "/v1/responses/resp_9?provider=openai")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(b), "completed") {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if col.one(t).Estimated != true {
		t.Fatal("get should be estimated")
	}

	col2 := &collector{}
	gw2 := httptest.NewServer(NewServer(cfg, col2).Handler())
	t.Cleanup(gw2.Close)
	req, _ := http.NewRequest(http.MethodDelete, gw2.URL+"/v1/responses/resp_9", nil)
	req.Header.Set("X-Provider", "openai")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 200 || !strings.Contains(string(b2), "deleted") {
		t.Fatalf("%d %s", resp2.StatusCode, b2)
	}
	if gets != 1 || dels != 1 {
		t.Fatalf("gets=%d dels=%d", gets, dels)
	}
	col2.one(t)
}

func TestResponsesGetMissingProvider(t *testing.T) {
	cfg, _ := config.Parse([]byte(`providers: { openai: { kind: openai, base_url: "http://x" } }`))
	// no default dialect
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	resp, err := http.Get(gw.URL + "/v1/responses/r1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
}

func TestExtractResponsesUsage(t *testing.T) {
	ev := &hooks.UsageEvent{}
	if extractResponsesUsage([]byte(`{"type":"response.created"}`), ev) {
		t.Fatal("no usage")
	}
	if !extractResponsesUsage([]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":1,"output_tokens":2}}}`), ev) {
		t.Fatal("want usage")
	}
	if ev.TokensIn != 1 || ev.TokensOut != 2 {
		t.Fatalf("%+v", ev)
	}
}

func TestApplyResponsesUsageVariants(t *testing.T) {
	ev := &hooks.UsageEvent{}
	applyResponsesUsage([]byte(`not-json`), ev)
	if !ev.Estimated {
		t.Fatal("want estimated")
	}
	ev = &hooks.UsageEvent{}
	applyResponsesUsage([]byte(`{"usage":{"prompt_tokens":5,"completion_tokens":6}}`), ev)
	if ev.TokensIn != 5 || ev.TokensOut != 6 || ev.Estimated {
		t.Fatalf("%+v", ev)
	}
	ev = &hooks.UsageEvent{}
	applyResponsesUsage([]byte(`{"usage":{}}`), ev)
	if !ev.Estimated {
		t.Fatal("empty usage estimated")
	}
}

func TestResponsesTLSNotRequiredForHTTP(t *testing.T) {
	// ensure unknown provider on GET
	cfg, _ := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "http://x" }
defaults: { openai_dialect: openai }
`))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	resp, err := http.Get(gw.URL + "/v1/responses/r1?provider=nope")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
}

func TestResponsesGetNonOpenAIProvider(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  anthropic: { kind: anthropic, base_url: "http://x" }
  openai: { kind: openai, base_url: "http://y" }
defaults:
  openai_dialect: openai
`))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	resp, err := http.Get(gw.URL + "/v1/responses/r1?provider=anthropic")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
}

func TestResponsesInvalidJSON(t *testing.T) {
	// valid model peek fails when completely invalid - unmarshal head fails
	cfg, _ := config.Parse([]byte(`providers: { openai: { kind: openai, base_url: "http://x" } }
defaults: { openai_dialect: openai }`))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	resp, err := http.Post(gw.URL+"/v1/responses", "application/json", strings.NewReader(`[`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
}
