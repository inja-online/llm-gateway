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

func TestRewriteBatchCreateModels(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  anthropic: { kind: anthropic, base_url: "https://api.anthropic.com/v1" }
  openai: { kind: openai, base_url: "https://api.openai.com/v1" }
defaults:
  anthropic_dialect: anthropic
aliases:
  fast: anthropic/claude-haiku-4
  other: openai/gpt-4o
`))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("provider_prefix_and_alias", func(t *testing.T) {
		in := `{
  "requests": [
    {
      "custom_id": "a",
      "params": {
        "model": "anthropic/claude-sonnet-5",
        "max_tokens": 10,
        "messages": [{"role":"user","content":"hi"}]
      }
    },
    {
      "custom_id": "b",
      "params": {
        "model": "fast",
        "max_tokens": 8,
        "messages": [{"role":"user","content":"yo"}]
      }
    },
    {
      "custom_id": "c",
      "params": {
        "model": "claude-opus-4",
        "max_tokens": 5,
        "messages": [{"role":"user","content":"bare"}]
      }
    }
  ]
}`
		out, first, up, n, err := rewriteBatchCreateModels(cfg, []byte(in), "anthropic")
		if err != nil {
			t.Fatal(err)
		}
		if n != 3 || first != "anthropic/claude-sonnet-5" || up != "claude-sonnet-5" {
			t.Fatalf("meta n=%d first=%q up=%q", n, first, up)
		}
		var root map[string]any
		if json.Unmarshal(out, &root) != nil {
			t.Fatal("bad json")
		}
		reqs := root["requests"].([]any)
		models := []string{}
		for _, r := range reqs {
			p := r.(map[string]any)["params"].(map[string]any)
			models = append(models, p["model"].(string))
		}
		want := []string{"claude-sonnet-5", "claude-haiku-4", "claude-opus-4"}
		for i := range want {
			if models[i] != want[i] {
				t.Errorf("model[%d]=%q want %q", i, models[i], want[i])
			}
		}
		// Preserve non-model fields.
		p0 := reqs[0].(map[string]any)["params"].(map[string]any)
		if p0["max_tokens"].(float64) != 10 {
			t.Errorf("max_tokens rewritten unexpectedly: %v", p0["max_tokens"])
		}
	})

	t.Run("wrong_provider_prefix", func(t *testing.T) {
		in := `{"requests":[{"custom_id":"x","params":{"model":"openai/gpt-4o","max_tokens":1,"messages":[]}}]}`
		_, _, _, _, err := rewriteBatchCreateModels(cfg, []byte(in), "anthropic")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "openai") && !strings.Contains(err.Error(), "anthropic") {
			t.Fatalf("err: %v", err)
		}
	})

	t.Run("alias_to_non_anthropic", func(t *testing.T) {
		in := `{"requests":[{"custom_id":"x","params":{"model":"other","max_tokens":1,"messages":[]}}]}`
		_, _, _, _, err := rewriteBatchCreateModels(cfg, []byte(in), "anthropic")
		if err == nil {
			t.Fatal("expected error for openai alias")
		}
	})

	t.Run("unknown_provider_in_model", func(t *testing.T) {
		in := `{"requests":[{"custom_id":"x","params":{"model":"nope/m","max_tokens":1,"messages":[]}}]}`
		_, _, _, _, err := rewriteBatchCreateModels(cfg, []byte(in), "anthropic")
		if err == nil || !strings.Contains(err.Error(), "unknown provider") {
			t.Fatalf("err: %v", err)
		}
	})

	t.Run("provider_mismatch_two_anthropic", func(t *testing.T) {
		cfg2, err := config.Parse([]byte(`
providers:
  a1: { kind: anthropic, base_url: "https://a1" }
  a2: { kind: anthropic, base_url: "https://a2" }
defaults:
  anthropic_dialect: a1
`))
		if err != nil {
			t.Fatal(err)
		}
		in := `{"requests":[{"custom_id":"x","params":{"model":"a2/claude","max_tokens":1,"messages":[]}}]}`
		_, _, _, _, err = rewriteBatchCreateModels(cfg2, []byte(in), "a1")
		if err == nil || !strings.Contains(err.Error(), "a2") {
			t.Fatalf("err: %v", err)
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		_, _, _, _, err := rewriteBatchCreateModels(cfg, []byte(`not-json`), "anthropic")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("no_requests_passthrough", func(t *testing.T) {
		in := []byte(`{"other":true}`)
		out, first, up, n, err := rewriteBatchCreateModels(cfg, in, "anthropic")
		if err != nil || first != "" || up != "" || n != 0 {
			t.Fatalf("out meta err=%v first=%q n=%d", err, first, n)
		}
		if string(out) != string(in) {
			t.Fatalf("body changed: %s", out)
		}
	})

	t.Run("missing_model_skipped", func(t *testing.T) {
		in := `{"requests":[{"custom_id":"x","params":{"max_tokens":1}}]}`
		out, _, _, n, err := rewriteBatchCreateModels(cfg, []byte(in), "anthropic")
		if err != nil || n != 1 {
			t.Fatalf("err=%v n=%d", err, n)
		}
		if !strings.Contains(string(out), `"custom_id":"x"`) {
			t.Fatalf("body: %s", out)
		}
	})
}

func TestBatchesCreateListGetCancelResults(t *testing.T) {
	var (
		gotCreatePath, gotModel, gotKey, gotVersion, gotBeta string
		createBody                                           []byte
		posts, lists, gets, cancels, results                 int
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/messages/batches":
			posts++
			gotCreatePath = r.URL.Path
			gotKey = r.Header.Get("x-api-key")
			gotVersion = r.Header.Get("anthropic-version")
			gotBeta = r.Header.Get("anthropic-beta")
			createBody, _ = io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(createBody, &body)
			reqs, _ := body["requests"].([]any)
			if len(reqs) > 0 {
				p := reqs[0].(map[string]any)["params"].(map[string]any)
				gotModel, _ = p["model"].(string)
			}
			fmt.Fprint(w, `{"id":"msgbatch_1","type":"message_batch","processing_status":"in_progress"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/messages/batches":
			lists++
			if r.URL.Query().Get("limit") != "2" {
				t.Errorf("limit %q", r.URL.Query().Get("limit"))
			}
			// provider query must be stripped
			if r.URL.Query().Get("provider") != "" {
				t.Errorf("provider leaked: %q", r.URL.Query().Get("provider"))
			}
			fmt.Fprint(w, `{"data":[{"id":"msgbatch_1"}],"has_more":false}`)
		case r.Method == http.MethodGet && r.URL.Path == "/messages/batches/msgbatch_1":
			gets++
			fmt.Fprint(w, `{"id":"msgbatch_1","processing_status":"ended"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/messages/batches/msgbatch_1/cancel":
			cancels++
			fmt.Fprint(w, `{"id":"msgbatch_1","processing_status":"canceling"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/messages/batches/msgbatch_1/results":
			results++
			w.Header().Set("Content-Type", "application/x-jsonl")
			fmt.Fprint(w, `{"custom_id":"a","result":{"type":"succeeded"}}`+"\n")
		default:
			t.Errorf("%s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(upstream.Close)

	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  anthropic: { kind: anthropic, base_url: %q }
defaults:
  anthropic_dialect: anthropic
aliases:
  fast: anthropic/claude-haiku-4
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}

	// create
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	createPayload := `{
  "requests": [
    {
      "custom_id": "a",
      "params": {
        "model": "anthropic/claude-sonnet-5",
        "max_tokens": 16,
        "messages": [{"role":"user","content":"hi"}]
      }
    },
    {
      "custom_id": "b",
      "params": {
        "model": "fast",
        "max_tokens": 8,
        "messages": [{"role":"user","content":"yo"}]
      }
    }
  ]
}`
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/messages/batches", strings.NewReader(createPayload))
	req.Header.Set("x-api-key", "sk-ant-batch")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "message-batches-2024-09-24")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(b), "msgbatch_1") {
		t.Fatalf("create %d %s", resp.StatusCode, b)
	}
	if gotCreatePath != "/messages/batches" || gotModel != "claude-sonnet-5" {
		t.Fatalf("upstream create path=%q model=%q body=%s", gotCreatePath, gotModel, createBody)
	}
	if gotKey != "sk-ant-batch" || gotVersion != "2023-06-01" || gotBeta != "message-batches-2024-09-24" {
		t.Fatalf("headers key=%q ver=%q beta=%q", gotKey, gotVersion, gotBeta)
	}
	// second model rewritten too
	if !strings.Contains(string(createBody), `"claude-haiku-4"`) {
		t.Fatalf("alias not rewritten: %s", createBody)
	}
	if strings.Contains(string(createBody), `"anthropic/claude-sonnet-5"`) {
		t.Fatalf("prefix not stripped: %s", createBody)
	}
	ev := col.one(t)
	if ev.Status != hooks.StatusOK || !ev.Estimated || ev.Provider != "anthropic" {
		t.Fatalf("create event: %+v", ev)
	}
	if ev.Model != "anthropic/claude-sonnet-5" || ev.UpstreamModel != "claude-sonnet-5" {
		t.Fatalf("create models: %+v", ev)
	}
	if ev.TokensIn < 1 {
		t.Fatalf("expected estimated tokens_in: %+v", ev)
	}

	// list
	col2 := &collector{}
	gw2 := httptest.NewServer(NewServer(cfg, col2).Handler())
	t.Cleanup(gw2.Close)
	resp2, _ := http.Get(gw2.URL + "/v1/messages/batches?limit=2&provider=anthropic")
	b2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 200 || !strings.Contains(string(b2), "msgbatch_1") {
		t.Fatalf("list %d %s", resp2.StatusCode, b2)
	}
	if !col2.one(t).Estimated {
		t.Fatal("list should be light/estimated")
	}

	// get
	col3 := &collector{}
	gw3 := httptest.NewServer(NewServer(cfg, col3).Handler())
	t.Cleanup(gw3.Close)
	resp3, _ := http.Get(gw3.URL + "/v1/messages/batches/msgbatch_1")
	b3, _ := io.ReadAll(resp3.Body)
	resp3.Body.Close()
	if resp3.StatusCode != 200 || !strings.Contains(string(b3), "ended") {
		t.Fatalf("get %d %s", resp3.StatusCode, b3)
	}
	col3.one(t)

	// cancel
	col4 := &collector{}
	gw4 := httptest.NewServer(NewServer(cfg, col4).Handler())
	t.Cleanup(gw4.Close)
	req4, _ := http.NewRequest(http.MethodPost, gw4.URL+"/v1/messages/batches/msgbatch_1/cancel", nil)
	req4.Header.Set("x-api-key", "sk-ant-batch")
	resp4, _ := http.DefaultClient.Do(req4)
	b4, _ := io.ReadAll(resp4.Body)
	resp4.Body.Close()
	if resp4.StatusCode != 200 || !strings.Contains(string(b4), "canceling") {
		t.Fatalf("cancel %d %s", resp4.StatusCode, b4)
	}
	col4.one(t)

	// results (JSONL stream)
	col5 := &collector{}
	gw5 := httptest.NewServer(NewServer(cfg, col5).Handler())
	t.Cleanup(gw5.Close)
	resp5, _ := http.Get(gw5.URL + "/v1/messages/batches/msgbatch_1/results")
	b5, _ := io.ReadAll(resp5.Body)
	resp5.Body.Close()
	if resp5.StatusCode != 200 || !strings.Contains(string(b5), "custom_id") {
		t.Fatalf("results %d %s", resp5.StatusCode, b5)
	}
	if ct := resp5.Header.Get("Content-Type"); !strings.Contains(ct, "jsonl") && !strings.Contains(ct, "json") {
		// allowlisted Content-Type from upstream
		t.Logf("results content-type: %s", ct)
	}
	col5.one(t)

	if posts != 1 || lists != 1 || gets != 1 || cancels != 1 || results != 1 {
		t.Fatalf("counts post=%d list=%d get=%d cancel=%d results=%d", posts, lists, gets, cancels, results)
	}
}

func TestBatchesRejectsNonAnthropic(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "https://api.openai.com/v1" }
defaults:
  anthropic_dialect: openai
`))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	resp, err := http.Get(gw.URL + "/v1/messages/batches")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
}

func TestBatchesMissingProvider(t *testing.T) {
	cfg, _ := config.Parse([]byte(`providers: { anthropic: { kind: anthropic, base_url: "http://x" } }`))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	resp, err := http.Post(gw.URL+"/v1/messages/batches", "application/json",
		strings.NewReader(`{"requests":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
}

func TestBatchesUnknownProvider(t *testing.T) {
	cfg, _ := config.Parse([]byte(`
providers:
  anthropic: { kind: anthropic, base_url: "http://x" }
defaults:
  anthropic_dialect: anthropic
`))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	resp, _ := http.Get(gw.URL + "/v1/messages/batches?provider=missing")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
}

func TestBatchesCreateModelRewriteError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not call upstream")
		w.WriteHeader(500)
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  anthropic: { kind: anthropic, base_url: %q }
  openai: { kind: openai, base_url: "http://o" }
defaults:
  anthropic_dialect: anthropic
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	payload := `{"requests":[{"custom_id":"a","params":{"model":"openai/gpt-4o","max_tokens":1,"messages":[]}}]}`
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/messages/batches", strings.NewReader(payload))
	req.Header.Set("x-api-key", "sk")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d %s", resp.StatusCode, body)
	}
	col.one(t)
}

func TestBatchesCreateUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"bad"}}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  anthropic: { kind: anthropic, base_url: %q }
defaults:
  anthropic_dialect: anthropic
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	payload := `{"requests":[{"custom_id":"a","params":{"model":"claude-x","max_tokens":1,"messages":[{"role":"user","content":"h"}]}}]}`
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/messages/batches", strings.NewReader(payload))
	req.Header.Set("x-api-key", "sk")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 400 || !strings.Contains(string(b), "bad") {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	ev := col.one(t)
	if ev.Status != hooks.StatusUpstreamError {
		t.Fatalf("event: %+v", ev)
	}
}

func TestBatchesXProviderHeader(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"data":[]}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  ant: { kind: anthropic, base_url: %q }
  other: { kind: anthropic, base_url: "http://127.0.0.1:9" }
defaults:
  anthropic_dialect: other
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	req, _ := http.NewRequest(http.MethodGet, gw.URL+"/v1/messages/batches", nil)
	req.Header.Set("X-Provider", "ant")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || gotPath != "/messages/batches" {
		t.Fatalf("status=%d path=%q", resp.StatusCode, gotPath)
	}
	if col.one(t).Provider != "ant" {
		t.Fatal("provider from X-Provider")
	}
}
