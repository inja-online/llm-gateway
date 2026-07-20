package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
)

func TestAssistantsThreadsRunsProxy(t *testing.T) {
	var got []string
	var gotBeta string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path)
		if b := r.Header.Get("OpenAI-Beta"); b != "" {
			gotBeta = b
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"obj","object":"assistant"}`))
	}))
	t.Cleanup(up.Close)
	cfg, _ := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "` + up.URL + `" }
defaults:
  openai_dialect: openai
`))
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/assistants", strings.NewReader(`{"model":"gpt-4o"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OpenAI-Beta", "assistants=v2")
	http.DefaultClient.Do(req)
	http.Get(gw.URL + "/v1/assistants/asst_1")

	reqT, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/threads", strings.NewReader(`{}`))
	reqT.Header.Set("Content-Type", "application/json")
	reqT.Header.Set("OpenAI-Beta", "assistants=v2")
	http.DefaultClient.Do(reqT)

	reqR, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/threads/thr_1/runs", strings.NewReader(`{"assistant_id":"asst_1"}`))
	reqR.Header.Set("Content-Type", "application/json")
	reqR.Header.Set("OpenAI-Beta", "assistants=v2")
	http.DefaultClient.Do(reqR)

	reqM, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/threads/thr_1/messages", strings.NewReader(`{"role":"user","content":"hi"}`))
	reqM.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(reqM)

	j := strings.Join(got, "\n")
	for _, w := range []string{
		"POST /assistants", "GET /assistants/asst_1",
		"POST /threads", "POST /threads/thr_1/runs", "POST /threads/thr_1/messages",
	} {
		if !strings.Contains(j, w) {
			t.Fatalf("missing %s in %s", w, j)
		}
	}
	if gotBeta != "assistants=v2" {
		t.Fatalf("OpenAI-Beta not forwarded: %q", gotBeta)
	}
}
