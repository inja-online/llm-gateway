package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
)

func TestFineTuningJobsProxy(t *testing.T) {
	var got []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ftjob-1","object":"fine_tuning.job"}`))
	}))
	t.Cleanup(up.Close)

	cfg, err := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "` + up.URL + `" }
defaults:
  openai_dialect: openai
`))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/fine_tuning/jobs",
		strings.NewReader(`{"model":"gpt-4o-mini","training_file":"file-1"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	io.ReadAll(resp.Body)
	resp.Body.Close()

	http.Get(gw.URL + "/v1/fine_tuning/jobs")
	http.Get(gw.URL + "/v1/fine_tuning/jobs/ftjob-1")
	reqC, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/fine_tuning/jobs/ftjob-1/cancel", nil)
	http.DefaultClient.Do(reqC)
	http.Get(gw.URL + "/v1/fine_tuning/jobs/ftjob-1/events")
	http.Get(gw.URL + "/v1/fine_tuning/jobs/ftjob-1/checkpoints")

	want := []string{
		"POST /fine_tuning/jobs",
		"GET /fine_tuning/jobs",
		"GET /fine_tuning/jobs/ftjob-1",
		"POST /fine_tuning/jobs/ftjob-1/cancel",
		"GET /fine_tuning/jobs/ftjob-1/events",
		"GET /fine_tuning/jobs/ftjob-1/checkpoints",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("i=%d got %q want %q", i, got[i], want[i])
		}
	}
}
