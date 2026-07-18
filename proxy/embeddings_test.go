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

func TestEmbeddingsPassthroughOpenAI(t *testing.T) {
	var gotPath, gotAuth, gotModel string
	var gotInput any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		gotModel, _ = body["model"].(string)
		gotInput = body["input"]
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.1,0.2]}],"model":"text-embedding-3-small","usage":{"prompt_tokens":4,"total_tokens":4}}`)
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

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/embeddings",
		strings.NewReader(`{"model":"openai/text-embedding-3-small","input":"hello world"}`))
	req.Header.Set("Authorization", "Bearer sk-emb")
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
	if gotPath != "/embeddings" {
		t.Fatalf("path %s", gotPath)
	}
	if gotAuth != "Bearer sk-emb" {
		t.Fatalf("auth %q", gotAuth)
	}
	if gotModel != "text-embedding-3-small" {
		t.Fatalf("model %q", gotModel)
	}
	if gotInput != "hello world" {
		t.Fatalf("input %v", gotInput)
	}
	if !strings.Contains(string(body), `"embedding"`) {
		t.Fatalf("%s", body)
	}
	ev := col.one(t)
	if ev.Modality != ModalityEmbedding {
		t.Fatalf("modality %q", ev.Modality)
	}
	if ev.Provider != "openai" || ev.Status != hooks.StatusOK || ev.TokensIn != 4 || ev.Estimated {
		t.Fatalf("%+v", ev)
	}
	if ev.Model != "openai/text-embedding-3-small" || ev.UpstreamModel != "text-embedding-3-small" {
		t.Fatalf("routing %+v", ev)
	}
}

func TestEmbeddingsPassthroughOpenAICompatArray(t *testing.T) {
	var gotInput any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		gotInput = body["input"]
		fmt.Fprint(w, `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[1]},{"object":"embedding","index":1,"embedding":[2]}],"usage":{"prompt_tokens":6,"total_tokens":6}}`)
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  up: { kind: openai_compat, base_url: %q }
defaults:
  openai_dialect: up
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	resp, err := http.Post(gw.URL+"/v1/embeddings", "application/json",
		strings.NewReader(`{"model":"emb-model","input":["a","b"]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	arr, ok := gotInput.([]any)
	if !ok || len(arr) != 2 {
		t.Fatalf("input %v", gotInput)
	}
	ev := col.one(t)
	if ev.TokensIn != 6 || ev.Modality != ModalityEmbedding {
		t.Fatalf("%+v", ev)
	}
}

func TestEmbeddingsRejectsAnthropic(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not call upstream")
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

	resp, err := http.Post(gw.URL+"/v1/embeddings", "application/json",
		strings.NewReader(`{"model":"anthropic/claude","input":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var env map[string]any
	json.NewDecoder(resp.Body).Decode(&env)
	if _, ok := env["error"]; !ok {
		t.Fatalf("want openai error envelope: %v", env)
	}
	ev := col.one(t)
	if ev.Modality != ModalityEmbedding || ev.Status != hooks.StatusBadRequest {
		t.Fatalf("%+v", ev)
	}
}

func TestEmbeddingsMissingModel(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "https://example.invalid" }
defaults:
  openai_dialect: openai
`))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	resp, err := http.Post(gw.URL+"/v1/embeddings", "application/json",
		strings.NewReader(`{"input":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", resp.StatusCode)
	}
	ev := col.one(t)
	if ev.Status != hooks.StatusBadRequest || ev.Modality != ModalityEmbedding {
		t.Fatalf("%+v", ev)
	}
}

func TestEmbeddingsOpenAIToGoogleSingle(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Header.Get("x-goog-api-key") != "gkey" {
			t.Errorf("auth %q", r.Header.Get("x-goog-api-key"))
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"embedding":{"values":[0.5,0.25]},"usageMetadata":{"promptTokenCount":3}}`)
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  openai_dialect: google
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/embeddings",
		strings.NewReader(`{"model":"gemini-embedding-001","input":"hello","dimensions":8}`))
	req.Header.Set("Authorization", "Bearer gkey")
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
	if gotPath != "/models/gemini-embedding-001:embedContent" {
		t.Fatalf("path %s", gotPath)
	}
	if gotBody["outputDimensionality"] != float64(8) {
		t.Fatalf("dims %v", gotBody["outputDimensionality"])
	}
	content, _ := gotBody["content"].(map[string]any)
	parts, _ := content["parts"].([]any)
	if len(parts) != 1 {
		t.Fatalf("parts %v", gotBody)
	}

	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out["object"] != "list" {
		t.Fatalf("%v", out)
	}
	data, _ := out["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("data %v", out)
	}
	item, _ := data[0].(map[string]any)
	emb, _ := item["embedding"].([]any)
	if len(emb) != 2 || emb[0] != 0.5 {
		t.Fatalf("embedding %v", emb)
	}
	usage, _ := out["usage"].(map[string]any)
	if usage["prompt_tokens"] != float64(3) {
		t.Fatalf("usage %v", usage)
	}

	ev := col.one(t)
	if ev.Modality != ModalityEmbedding || ev.Provider != "google" || ev.TokensIn != 3 {
		t.Fatalf("%+v", ev)
	}
}

