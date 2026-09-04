package main

import (
	"io/fs"
	"net/http"
	"path"

	gateway "github.com/inja-online/llm-gateway"
	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/dashboard"
	"github.com/inja-online/llm-gateway/hooks/ring"
	"github.com/inja-online/llm-gateway/hooks/sqlite"
	"github.com/inja-online/llm-gateway/proxy"
	"github.com/inja-online/llm-gateway/subauth"
)

func buildHandler(cfg *config.Config, opts ...gateway.Option) (http.Handler, error) {
	if cfg == nil || !cfg.Dashboard.IsEnabled() {
		return gateway.New(cfg, opts...)
	}
	ringSink := ring.New(cfg.Dashboard.Ring())
	extra := []gateway.Option{gateway.WithHook(ringSink)}
	sqlPath := ""
	if cfg.Hooks.SQLite != nil {
		sqlPath = cfg.Hooks.SQLite.Path
	}
	sqlSink, err := sqlite.New(sqlPath)
	if err != nil {
		return nil, err
	}
	extra = append(extra, gateway.WithHook(sqlSink))
	extra = append(extra, opts...)
	h, err := gateway.New(cfg, extra...)
	if err != nil {
		return nil, err
	}
	return mountOperator(cfg, h, ringSink, sqlSink), nil
}

func mountOperator(cfg *config.Config, h http.Handler, ringSink *ring.Sink, sqlSink *sqlite.Sink) http.Handler {
	root := http.NewServeMux()
	if uiEnabled {
		if dist, err := fs.Sub(uiFS, "dist"); err == nil {
			root.Handle("/ui/", http.StripPrefix("/ui", http.FileServer(http.FS(spaFS{dist}))))
		}
	}

	jsonlPath := ""
	if cfg.Hooks.JSONL != nil {
		switch cfg.Hooks.JSONL.Output {
		case "", "stdout", "stderr":
		default:
			jsonlPath = cfg.Hooks.JSONL.Output
		}
	}
	authPath, _ := subauth.ResolvePath()
	dashH := dashboard.New(dashboard.Options{
		Version:   version,
		AuthPath:  authPath,
		Ring:      ringSink,
		SQLite:    sqlSink,
		JSONLPath: jsonlPath,
		Dashboard: cfg.Dashboard,
	}).Handler()
	authH := proxy.WrapEdgeAuth(cfg, dashH)
	root.Handle("/v1/dashboard/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			dashH.ServeHTTP(w, r)
			return
		}
		authH.ServeHTTP(w, r)
	}))
	root.Handle("/", h)
	return root
}

// spaFS serves index.html when the path has no file (SPA routes).
type spaFS struct{ fs.FS }

func (s spaFS) Open(name string) (fs.File, error) {
	f, err := s.FS.Open(name)
	if err == nil {
		return f, nil
	}
	if path.Ext(name) != "" {
		return nil, err
	}
	return s.FS.Open("index.html")
}
