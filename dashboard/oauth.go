package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/inja-online/llm-gateway/subauth"
)

var startChatGPT = subauth.StartChatGPTFlow
var startGrok = subauth.StartGrokDevice
var waitChatGPT = func(ctx context.Context, f *subauth.ChatGPTFlow) (subauth.Credential, error) {
	return f.Wait(ctx)
}
var pollGrok = func(ctx context.Context, d *subauth.GrokDevice) (subauth.Credential, error) {
	return d.Poll(ctx)
}

const oauthTTL = 10 * time.Minute

type oauthSession struct {
	kind            string
	authorizeURL    string
	userCode        string
	verificationURI string
	state           string
	err             string
	started         time.Time
	cancel          context.CancelFunc
	run             context.Context
}

func (s *Server) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_error", "bad json")
		return
	}
	switch body.Provider {
	case subauth.ProviderChatGPT:
		s.startChatGPT(w, r)
	case subauth.ProviderGrok:
		s.startGrok(w, r)
	case subauth.ProviderClaude, subauth.ProviderGemini:
		writeError(w, http.StatusBadRequest, "invalid_request_error", "wrong method")
	default:
		writeError(w, http.StatusBadRequest, "invalid_request_error", "unknown provider")
	}
}

func (s *Server) startChatGPT(w http.ResponseWriter, r *http.Request) {
	flow, err := startChatGPT(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}
	sess := s.putPending(subauth.ProviderChatGPT, &oauthSession{
		kind:         "redirect",
		authorizeURL: flow.AuthorizeURL,
		state:        "pending",
	})
	go func() {
		defer sess.cancel()
		defer flow.Close()
		cred, err := waitChatGPT(sess.run, flow)
		s.finishOAuth(subauth.ProviderChatGPT, sess, cred, err)
	}()
	writeJSON(w, http.StatusOK, map[string]any{
		"kind":          "redirect",
		"authorize_url": flow.AuthorizeURL,
		"state":         "pending",
	})
}

func (s *Server) startGrok(w http.ResponseWriter, r *http.Request) {
	dev, err := startGrok(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}
	sess := s.putPending(subauth.ProviderGrok, &oauthSession{
		kind:            "device",
		userCode:        dev.UserCode,
		verificationURI: dev.VerificationURI,
		state:           "pending",
	})
	go func() {
		defer sess.cancel()
		cred, err := pollGrok(sess.run, dev)
		s.finishOAuth(subauth.ProviderGrok, sess, cred, err)
	}()
	writeJSON(w, http.StatusOK, map[string]any{
		"kind":             "device",
		"user_code":        dev.UserCode,
		"verification_uri": dev.VerificationURI,
		"expires_in":       dev.ExpiresIn,
		"interval":         dev.Interval,
	})
}

func (s *Server) handleOAuthStatus(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	s.oauthMu.Lock()
	defer s.oauthMu.Unlock()
	sess := s.oauth[provider]
	if sess == nil || time.Since(sess.started) > oauthTTL {
		if sess != nil {
			delete(s.oauth, provider)
		}
		writeJSON(w, http.StatusOK, map[string]any{"state": "idle"})
		return
	}
	out := map[string]any{"state": sess.state, "kind": sess.kind}
	if sess.authorizeURL != "" {
		out["authorize_url"] = sess.authorizeURL
	}
	if sess.userCode != "" {
		out["user_code"] = sess.userCode
	}
	if sess.verificationURI != "" {
		out["verification_uri"] = sess.verificationURI
	}
	if sess.err != "" {
		out["error"] = sess.err
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleOAuthComplete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider string `json:"provider"`
		Token    string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_error", "bad json")
		return
	}
	if body.Provider != subauth.ProviderClaude {
		writeError(w, http.StatusBadRequest, "invalid_request_error", "claude paste only")
		return
	}
	tok := strings.TrimSpace(body.Token)
	if tok == "" {
		writeError(w, http.StatusBadRequest, "invalid_request_error", "empty token")
		return
	}
	c := subauth.Credential{
		Provider:    subauth.ProviderClaude,
		AccessToken: tok,
		Expiry:      time.Now().Add(365 * 24 * time.Hour),
		Source:      "setup_token",
	}
	if err := s.putCred(c); err != nil {
		writeError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleOAuthImport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_error", "bad json")
		return
	}
	var (
		c   subauth.Credential
		err error
	)
	switch body.Provider {
	case subauth.ProviderChatGPT:
		c, err = subauth.ImportChatGPTFromCodexCLI()
	case subauth.ProviderClaude:
		c, err = subauth.ImportClaudeFromCLI()
	case subauth.ProviderGrok:
		c, err = subauth.ImportGrokFromCLI()
	case subauth.ProviderGemini:
		c, err = subauth.ImportGeminiFromCLI()
	default:
		writeError(w, http.StatusBadRequest, "invalid_request_error", "unknown provider")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if err := s.putCred(c); err != nil {
		writeError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) putPending(provider string, sess *oauthSession) *oauthSession {
	sess.started = time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), oauthTTL)
	sess.cancel = cancel
	sess.run = ctx
	s.oauthMu.Lock()
	defer s.oauthMu.Unlock()
	if s.oauth == nil {
		s.oauth = map[string]*oauthSession{}
	}
	if old := s.oauth[provider]; old != nil && old.cancel != nil {
		old.cancel()
	}
	s.oauth[provider] = sess
	return sess
}

func (s *Server) finishOAuth(provider string, sess *oauthSession, cred subauth.Credential, err error) {
	if err == nil {
		err = s.putCred(cred)
	}
	s.oauthMu.Lock()
	defer s.oauthMu.Unlock()
	if s.oauth[provider] != sess {
		return
	}
	if err != nil {
		sess.state = "error"
		sess.err = err.Error()
		return
	}
	sess.state = "complete"
}

func (s *Server) putCred(c subauth.Credential) error {
	store, err := subauth.Load(s.opts.AuthPath)
	if err != nil {
		return err
	}
	store.Put(c)
	return store.Save(s.opts.AuthPath)
}
