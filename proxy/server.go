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
	cfg          *config.Config
	hook         hooks.Hook
	client       *http.Client
	tokenSources map[string]TokenSource // provider name → ADC / SA token source
}

func NewServer(cfg *config.Config, hook hooks.Hook) *Server {
	if hook == nil {
		hook = hooks.Multi{}
	}
	return &Server{
		cfg:  cfg,
		hook: hook,
		client: &http.Client{
			// No overall timeout: streams are long-lived. Per-request contexts
			// propagate client disconnects; the transport bounds dials.
			Transport: &http.Transport{
				MaxIdleConnsPerHost:   32,
				IdleConnTimeout:       90 * time.Second,
				ResponseHeaderTimeout: 60 * time.Second,
			},
		},
		tokenSources: make(map[string]TokenSource),
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
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	return s.withEdgeAuth(mux)
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
//
// Auth modes:
//   - api_key (default): kind-specific header (Bearer / x-api-key / x-goog-api-key)
//   - bearer: always Authorization: Bearer
//   - adc / service_account: Authorization: Bearer from TokenSource (see applyAuthToken)
//
// For adc/service_account without a pre-fetched token, use applyAuthWithSource.
func applyAuth(req *http.Request, p config.Provider, clientKey string) {
	mode := p.AuthMode()
	if mode == config.AuthADC || mode == config.AuthServiceAccount {
		// Token must be supplied as clientKey by the caller (from TokenSource).
		if clientKey == "" {
			return
		}
		req.Header.Set("Authorization", "Bearer "+clientKey)
		return
	}
	key := clientKey
	if p.APIKeyEnv != "" {
		if env := envLookup(p.APIKeyEnv); env != "" {
			key = env
		}
	}
	if key == "" {
		return
	}
	if mode == config.AuthBearer {
		req.Header.Set("Authorization", "Bearer "+key)
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

// resolveUpstreamKey returns the credential to send upstream and whether ADC
// mode failed (token source missing or error). On ADC failure, msg is set.
func (s *Server) resolveUpstreamKey(r *http.Request, providerName string, p config.Provider) (key string, errMsg string) {
	mode := p.AuthMode()
	if mode == config.AuthADC || mode == config.AuthServiceAccount {
		ts := s.tokenSource(providerName)
		if ts == nil {
			return "", "provider " + providerName + ": auth " + mode + " requires a TokenSource (SetTokenSource); real Google ADC is optional — inject a source or use auth: api_key"
		}
		tok, err := ts.Token(r.Context())
		if err != nil {
			return "", "token source: " + err.Error()
		}
		return tok, ""
	}
	return clientKey(r), ""
}

// copyForwardHeaders copies selected client headers to the upstream request.
// Used for OpenRouter (HTTP-Referer, X-Title), OpenAI org/project, and similar.
func copyForwardHeaders(dst, src *http.Request) {
	for _, h := range []string{
		"HTTP-Referer",
		"Referer",
		"X-Title",
		"OpenAI-Organization",
		"OpenAI-Project",
		"anthropic-beta",
		"anthropic-version",
	} {
		if v := src.Header.Get(h); v != "" {
			dst.Header.Set(h, v)
		}
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
