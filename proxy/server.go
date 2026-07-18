// Package proxy implements the gateway's HTTP pipeline: route, forward,
// meter. It holds no state beyond config and hooks — no database.
package proxy

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

type Server struct {
	cfg      *config.Config
	hook     hooks.Hook
	client   *http.Client
	sessions *sessionLimiter
}

func NewServer(cfg *config.Config, hook hooks.Hook) *Server {
	if hook == nil {
		hook = hooks.Multi{}
	}
	return &Server{
		cfg:  cfg,
		hook: hook,
		sessions: newSessionLimiter(cfg.Realtime.MaxSessions, cfg.Realtime.MaxSessionMinutes),
		client: &http.Client{
			// No overall timeout: streams are long-lived. Per-request contexts
			// propagate client disconnects; the transport bounds dials.
			Transport: &http.Transport{
				MaxIdleConnsPerHost:   32,
				IdleConnTimeout:       90 * time.Second,
				ResponseHeaderTimeout: 60 * time.Second,
			},
		},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", s.handleOpenAI)
	mux.HandleFunc("POST /v1/messages", s.handleAnthropic)
	mux.HandleFunc("POST /v1/messages/count_tokens", s.handleCountTokens)
	// Native Gemini generateContent / streamGenerateContent (model in path).
	mux.HandleFunc("POST /v1beta/models/{action}", s.handleGoogle)
	// OpenAI-compatible image & video generation (passthrough to openai / openai_compat).
	mux.HandleFunc("POST /v1/images/generations", s.handleImagesGenerations)
	mux.HandleFunc("POST /v1/images/edits", s.handleImagesEdits)
	mux.HandleFunc("POST /v1/images/variations", s.handleImagesVariations)
	mux.HandleFunc("POST /v1/videos", s.handleVideosCreate)
	mux.HandleFunc("GET /v1/videos/{id}", s.handleVideosGet)
	// OpenAI Responses API (create + stored resource lifecycle).
	mux.HandleFunc("POST /v1/responses", s.handleResponses)
	mux.HandleFunc("GET /v1/responses/{id}", s.handleResponsesGet)
	mux.HandleFunc("DELETE /v1/responses/{id}", s.handleResponsesDelete)
	// OpenAI Files API (no gateway persistence).
	mux.HandleFunc("POST /v1/files", s.handleFilesUpload)
	mux.HandleFunc("GET /v1/files", s.handleFilesList)
	mux.HandleFunc("GET /v1/files/{id}", s.handleFilesGet)
	mux.HandleFunc("DELETE /v1/files/{id}", s.handleFilesDelete)
	mux.HandleFunc("GET /v1/files/{id}/content", s.handleFilesContent)
	// Moderations.
	mux.HandleFunc("POST /v1/moderations", s.handleModerations)
	// Realtime WebSocket (OpenAI Realtime + Google Live skeleton).
	mux.HandleFunc("GET /v1/realtime", s.handleRealtime)
	mux.HandleFunc("GET /v1beta/models/{action}", s.handleGoogleLiveRoute)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	return mux
}

// handleGoogleLiveRoute dispatches GET /v1beta/models/{action} for Live WS only.
// Non-bidi actions return 404 (POST generateContent uses the POST handler).
func (s *Server) handleGoogleLiveRoute(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	if strings.HasSuffix(action, ":bidiGenerateContent") {
		s.handleGoogleLive(w, r)
		return
	}
	http.NotFound(w, r)
}

// clientKey extracts the credential the client sent (OpenAI Bearer, Anthropic
// x-api-key, or Google x-goog-api-key). The gateway never validates it — it forwards.
func clientKey(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); auth != "" {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if k := r.Header.Get("x-api-key"); k != "" {
		return k
	}
	return r.Header.Get("x-goog-api-key")
}

// applyAuth sets the upstream auth header. A configured api_key_env replaces
// the client key entirely; otherwise the client key is forwarded.
func applyAuth(req *http.Request, p config.Provider, clientKey string) {
	key := clientKey
	if p.APIKeyEnv != "" {
		if env := envLookup(p.APIKeyEnv); env != "" {
			key = env
		}
	}
	if key == "" {
		return
	}
	switch p.Kind {
	case config.KindAnthropic:
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	case config.KindGoogle:
		req.Header.Set("x-goog-api-key", key)
	default:
		req.Header.Set("Authorization", "Bearer "+key)
	}
}

func hashKey(key string) string {
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:12]
}

func newRequestID() string {
	var b [8]byte
	rand.Read(b[:])
	return "req_" + hex.EncodeToString(b[:])
}
