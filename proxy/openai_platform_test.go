package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
)

func TestVectorStoresUploadsContainersProxy(t *testing.T) {
	var got []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"obj-1","object":"ok"}`))
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

	// Vector stores
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/vector_stores", strings.NewReader(`{"name":"kb"}`))
	req.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req)
	http.Get(gw.URL + "/v1/vector_stores")
	http.Get(gw.URL + "/v1/vector_stores/vs_1")
	reqF, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/vector_stores/vs_1/files", strings.NewReader(`{"file_id":"file-1"}`))
	reqF.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(reqF)

	// Uploads
	reqU, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/uploads", strings.NewReader(`{"filename":"a.bin"}`))
	reqU.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(reqU)
	reqP, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/uploads/up_1/parts", strings.NewReader(`{}`))
	reqP.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(reqP)
	reqC, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/uploads/up_1/complete", strings.NewReader(`{}`))
	reqC.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(reqC)

	// Containers
	reqN, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/containers", strings.NewReader(`{"name":"c"}`))
	reqN.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(reqN)
	http.Get(gw.URL + "/v1/containers/ctr_1")

	// Drain any body readers
	for _, s := range got {
		_ = s
	}
	// Spot-check key paths
	joined := strings.Join(got, "\n")
	for _, want := range []string{
		"POST /vector_stores",
		"GET /vector_stores",
		"GET /vector_stores/vs_1",
		"POST /vector_stores/vs_1/files",
		"POST /uploads",
		"POST /uploads/up_1/parts",
		"POST /uploads/up_1/complete",
		"POST /containers",
		"GET /containers/ctr_1",
	} {
		if !strings.Contains(joined, want) {
			// read remaining responses if any by listing
			t.Fatalf("missing %q in\n%s", want, joined)
		}
	}
	_ = io.Discard
}
