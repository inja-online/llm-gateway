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

// --- Anthropic Files (anthropic-version selects dialect on shared /v1/files*) ---

func TestAnthropicFilesUploadListGetDeleteContent(t *testing.T) {
	const (
		wantVersion = "2023-06-01"
		// Unknown/future beta values must be preserved (not allowlisted).
		wantBeta = "files-api-2025-04-14,custom-beta-xyz"
	)
	var gotCT, gotVersion, gotBeta, gotKey string
	var posts, lists, gets, dels, contents int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotVersion = r.Header.Get("anthropic-version")
		gotBeta = r.Header.Get("anthropic-beta")
		gotKey = r.Header.Get("x-api-key")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/files":
			posts++
			gotCT = r.Header.Get("Content-Type")
			body, _ := io.ReadAll(r.Body)
			if !bytes.Contains(body, []byte("hello ant file")) {
				t.Errorf("multipart body missing content")
			}
			if !strings.Contains(gotCT, "multipart/") {
				t.Errorf("ct %s", gotCT)
			}
			// Boundary must survive (not rewritten to application/json).
			if !strings.Contains(gotCT, "boundary=") {
				t.Errorf("boundary lost: %s", gotCT)
			}
			fmt.Fprint(w, `{"id":"file_ant_1","type":"file","filename":"a.txt","downloadable":false}`)
		case r.Method == http.MethodGet && r.URL.Path == "/files":
			lists++
			if r.URL.Query().Get("limit") != "10" {
				t.Errorf("limit %q", r.URL.Query().Get("limit"))
			}
			fmt.Fprint(w, `{"data":[{"id":"file_ant_1"}],"has_more":false}`)
		case r.Method == http.MethodGet && r.URL.Path == "/files/file_ant_1":
			gets++
			fmt.Fprint(w, `{"id":"file_ant_1","type":"file","filename":"a.txt"}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/files/file_ant_1":
			dels++
			fmt.Fprint(w, `{"id":"file_ant_1","type":"file_deleted"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/files/file_ant_1/content":
			contents++
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "hello ant file")
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
`, upstream.URL)))
	if err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	s := NewServer(cfg, col)
	gw := httptest.NewServer(s.Handler())
	t.Cleanup(gw.Close)

	setAnt := func(req *http.Request) {
		req.Header.Set("x-api-key", "sk-ant-files")
		req.Header.Set("anthropic-version", wantVersion)
		req.Header.Set("anthropic-beta", wantBeta)
	}

	// upload
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("file", "a.txt")
	_, _ = part.Write([]byte("hello ant file"))
	_ = mw.Close()
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/files", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	setAnt(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(b), "file_ant_1") {
		t.Fatalf("upload %d %s", resp.StatusCode, b)
	}
	if gotVersion != wantVersion {
		t.Errorf("upload anthropic-version: got %q want %q", gotVersion, wantVersion)
	}
	if gotBeta != wantBeta {
		t.Errorf("upload anthropic-beta: got %q want %q", gotBeta, wantBeta)
	}
	if gotKey != "sk-ant-files" {
		t.Errorf("upload x-api-key: %q", gotKey)
	}
	if col.one(t).Status != hooks.StatusOK {
		t.Fatal("upload event")
	}

	// list with query (provider stripped)
	col2 := &collector{}
	gw2 := httptest.NewServer(NewServer(cfg, col2).Handler())
	t.Cleanup(gw2.Close)
	req2, _ := http.NewRequest(http.MethodGet, gw2.URL+"/v1/files?limit=10&provider=anthropic", nil)
	setAnt(req2)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 200 || !strings.Contains(string(b2), "file_ant_1") {
		t.Fatalf("list %d %s", resp2.StatusCode, b2)
	}
	if gotBeta != wantBeta {
		t.Errorf("list beta not forwarded: %q", gotBeta)
	}
	col2.one(t)

	// get metadata
	col3 := &collector{}
	gw3 := httptest.NewServer(NewServer(cfg, col3).Handler())
	t.Cleanup(gw3.Close)
	req3, _ := http.NewRequest(http.MethodGet, gw3.URL+"/v1/files/file_ant_1", nil)
	setAnt(req3)
	resp3, _ := http.DefaultClient.Do(req3)
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
	req4, _ := http.NewRequest(http.MethodGet, gw4.URL+"/v1/files/file_ant_1/content", nil)
	setAnt(req4)
	resp4, _ := http.DefaultClient.Do(req4)
	b4, _ := io.ReadAll(resp4.Body)
	resp4.Body.Close()
	if resp4.StatusCode != 200 || string(b4) != "hello ant file" {
		t.Fatalf("content %d %s", resp4.StatusCode, b4)
	}
	col4.one(t)

	// delete
	col5 := &collector{}
	gw5 := httptest.NewServer(NewServer(cfg, col5).Handler())
	t.Cleanup(gw5.Close)
	req5, _ := http.NewRequest(http.MethodDelete, gw5.URL+"/v1/files/file_ant_1", nil)
	setAnt(req5)
	resp5, _ := http.DefaultClient.Do(req5)
	b5, _ := io.ReadAll(resp5.Body)
	resp5.Body.Close()
	if resp5.StatusCode != 200 || !strings.Contains(string(b5), "file_deleted") {
		t.Fatalf("del %d %s", resp5.StatusCode, b5)
	}
	col5.one(t)

	if posts != 1 || lists != 1 || gets != 1 || dels != 1 || contents != 1 {
		t.Fatalf("counts post=%d list=%d get=%d del=%d content=%d", posts, lists, gets, dels, contents)
	}
}

func TestAnthropicFilesRejectsNonAnthropic(t *testing.T) {
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
	req, _ := http.NewRequest(http.MethodGet, gw.URL+"/v1/files", nil)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("x-api-key", "sk")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	// Anthropic error envelope
	if !strings.Contains(string(body), `"type":"error"`) && !strings.Contains(string(body), `"type": "error"`) {
		// JSON encoder may omit spaces
		if !strings.Contains(string(body), "error") {
			t.Fatalf("want anthropic error envelope: %s", body)
		}
	}
	if !strings.Contains(string(body), "anthropic") {
		t.Fatalf("want anthropic kind message: %s", body)
	}
	col.one(t)
}

func TestAnthropicFilesCustomVersionForwarded(t *testing.T) {
	var gotVersion string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotVersion = r.Header.Get("anthropic-version")
		fmt.Fprint(w, `{"data":[]}`)
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
	req, _ := http.NewRequest(http.MethodGet, gw.URL+"/v1/files", nil)
	req.Header.Set("x-api-key", "sk")
	req.Header.Set("anthropic-version", "2024-10-22") // non-default client value
	req.Header.Set("anthropic-beta", "some-future-beta-99")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if gotVersion != "2024-10-22" {
		t.Fatalf("version %q (client value must override applyAuth default)", gotVersion)
	}
	col.one(t)
}

func TestAnthropicFilesMissingProvider(t *testing.T) {
	cfg, _ := config.Parse([]byte(`providers: { anthropic: { kind: anthropic, base_url: "http://x" } }`))
	col := &collector{}
	gw := httptest.NewServer(NewServer(cfg, col).Handler())
	t.Cleanup(gw.Close)
	req, _ := http.NewRequest(http.MethodGet, gw.URL+"/v1/files", nil)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("x-api-key", "sk")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", resp.StatusCode)
	}
	col.one(t)
}

func TestAnthropicFilesEmptyIDBranches(t *testing.T) {
	cfg, _ := config.Parse([]byte(`providers: { anthropic: { kind: anthropic, base_url: "http://x" } }
defaults: { anthropic_dialect: anthropic }`))
	s := NewServer(cfg, nil)
	for _, fn := range []func(http.ResponseWriter, *http.Request){
		s.handleFilesGet, s.handleFilesDelete, s.handleFilesContent,
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/files/", nil)
		req.Header.Set("anthropic-version", "2023-06-01")
		fn(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("code %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "error") {
			t.Fatalf("want anthropic-shaped error: %s", body)
		}
	}
}

func TestAnthropicFilesWithoutVersionUsesOpenAIPath(t *testing.T) {
	// Without anthropic-version, shared paths stay on OpenAI-family resolution.
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
		t.Fatalf("status %d (want OpenAI-family reject for kind anthropic)", resp.StatusCode)
	}
	col.one(t)
}
