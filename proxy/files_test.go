package proxy

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

func TestFilesUploadListGetDeleteContent(t *testing.T) {
	var gotCT string
	var posts, lists, gets, dels, contents int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/files":
			posts++
			gotCT = r.Header.Get("Content-Type")
			body, _ := io.ReadAll(r.Body)
			if !bytes.Contains(body, []byte("hello file")) {
				t.Errorf("multipart body missing content")
			}
			if !strings.Contains(gotCT, "multipart/") {
				t.Errorf("ct %s", gotCT)
			}
			fmt.Fprint(w, `{"id":"file_1","object":"file","filename":"a.txt"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/files":
			lists++
			if r.URL.Query().Get("purpose") != "assistants" {
				t.Errorf("purpose %q", r.URL.Query().Get("purpose"))
			}
			fmt.Fprint(w, `{"data":[{"id":"file_1"}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/files/file_1":
			gets++
			fmt.Fprint(w, `{"id":"file_1","filename":"a.txt"}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/files/file_1":
			dels++
			fmt.Fprint(w, `{"id":"file_1","deleted":true}`)
		case r.Method == http.MethodGet && r.URL.Path == "/files/file_1/content":
			contents++
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "hello file")
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
	s := NewServer(cfg, col)
	gw := httptest.NewServer(s.Handler())
	t.Cleanup(gw.Close)

	// upload
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("purpose", "assistants")
	part, _ := mw.CreateFormFile("file", "a.txt")
	_, _ = part.Write([]byte("hello file"))
	_ = mw.Close()
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/files", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer sk-f")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(b), "file_1") {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if col.one(t).Status != hooks.StatusOK {
		t.Fatal("upload event")
	}

	// list with purpose query
	col2 := &collector{}
	gw2 := httptest.NewServer(NewServer(cfg, col2).Handler())
	t.Cleanup(gw2.Close)
	resp2, _ := http.Get(gw2.URL + "/v1/files?purpose=assistants&provider=openai")
	b2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 200 || !strings.Contains(string(b2), "file_1") {
		t.Fatalf("list %d %s", resp2.StatusCode, b2)
	}
	col2.one(t)

	// get
	col3 := &collector{}
	gw3 := httptest.NewServer(NewServer(cfg, col3).Handler())
	t.Cleanup(gw3.Close)
	resp3, _ := http.Get(gw3.URL + "/v1/files/file_1")
	b3, _ := io.ReadAll(resp3.Body)
	resp3.Body.Close()
	if resp3.StatusCode != 200 || !strings.Contains(string(b3), "a.txt") {
		t.Fatalf("get %d %s", resp3.StatusCode, b3)
	}
	col3.one(t)

	// content
	col4 := &collector{}
	gw4 := httptest.NewServer(NewServer(cfg, col4).Handler())
	t.Cleanup(gw4.Close)
	resp4, _ := http.Get(gw4.URL + "/v1/files/file_1/content")
	b4, _ := io.ReadAll(resp4.Body)
	resp4.Body.Close()
	if resp4.StatusCode != 200 || string(b4) != "hello file" {
		t.Fatalf("content %d %s", resp4.StatusCode, b4)
	}
	col4.one(t)

	// delete
	col5 := &collector{}
	gw5 := httptest.NewServer(NewServer(cfg, col5).Handler())
	t.Cleanup(gw5.Close)
	req5, _ := http.NewRequest(http.MethodDelete, gw5.URL+"/v1/files/file_1", nil)
	resp5, _ := http.DefaultClient.Do(req5)
	b5, _ := io.ReadAll(resp5.Body)
	resp5.Body.Close()
	if resp5.StatusCode != 200 || !strings.Contains(string(b5), "deleted") {
		t.Fatalf("del %d %s", resp5.StatusCode, b5)
	}
	col5.one(t)

	if posts != 1 || lists != 1 || gets != 1 || dels != 1 || contents != 1 {
		t.Fatalf("counts post=%d list=%d get=%d del=%d content=%d", posts, lists, gets, dels, contents)
	}
}

func TestFilesRejectsNonOpenAI(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  anthropic: { kind: anthropic, base_url: "https://api.anthropic.com/v1" }
defaults:
  openai_dialect: anthropic
`))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	resp, err := http.Get(gw.URL + "/v1/files")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
}

func TestFilesMissingID(t *testing.T) {
	cfg, _ := config.Parse([]byte(`providers: { openai: { kind: openai, base_url: "http://x" } }
defaults: { openai_dialect: openai }`))
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)
	// empty id routes won't match path patterns the same way; hit content with blank via handler unit
	// list still works
	// unknown provider
	col := &collector{}
	gw2 := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw2.Close)
	resp, _ := http.Get(gw2.URL + "/v1/files?provider=missing")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
}

func TestFilesUploadMissingProvider(t *testing.T) {
	cfg, _ := config.Parse([]byte(`providers: { openai: { kind: openai, base_url: "http://x" } }`))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	resp, err := http.Post(gw.URL+"/v1/files", "multipart/form-data; boundary=x", strings.NewReader("--x--"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
}

func TestFilesContentUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		fmt.Fprint(w, `{"error":{"message":"missing"}}`)
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
	resp, err := http.Get(gw.URL + "/v1/files/file_x/content")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
}

func TestFilesEmptyIDBranches(t *testing.T) {
	cfg, _ := config.Parse([]byte(`providers: { openai: { kind: openai, base_url: "http://x" } }
defaults: { openai_dialect: openai }`))
	s := NewServer(cfg, nil)
	for _, fn := range []func(http.ResponseWriter, *http.Request){
		s.handleFilesGet, s.handleFilesDelete, s.handleFilesContent,
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/files/", nil)
		// PathValue("id") empty without ServeMux pattern
		fn(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("code %d", rec.Code)
		}
	}
}
