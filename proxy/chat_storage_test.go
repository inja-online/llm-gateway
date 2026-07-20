package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
)

func TestChatCompletionsStorageProxy(t *testing.T) {
	var methods []string
	var paths []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion"}`))
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

	// List
	resp, _ := http.Get(gw.URL + "/v1/chat/completions")
	io.ReadAll(resp.Body)
	resp.Body.Close()
	// Get
	resp2, _ := http.Get(gw.URL + "/v1/chat/completions/chatcmpl-1")
	io.ReadAll(resp2.Body)
	resp2.Body.Close()
	// Update
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/chat/completions/chatcmpl-1",
		strings.NewReader(`{"metadata":{"k":"v"}}`))
	req.Header.Set("Content-Type", "application/json")
	resp3, _ := http.DefaultClient.Do(req)
	io.ReadAll(resp3.Body)
	resp3.Body.Close()
	// Delete
	req4, _ := http.NewRequest(http.MethodDelete, gw.URL+"/v1/chat/completions/chatcmpl-1", nil)
	resp4, _ := http.DefaultClient.Do(req4)
	io.ReadAll(resp4.Body)
	resp4.Body.Close()

	wantM := []string{"GET", "GET", "POST", "DELETE"}
	wantP := []string{"/chat/completions", "/chat/completions/chatcmpl-1", "/chat/completions/chatcmpl-1", "/chat/completions/chatcmpl-1"}
	if len(methods) != 4 {
		t.Fatalf("methods=%v paths=%v", methods, paths)
	}
	for i := range wantM {
		if methods[i] != wantM[i] || paths[i] != wantP[i] {
			t.Fatalf("i=%d method=%s path=%s", i, methods[i], paths[i])
		}
	}
}
