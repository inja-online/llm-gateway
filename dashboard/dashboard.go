package dashboard

import (
	"context"
	"net/http"
	"sync"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
	"github.com/inja-online/llm-gateway/hooks/ring"
	"github.com/inja-online/llm-gateway/hooks/sqlite"
)

// sqliteSink is satisfied by hooks/sqlite.Sink (real or nodb stub).
type sqliteSink interface {
	Query(ctx context.Context, q sqlite.Query) ([]hooks.UsageEvent, error)
	Path() string
	SetPath(string) error
}

type Options struct {
	Version   string
	AuthPath  string
	Ring      *ring.Sink
	SQLite    sqliteSink
	JSONLPath string
	Dashboard config.Dashboard
}

type Server struct {
	opts     Options
	ringSize int
	oauthMu  sync.Mutex
	oauth    map[string]*oauthSession
}

func New(opts Options) *Server {
	return &Server{opts: opts, ringSize: opts.Dashboard.Ring()}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/dashboard/meta", s.handleMeta)
	mux.HandleFunc("GET /v1/dashboard/profiles", s.handleProfiles)
	mux.HandleFunc("POST /v1/dashboard/profiles/{provider}/logout", s.handleLogout)
	mux.HandleFunc("POST /v1/dashboard/profiles/{provider}/disable", s.handleDisable)
	mux.HandleFunc("GET /v1/dashboard/usage", s.handleUsage)
	mux.HandleFunc("GET /v1/dashboard/logs", s.handleLogs)
	mux.HandleFunc("GET /v1/dashboard/logs/stream", s.handleLogStream)
	mux.HandleFunc("POST /v1/dashboard/oauth/start", s.handleOAuthStart)
	mux.HandleFunc("POST /v1/dashboard/oauth/complete", s.handleOAuthComplete)
	mux.HandleFunc("POST /v1/dashboard/oauth/import", s.handleOAuthImport)
	mux.HandleFunc("GET /v1/dashboard/oauth/status", s.handleOAuthStatus)
	mux.HandleFunc("GET /v1/dashboard/settings", s.handleGetSettings)
	mux.HandleFunc("PUT /v1/dashboard/settings", s.handlePutSettings)
	return mux
}

func (s *Server) handleMeta(w http.ResponseWriter, _ *http.Request) {
	avail := sqlite.Available()
	writeJSON(w, http.StatusOK, map[string]any{
		"version":         s.opts.Version,
		"dashboard":       true,
		"sqlite":          avail,
		"noweb":           false,
		"nodb":            !avail,
		"ring_size":       s.opts.Dashboard.Ring(),
		"sqlite_path_set": s.opts.SQLite != nil && s.opts.SQLite.Path() != "",
	})
}
