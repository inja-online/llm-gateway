package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
)

func TestPlatformWaveGoogleProxies(t *testing.T) {
	var got []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(up.Close)
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  google: { kind: google, base_url: %q }
defaults:
  google_dialect: google
`, up.URL)))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, &collector{}).Handler())
	t.Cleanup(gw.Close)

	postJSON(t, gw.URL+"/v1beta/files", `{}`)
	http.Get(gw.URL + "/v1beta/files")
	http.Get(gw.URL + "/v1beta/files/f1")
	postJSON(t, gw.URL+"/v1beta/interactions", `{}`)
	http.Get(gw.URL + "/v1beta/interactions/i1")
	postJSON(t, gw.URL+"/v1beta/batches", `{}`)
	http.Get(gw.URL + "/v1beta/batches/b1")
	postJSON(t, gw.URL+"/v1beta/models/gemini-2.0-flash:batchGenerateContent", `{"requests":[]}`)
	postJSON(t, gw.URL+"/v1beta/models/gemini-2.0-flash:asyncBatchEmbedContent", `{}`)

	j := strings.Join(got, "\n")
	for _, w := range []string{
		"POST /files", "GET /files", "GET /files/f1",
		"POST /interactions", "GET /interactions/i1",
		"POST /batches", "GET /batches/b1",
		"POST /models/gemini-2.0-flash:batchGenerateContent",
		"POST /models/gemini-2.0-flash:asyncBatchEmbedContent",
	} {
		if !strings.Contains(j, w) {
			t.Fatalf("missing %s in\n%s", w, j)
		}
	}
}

func TestPlatformWaveOpenAIAndAnthropic(t *testing.T) {
	var got []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(up.Close)
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
  anthropic: { kind: anthropic, base_url: %q }
defaults:
  openai_dialect: openai
  anthropic_dialect: anthropic
`, up.URL, up.URL)))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, &collector{}).Handler())
	t.Cleanup(gw.Close)

	postJSON(t, gw.URL+"/v1/realtime/client_secrets", `{}`)
	postJSON(t, gw.URL+"/v1/realtime/calls", `{}`)
	postJSON(t, gw.URL+"/v1/evals", `{}`)
	http.Get(gw.URL + "/v1/evals/e1/runs")
	http.Get(gw.URL + "/v1/organization/users")
	postJSON(t, gw.URL+"/v1/responses/compact", `{}`)
	http.Get(gw.URL + "/v1/responses/r1/input_items")
	reqDel, _ := http.NewRequest(http.MethodDelete, gw.URL+"/v1/models/ft:gpt-4o:org:custom:abc", nil)
	http.DefaultClient.Do(reqDel)
	http.Get(gw.URL + "/v1/videos")
	reqVD, _ := http.NewRequest(http.MethodDelete, gw.URL+"/v1/videos/vid_1", nil)
	http.DefaultClient.Do(reqVD)
	postJSON(t, gw.URL+"/v1/videos/vid_1/remix", `{}`)
	http.Get(gw.URL + "/v1/chat/deferred-completion/dc_1")
	postJSON(t, gw.URL+"/v1/rerank", `{}`)
	postJSON(t, gw.URL+"/v1/ocr", `{}`)

	postJSON(t, gw.URL+"/v1/agents", `{}`)
	http.Get(gw.URL + "/v1/agents/a1")
	postJSON(t, gw.URL+"/v1/sessions", `{}`)
	http.Get(gw.URL + "/v1/environments/e1")

	j := strings.Join(got, "\n")
	for _, w := range []string{
		"POST /realtime/client_secrets",
		"POST /realtime/calls",
		"POST /evals",
		"GET /evals/e1/runs",
		"GET /organization/users",
		"POST /responses/compact",
		"GET /responses/r1/input_items",
		"DELETE /models/ft:gpt-4o:org:custom:abc",
		"GET /videos",
		"DELETE /videos/vid_1",
		"POST /videos/vid_1/remix",
		"GET /chat/deferred-completion/dc_1",
		"POST /rerank",
		"POST /ocr",
		"POST /agents",
		"GET /agents/a1",
		"POST /sessions",
		"GET /environments/e1",
	} {
		if !strings.Contains(j, w) {
			t.Fatalf("missing %s in\n%s", w, j)
		}
	}
}

func TestPlatformWaveCoverageExtras(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(up.Close)
	cfg, err := config.Parse([]byte(fmt.Sprintf(`
providers:
  openai: { kind: openai, base_url: %q }
  anthropic: { kind: anthropic, base_url: %q }
  google: { kind: google, base_url: %q }
defaults:
  openai_dialect: openai
  anthropic_dialect: anthropic
  google_dialect: google
`, up.URL, up.URL, up.URL)))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, &collector{}).Handler())
	t.Cleanup(gw.Close)

	// Remaining handlers + nested rest paths.
	postJSON(t, gw.URL+"/v1/realtime/translations", `{}`)
	http.Get(gw.URL + "/v1/realtime/translations")
	http.Get(gw.URL + "/v1/organizations/org_1/users")
	http.Get(gw.URL + "/v1/sessions/s1/events")
	postJSON(t, gw.URL+"/v1/environments", `{}`)
	postJSON(t, gw.URL+"/v1beta/files/f1:download", `{}`)
	postJSON(t, gw.URL+"/v1/agents/a1/runs", `{}`)
	postJSON(t, gw.URL+"/v1/responses/r1/input_items", `{}`)
	// empty id error branches (direct handler calls)
	srv := NewServer(cfg, &collector{})
	for _, fn := range []func(http.ResponseWriter, *http.Request){
		srv.handleVideosDelete, srv.handleVideosRemix, srv.handleDeferredCompletion,
		srv.handleModelsDelete, srv.handleGoogleFilesID, srv.handleGoogleInteractionsID,
		srv.handleGoogleBatchesID, srv.handleAnthropicAgentsID, srv.handleAnthropicSessionsID,
		srv.handleAnthropicEnvironmentsID, srv.handleEvalsID, srv.handleResponsesInputItems,
	} {
		rec := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		fn(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("want 400 for empty id, got %d", rec.Code)
		}
	}
}

func postJSON(t *testing.T, url, body string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}
