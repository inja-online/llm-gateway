package dashboard

import (
	"encoding/json"
	"net/http"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks/sqlite"
)

func (s *Server) handleGetSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.settings())
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SQLitePath *string `json:"sqlite_path"`
		RingSize   *int    `json:"ring_size"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_error", "", "bad json")
		return
	}
	if body.RingSize != nil {
		n := *body.RingSize
		if n > config.MaxDashboardRingSize {
			writeError(w, http.StatusBadRequest, "invalid_request_error", "", "ring_size too large")
			return
		}
		if n <= 0 {
			n = config.DefaultDashboardRingSize
		}
		s.ringSize = n
		s.opts.Dashboard.RingSize = n
		if s.opts.Ring != nil {
			s.opts.Ring.Resize(n)
		}
	}
	if body.SQLitePath != nil {
		p := *body.SQLitePath
		if p != "" && !sqlite.Available() {
			writeError(w, http.StatusBadRequest, "invalid_request_error", "sqlite_disabled", "sqlite disabled")
			return
		}
		if s.opts.SQLite != nil {
			if err := s.opts.SQLite.SetPath(p); err != nil {
				writeError(w, http.StatusInternalServerError, "api_error", "", err.Error())
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, s.settings())
}

func (s *Server) settings() map[string]any {
	path := ""
	if s.opts.SQLite != nil {
		path = s.opts.SQLite.Path()
	}
	return map[string]any{
		"ring_size":      s.ringSize,
		"sqlite_path":    path,
		"sqlite_enabled": sqlite.Available() && path != "",
		"jsonl_output":   s.opts.JSONLPath,
	}
}