func TestEmbeddingsOpenAIToGoogleBatch(t *testing.T) {
	var gotPath string
	var nReqs int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		reqs, _ := body["requests"].([]any)
		nReqs = len(reqs)
		fmt.Fprint(w, `{"embeddings":[{"values":[1,0]},{"values":[0,1]}],"usageMetadata":{"promptTokenCount":5}}`)
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  openai_dialect: google
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)

	resp, err := http.Post(gw.URL+"/v1/embeddings", "application/json",
		strings.NewReader(`{"model":"google/gemini-embedding-001","input":["one","two"]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if gotPath != "/models/gemini-embedding-001:batchEmbedContents" {
		t.Fatalf("path %s", gotPath)
	}
	if nReqs != 2 {
		t.Fatalf("requests %d", nReqs)
	}
	var out map[string]any
	json.Unmarshal(body, &out)
	data, _ := out["data"].([]any)
	if len(data) != 2 {
		t.Fatalf("%v", out)
	}
	ev := col.one(t)
	if ev.TokensIn != 5 || ev.UpstreamModel != "gemini-embedding-001" {
		t.Fatalf("%+v", ev)
	}
}

func TestGoogleNativeEmbedContent(t *testing.T) {
	var gotPath, gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Header.Get("x-goog-api-key") != "gk" {
			t.Errorf("auth %q", r.Header.Get("x-goog-api-key"))
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		gotModel, _ = body["model"].(string)
		fmt.Fprint(w, `{"embedding":{"values":[0.9]},"usageMetadata":{"promptTokenCount":2}}`)
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

	req, _ := http.NewRequest(http.MethodPost,
		gw.URL+"/v1beta/models/gemini-embedding-001:embedContent",
		strings.NewReader(`{"model":"models/gemini-embedding-001","content":{"parts":[{"text":"hi"}]}}`))
	req.Header.Set("x-goog-api-key", "gk")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if gotPath != "/models/gemini-embedding-001:embedContent" {
		t.Fatalf("path %s", gotPath)
	}
	if gotModel != "models/gemini-embedding-001" {
		t.Fatalf("model rewrite %q", gotModel)
	}
	ev := col.one(t)
	if ev.DialectIn != DialectGoogle || ev.Modality != ModalityEmbedding {
		t.Fatalf("%+v", ev)
	}
	if ev.TokensIn != 2 || ev.Status != hooks.StatusOK {
		t.Fatalf("%+v", ev)
	}
}

func TestGoogleNativeBatchEmbedContents(t *testing.T) {
	var gotPath string
	var nestedModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		reqs, _ := body["requests"].([]any)
		if len(reqs) > 0 {
			m, _ := reqs[0].(map[string]any)
			nestedModel, _ = m["model"].(string)
		}
		fmt.Fprint(w, `{"embeddings":[{"values":[1]},{"values":[2]}],"usageMetadata":{"promptTokenCount":7}}`)
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

	payload := `{"requests":[
		{"model":"models/wrong","content":{"parts":[{"text":"a"}]}},
		{"model":"models/wrong","content":{"parts":[{"text":"b"}]}}
	]}`
	req, _ := http.NewRequest(http.MethodPost,
		gw.URL+"/v1beta/models/gemini-embedding-001:batchEmbedContents",
		strings.NewReader(payload))
	req.Header.Set("x-goog-api-key", "gk")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if gotPath != "/models/gemini-embedding-001:batchEmbedContents" {
		t.Fatalf("path %s", gotPath)
	}
	if nestedModel != "models/gemini-embedding-001" {
		t.Fatalf("nested model %q", nestedModel)
	}
	ev := col.one(t)
	if ev.Modality != ModalityEmbedding || ev.TokensIn != 7 {
		t.Fatalf("%+v", ev)
	}
}

func TestParseGoogleActionEmbed(t *testing.T) {
	m, method, ok := parseGoogleAction("gemini-embedding-001:embedContent")
	if !ok || m != "gemini-embedding-001" || method != "embedContent" {
		t.Fatalf("%q %q %v", m, method, ok)
	}
	m, method, ok = parseGoogleAction("gemini-embedding-001:batchEmbedContents")
	if !ok || m != "gemini-embedding-001" || method != "batchEmbedContents" {
		t.Fatalf("%q %q %v", m, method, ok)
	}
}

func TestParseOpenAIEmbeddingInput(t *testing.T) {
	got, err := parseOpenAIEmbeddingInput(json.RawMessage(`"hi"`))
	if err != nil || len(got) != 1 || got[0] != "hi" {
		t.Fatalf("%v %v", got, err)
	}
	got, err = parseOpenAIEmbeddingInput(json.RawMessage(`["a","b"]`))
	if err != nil || len(got) != 2 {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := parseOpenAIEmbeddingInput(json.RawMessage(`[1,2]`)); err == nil {
		t.Fatal("expected error for token ids")
	}
	if _, err := parseOpenAIEmbeddingInput(json.RawMessage(`null`)); err == nil {
		t.Fatal("expected error for null")
	}
}
