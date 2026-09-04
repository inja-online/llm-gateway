//go:build !nodb

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/inja-online/llm-gateway/hooks"
	_ "modernc.org/sqlite"
)

var _ hooks.Hook = (*Sink)(nil)

const schema = `
CREATE TABLE IF NOT EXISTS usage_events (
  request_id TEXT PRIMARY KEY,
  time TEXT NOT NULL,
  dialect_in TEXT,
  provider TEXT,
  model TEXT,
  upstream_model TEXT,
  modality TEXT,
  transport TEXT,
  tokens_in INTEGER,
  tokens_out INTEGER,
  cached_tokens INTEGER,
  cache_write_tokens INTEGER,
  reasoning_tokens INTEGER,
  estimated INTEGER,
  stream INTEGER,
  status TEXT,
  http_status INTEGER,
  latency_ms INTEGER,
  ttft_ms INTEGER,
  key_hash TEXT,
  dropped_fields TEXT,
  media TEXT
);
CREATE INDEX IF NOT EXISTS idx_usage_time ON usage_events(time);
`

const insertSQL = `INSERT OR REPLACE INTO usage_events (
  request_id, time, dialect_in, provider, model, upstream_model,
  modality, transport, tokens_in, tokens_out, cached_tokens,
  cache_write_tokens, reasoning_tokens, estimated, stream,
  status, http_status, latency_ms, ttft_ms, key_hash,
  dropped_fields, media
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

type Query struct {
	From, To                   time.Time
	Provider, Model, Status, Q string
	Limit                      int
	Cursor                     string
}

// Sink writes usage events to a CGO-free SQLite file. Empty path is a no-op.
type Sink struct {
	mu     sync.Mutex
	path   string
	db     *sql.DB
	insert *sql.Stmt
}

func New(path string) (*Sink, error) {
	s := &Sink{}
	if path == "" {
		return s, nil
	}
	if err := s.openLocked(path); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Sink) OnUsage(_ context.Context, ev hooks.UsageEvent) {
	// ponytail: global mutex; drop if busy so the proxy never waits on sqlite.
	if !s.mu.TryLock() {
		return
	}
	defer s.mu.Unlock()
	if s.insert == nil {
		return
	}
	dropped, media := encodeJSON(ev.DroppedFields), encodeJSON(ev.Media)
	_, _ = s.insert.Exec(
		ev.RequestID, ev.Time.UTC().Format(time.RFC3339Nano),
		ev.DialectIn, ev.Provider, ev.Model, ev.UpstreamModel,
		ev.Modality, ev.Transport, ev.TokensIn, ev.TokensOut, ev.CachedTokens,
		ev.CacheWriteTokens, ev.ReasoningTokens, boolInt(ev.Estimated), boolInt(ev.Stream),
		ev.Status, ev.HTTPStatus, ev.LatencyMS, ev.TTFTMS, ev.KeyHash,
		dropped, media,
	)
}

func (s *Sink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeLocked()
}

func (s *Sink) Path() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.path
}

func (s *Sink) SetPath(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.closeLocked(); err != nil {
		return err
	}
	if path == "" {
		return nil
	}
	return s.openLocked(path)
}

func (s *Sink) Query(ctx context.Context, q Query) ([]hooks.UsageEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil, nil
	}

	sqlStr := `SELECT request_id, time, dialect_in, provider, model, upstream_model,
		modality, transport, tokens_in, tokens_out, cached_tokens,
		cache_write_tokens, reasoning_tokens, estimated, stream,
		status, http_status, latency_ms, ttft_ms, key_hash,
		dropped_fields, media FROM usage_events WHERE 1=1`
	args := []any{}
	if !q.From.IsZero() {
		sqlStr += ` AND time >= ?`
		args = append(args, q.From.UTC().Format(time.RFC3339Nano))
	}
	if !q.To.IsZero() {
		sqlStr += ` AND time <= ?`
		args = append(args, q.To.UTC().Format(time.RFC3339Nano))
	}
	if q.Provider != "" {
		sqlStr += ` AND provider = ?`
		args = append(args, q.Provider)
	}
	if q.Model != "" {
		sqlStr += ` AND model = ?`
		args = append(args, q.Model)
	}
	if q.Status != "" {
		sqlStr += ` AND status = ?`
		args = append(args, q.Status)
	}
	if q.Q != "" {
		sqlStr += ` AND request_id LIKE ?`
		args = append(args, "%"+q.Q+"%")
	}
	if q.Cursor != "" {
		var ct, cid string
		err := s.db.QueryRowContext(ctx, `SELECT time, request_id FROM usage_events WHERE request_id = ?`, q.Cursor).Scan(&ct, &cid)
		if err == nil {
			sqlStr += ` AND (time < ? OR (time = ? AND request_id < ?))`
			args = append(args, ct, ct, cid)
		}
	}
	sqlStr += ` ORDER BY time DESC, request_id DESC`
	if q.Limit > 0 {
		sqlStr += ` LIMIT ?`
		args = append(args, q.Limit)
	}

	rows, err := s.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []hooks.UsageEvent
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (s *Sink) openLocked(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	_ = f.Close()
	_ = os.Chmod(path, 0o600)

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		_ = db.Close()
		return err
	}
	if _, err := db.Exec(`PRAGMA busy_timeout=0`); err != nil {
		_ = db.Close()
		return err
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return err
	}
	stmt, err := db.Prepare(insertSQL)
	if err != nil {
		_ = db.Close()
		return err
	}
	s.path = path
	s.db = db
	s.insert = stmt
	return nil
}

func (s *Sink) closeLocked() error {
	var err error
	if s.insert != nil {
		err = s.insert.Close()
		s.insert = nil
	}
	if s.db != nil {
		if e := s.db.Close(); err == nil {
			err = e
		}
		s.db = nil
	}
	s.path = ""
	return err
}

func scanEvent(rows *sql.Rows) (hooks.UsageEvent, error) {
	var ev hooks.UsageEvent
	var tstr string
	var estimated, stream int
	var dropped, media sql.NullString
	err := rows.Scan(
		&ev.RequestID, &tstr, &ev.DialectIn, &ev.Provider, &ev.Model, &ev.UpstreamModel,
		&ev.Modality, &ev.Transport, &ev.TokensIn, &ev.TokensOut, &ev.CachedTokens,
		&ev.CacheWriteTokens, &ev.ReasoningTokens, &estimated, &stream,
		&ev.Status, &ev.HTTPStatus, &ev.LatencyMS, &ev.TTFTMS, &ev.KeyHash,
		&dropped, &media,
	)
	if err != nil {
		return ev, err
	}
	ev.Time, _ = time.Parse(time.RFC3339Nano, tstr)
	ev.Estimated = estimated != 0
	ev.Stream = stream != 0
	if dropped.Valid && dropped.String != "" {
		_ = json.Unmarshal([]byte(dropped.String), &ev.DroppedFields)
	}
	if media.Valid && media.String != "" {
		var m hooks.MediaUsage
		if json.Unmarshal([]byte(media.String), &m) == nil {
			ev.Media = &m
		}
	}
	return ev, nil
}

func encodeJSON(v any) any {
	if v == nil {
		return nil
	}
	if s, ok := v.([]string); ok && len(s) == 0 {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return nil
	}
	return string(b)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
