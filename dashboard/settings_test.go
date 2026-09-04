package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
	"github.com/inja-online/llm-gateway/hooks/ring"
	"github.com/inja-online/llm-gateway/hooks/sqlite"
)

type settingsSQL struct {
	path string
}

func (f *settingsSQL) Query(context.Context, sqlite.Query) ([]hooks.UsageEvent, error) {
	return nil, nil
}
func (f *settingsSQL) Path() string           { return f.path }
func (f *settingsSQL) SetPath(p string) error { f.path = p; return nil }

type settingsDTO struct {
	RingSize      int    `json:"ring_size"`
	SQLitePath    string `json:"sqlite_path"`
	SQLiteEnabled bool   `json:"sqlite_enabled"`
	JSONLOutput   string `json:"jsonl_output"`
}

func getSettings(t *testing.T, h http.Handler) (int, settingsDTO, []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/settings", nil))
	var got settingsDTO
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil && rec.Code == http.StatusOK {
			t.Fatalf("decode: %v body %s", err, rec.Body.Bytes())
		}
	}
	return rec.Code, got, rec.Body.Bytes()
}

func putSettings(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v1/dashboard/settings", strings.NewReader(body))
	h.ServeHTTP(rec, req)
	return rec
}

func TestGetSettings(t *testing.T) {
	sql := &settingsSQL{path: "/tmp/usage.sqlite"}
	h := New(Options{
		JSONLPath: "/var/log/usage.jsonl",
		SQLite:    sql,
		Dashboard: config.Dashboard{RingSize: 50},
	}).Handler()
	code, got, raw := getSettings(t, h)
	if code != http.StatusOK {
		t.Fatalf("status %d body %s", code, raw)
	}
	if got.RingSize != 50 {
		t.Fatalf("ring_size %d", got.RingSize)
	}
	if got.SQLitePath != "/tmp/usage.sqlite" {
		t.Fatalf("sqlite_path %q", got.SQLitePath)
	}
	wantEnabled := sqlite.Available() && got.SQLitePath != ""
	if got.SQLiteEnabled != wantEnabled {
		t.Fatalf("sqlite_enabled %v want %v", got.SQLiteEnabled, wantEnabled)
	}
	if got.JSONLOutput != "/var/log/usage.jsonl" {
		t.Fatalf("jsonl_output %q", got.JSONLOutput)
	}
}

func TestPutRingSize(t *testing.T) {
	r := ring.New(10)
	h := New(Options{Ring: r, Dashboard: config.Dashboard{RingSize: 10}}).Handler()

	rec := putSettings(t, h, `{"ring_size":40}`)
	if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Fatalf("put status %d body %s", rec.Code, rec.Body.Bytes())
	}
	code, got, raw := getSettings(t, h)
	if code != http.StatusOK {
		t.Fatalf("get status %d body %s", code, raw)
	}
	if got.RingSize != 40 {
		t.Fatalf("after put ring_size %d", got.RingSize)
	}

	rec = putSettings(t, h, `{"ring_size":0}`)
	if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Fatalf("put 0 status %d", rec.Code)
	}
	_, got, _ = getSettings(t, h)
	if got.RingSize != config.DefaultDashboardRingSize {
		t.Fatalf("ring_size<=0 want %d got %d", config.DefaultDashboardRingSize, got.RingSize)
	}

	rec = putSettings(t, h, `{"ring_size":-3}`)
	if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Fatalf("put -3 status %d", rec.Code)
	}
	_, got, _ = getSettings(t, h)
	if got.RingSize != config.DefaultDashboardRingSize {
		t.Fatalf("negative ring_size want %d got %d", config.DefaultDashboardRingSize, got.RingSize)
	}
}

func TestPutRingSizeTooLarge(t *testing.T) {
	h := New(Options{}).Handler()
	rec := putSettings(t, h, `{"ring_size":100001}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
	}
}

func TestPutSQLitePath(t *testing.T) {
	if !sqlite.Available() {
		t.Skip("SetPath under nodb is sqlite_disabled")
	}
	sql := &settingsSQL{path: "/old.db"}
	h := New(Options{SQLite: sql}).Handler()
	rec := putSettings(t, h, `{"sqlite_path":"/new.db"}`)
	if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Fatalf("put status %d body %s", rec.Code, rec.Body.Bytes())
	}
	if sql.path != "/new.db" {
		t.Fatalf("SetPath got %q", sql.path)
	}
	_, got, _ := getSettings(t, h)
	if got.SQLitePath != "/new.db" {
		t.Fatalf("sqlite_path %q", got.SQLitePath)
	}

	rec = putSettings(t, h, `{"sqlite_path":""}`)
	if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Fatalf("disable status %d body %s", rec.Code, rec.Body.Bytes())
	}
	if sql.path != "" {
		t.Fatalf("empty path should disable, got %q", sql.path)
	}
	_, got, _ = getSettings(t, h)
	if got.SQLiteEnabled {
		t.Fatalf("sqlite_enabled after disable")
	}
}

func TestPutSQLiteDisabled(t *testing.T) {
	if sqlite.Available() {
		t.Skip("sqlite_disabled only under -tags nodb")
	}
	h := New(Options{SQLite: &settingsSQL{}}).Handler()
	rec := putSettings(t, h, `{"sqlite_path":"/tmp/x.db"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(`"code":null`)) {
		t.Fatalf("code null: %s", rec.Body.Bytes())
	}
	var wrap struct {
		Error struct {
			Code *string `json:"code"`
			Type string  `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Code == nil || *wrap.Error.Code != "sqlite_disabled" {
		t.Fatalf("code %+v body %s", wrap.Error.Code, rec.Body.Bytes())
	}
}

func TestCORS(t *testing.T) {
	origin := "https://spa.example"
	h := CORS(origin, New(Options{}).Handler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/settings", nil))
	if rec.Header().Get("Access-Control-Allow-Origin") != origin {
		t.Fatalf("ACAO %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
	if rec.Header().Get("Access-Control-Allow-Origin") == "*" {
		t.Fatal("CORS *")
	}
	if rec.Header().Get("Access-Control-Allow-Headers") != "Authorization, Content-Type, x-api-key" {
		t.Fatalf("headers %q", rec.Header().Get("Access-Control-Allow-Headers"))
	}
	if rec.Header().Get("Access-Control-Allow-Methods") != "GET, POST, PUT, OPTIONS" {
		t.Fatalf("methods %q", rec.Header().Get("Access-Control-Allow-Methods"))
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, "/v1/dashboard/settings", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS status %d", rec.Code)
	}

	plain := New(Options{}).Handler()
	rec = httptest.NewRecorder()
	plain.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard/settings", nil))
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("empty CORSOrigin leaked ACAO %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}
