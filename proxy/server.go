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
	sessions     *sessionLimiter
	metrics      *gatewayMetrics
}

func NewServer(cfg *config.Config, hook hooks.Hook) *Server {
	if hook == nil {
		hook = hooks.Multi{}
	}
	m := newGatewayMetrics()
	// Record usage into Prometheus registry for GET /metrics (#95/#154).
	hook = metricsHook{m: m, next: hook}
	return &Server{
		cfg:      cfg,
		hook:     hook,
		metrics:  m,
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
		tokenSources: make(map[string]TokenSource),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", s.handleOpenAI)
	// Stored Chat Completions (store=true) resource API — openai family only.
	mux.HandleFunc("GET /v1/chat/completions", s.handleChatCompletionsList)
	mux.HandleFunc("GET /v1/chat/completions/{id}", s.handleChatCompletionsGet)
	mux.HandleFunc("POST /v1/chat/completions/{id}", s.handleChatCompletionsUpdate)
	mux.HandleFunc("DELETE /v1/chat/completions/{id}", s.handleChatCompletionsDelete)
	// Experimental: OpenAI Completions + DeepSeek FIM (prefix/suffix). See README.
	mux.HandleFunc("POST /v1/completions", s.handleCompletions)
	mux.HandleFunc("POST /beta/completions", s.handleCompletions)
	mux.HandleFunc("POST /v1/messages", s.handleAnthropic)
	mux.HandleFunc("POST /v1/messages/count_tokens", s.handleCountTokens)
	// Anthropic Message Batches (upstream-owned; gateway does not store results).
	mux.HandleFunc("POST /v1/messages/batches", s.handleBatchesCreate)
	mux.HandleFunc("GET /v1/messages/batches", s.handleBatchesList)
	mux.HandleFunc("GET /v1/messages/batches/{id}", s.handleBatchesGet)
	mux.HandleFunc("POST /v1/messages/batches/{id}/cancel", s.handleBatchesCancel)
	mux.HandleFunc("GET /v1/messages/batches/{id}/results", s.handleBatchesResults)
	// OpenAI Fine-tuning Jobs (openai / openai_compat; upstream-owned).
	mux.HandleFunc("POST /v1/fine_tuning/jobs", s.handleFineTuningJobsCreate)
	mux.HandleFunc("GET /v1/fine_tuning/jobs", s.handleFineTuningJobsList)
	mux.HandleFunc("GET /v1/fine_tuning/jobs/{id}", s.handleFineTuningJobsGet)
	mux.HandleFunc("POST /v1/fine_tuning/jobs/{id}/cancel", s.handleFineTuningJobsCancel)
	mux.HandleFunc("GET /v1/fine_tuning/jobs/{id}/events", s.handleFineTuningJobsEvents)
	mux.HandleFunc("GET /v1/fine_tuning/jobs/{id}/checkpoints", s.handleFineTuningJobsCheckpoints)
	// OpenAI Batches API (openai / openai_compat; upstream-owned).
	mux.HandleFunc("POST /v1/batches", s.handleOpenAIBatchesCreate)
	mux.HandleFunc("GET /v1/batches", s.handleOpenAIBatchesList)
	mux.HandleFunc("GET /v1/batches/{id}", s.handleOpenAIBatchesGet)
	mux.HandleFunc("POST /v1/batches/{id}/cancel", s.handleOpenAIBatchesCancel)
	mux.HandleFunc("GET /v1/models", s.handleModelsList)
	mux.HandleFunc("GET /v1/models/{id...}", s.handleModelsGet)
	mux.HandleFunc("POST /v1/embeddings", s.handleEmbeddings)
	mux.HandleFunc("POST /v1/responses", s.handleResponses)
	mux.HandleFunc("GET /v1/responses/{id}", s.handleResponsesGet)
	mux.HandleFunc("DELETE /v1/responses/{id}", s.handleResponsesDelete)
	mux.HandleFunc("POST /v1/files", s.handleFilesUpload)
	mux.HandleFunc("GET /v1/files", s.handleFilesList)
	mux.HandleFunc("GET /v1/files/{id}", s.handleFilesGet)
	mux.HandleFunc("GET /v1/files/{id}/content", s.handleFilesContent)
	mux.HandleFunc("DELETE /v1/files/{id}", s.handleFilesDelete)
	// Vector stores / Uploads / Containers (openai family; upstream-owned) (#113).
	mux.HandleFunc("POST /v1/vector_stores", s.handleVectorStoresRoot)
	mux.HandleFunc("GET /v1/vector_stores", s.handleVectorStoresRoot)
	mux.HandleFunc("GET /v1/vector_stores/{id}", s.handleVectorStoresID)
	mux.HandleFunc("POST /v1/vector_stores/{id}", s.handleVectorStoresID)
	mux.HandleFunc("DELETE /v1/vector_stores/{id}", s.handleVectorStoresID)
	mux.HandleFunc("GET /v1/vector_stores/{id}/{rest...}", s.handleVectorStoresID)
	mux.HandleFunc("POST /v1/vector_stores/{id}/{rest...}", s.handleVectorStoresID)
	mux.HandleFunc("DELETE /v1/vector_stores/{id}/{rest...}", s.handleVectorStoresID)
	mux.HandleFunc("POST /v1/uploads", s.handleUploadsRoot)
	mux.HandleFunc("GET /v1/uploads/{id}", s.handleUploadsID)
	mux.HandleFunc("POST /v1/uploads/{id}/{rest...}", s.handleUploadsID)
	mux.HandleFunc("POST /v1/containers", s.handleContainersRoot)
	mux.HandleFunc("GET /v1/containers", s.handleContainersRoot)
	mux.HandleFunc("GET /v1/containers/{id}", s.handleContainersID)
	mux.HandleFunc("DELETE /v1/containers/{id}", s.handleContainersID)
	mux.HandleFunc("GET /v1/containers/{id}/{rest...}", s.handleContainersID)
	mux.HandleFunc("POST /v1/containers/{id}/{rest...}", s.handleContainersID)
	mux.HandleFunc("POST /v1/moderations", s.handleModerations)
	// Moonshot / Kimi helpers (openai_compat; regional base via provider).
	mux.HandleFunc("POST /v1/tokenizers/estimate-token-count", s.handleMoonshotEstimateTokens)
	mux.HandleFunc("GET /v1/users/me/balance", s.handleMoonshotBalance)
	// Conversations API: intentional 501 stubs (stateless gateway; see README).
	mux.HandleFunc("POST /v1/conversations", s.handleConversationsNotImplemented)
	mux.HandleFunc("GET /v1/conversations", s.handleConversationsNotImplemented)
	mux.HandleFunc("GET /v1/conversations/{id}", s.handleConversationsNotImplemented)
	mux.HandleFunc("POST /v1/conversations/{id}", s.handleConversationsNotImplemented)
	mux.HandleFunc("DELETE /v1/conversations/{id}", s.handleConversationsNotImplemented)
	mux.HandleFunc("GET /v1/conversations/{id}/{rest...}", s.handleConversationsNotImplemented)
	mux.HandleFunc("POST /v1/conversations/{id}/{rest...}", s.handleConversationsNotImplemented)
	mux.HandleFunc("DELETE /v1/conversations/{id}/{rest...}", s.handleConversationsNotImplemented)
	mux.HandleFunc("GET /v1/realtime", s.handleRealtime)
	mux.HandleFunc("POST /v1beta/models/{action}", s.handleGoogle)
	mux.HandleFunc("GET /v1beta/models", s.handleGoogleModelsList)
	// Single path param — Live WS (:bidiGenerateContent) vs model get.
	mux.HandleFunc("GET /v1beta/models/{model}", s.handleGoogleModelOrLive)
	mux.HandleFunc("GET /v1beta/videos/{name...}", s.handleGoogleVideoPoll)
	mux.HandleFunc("POST /v1/images", s.handleAnthropicImagesGenerate)
	mux.HandleFunc("POST /v1/images/edits", s.handleImagesEdits)
	mux.HandleFunc("POST /v1/images/generations", s.handleImagesGenerations)
	mux.HandleFunc("POST /v1/images/variations", s.handleImagesVariations)
	mux.HandleFunc("POST /v1/videos", s.handleVideosCreate)
	mux.HandleFunc("GET /v1/videos/{id}", s.handleVideosGet)
	mux.HandleFunc("GET /v1/videos/{id}/content", s.handleVideosContent)
	mux.HandleFunc("POST /v1/audio/speech", s.handleAudioSpeech)
	mux.HandleFunc("POST /v1/audio/transcriptions", s.handleAudioTranscriptions)
	mux.HandleFunc("POST /v1/audio/translations", s.handleAudioTranslations)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	// Optional upstream probes (health_checks.enabled); not process liveness.
	mux.HandleFunc("GET /v1/health/providers", s.handleProviderHealth)
	// Prometheus text metrics (low cardinality; always on).
	mux.HandleFunc("GET /metrics", s.handleMetrics)
	return s.withEdgeAuth(mux)
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
// OpenAI org/project headers are NOT included here — use forwardOpenAIRequestHeaders
// so they only reach openai / openai_compat providers.
func copyForwardHeaders(dst, src *http.Request) {
	for _, h := range []string{
		"HTTP-Referer",
		"Referer",
		"X-Title",
		"anthropic-beta",
		"anthropic-version",
		"X-Client-Request-Id", // client correlation (#151)
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

// handleGoogleModelOrLive dispatches GET /v1beta/models/{model}:
// Live WS when the path ends with :bidiGenerateContent, else model get.
func (s *Server) handleGoogleModelOrLive(w http.ResponseWriter, r *http.Request) {
	model := r.PathValue("model")
	if strings.HasSuffix(model, ":bidiGenerateContent") {
		r.SetPathValue("action", model)
		s.handleGoogleLive(w, r)
		return
	}
	// Other method-style suffixes are POST-only; do not treat as model ids.
	if strings.Contains(model, ":") {
		http.NotFound(w, r)
		return
	}
	s.handleGoogleModelGet(w, r)
}
