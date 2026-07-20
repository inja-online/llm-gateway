package proxy

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// withEdgeAuth wraps h so that when edge_auth is enabled, every request except
// GET /healthz must present a configured key via Authorization: Bearer … or
// x-api-key. Comparison is constant-time against each configured key.
//
// Edge auth is independent of upstream credentials: with api_key_env set, the
// client only needs a valid edge key; the server substitutes the upstream key.
// key_hash on usage events still reflects the upstream-forwarded credential
// (client key or api_key_env), not a separate edge-only identity.
func (s *Server) withEdgeAuth(h http.Handler) http.Handler {
	if s.cfg == nil || !s.cfg.EdgeAuth.Enabled {
		return h
	}
	keys := s.cfg.EdgeKeys()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Process liveness + scrape endpoints stay open when edge auth is on.
		if r.URL.Path == "/healthz" || r.URL.Path == "/metrics" {
			h.ServeHTTP(w, r)
			return
		}
		cred := edgeCredential(r)
		if cred == "" || !edgeKeyOK(cred, keys) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("WWW-Authenticate", `Bearer realm="llm-gateway"`)
			w.WriteHeader(http.StatusUnauthorized)
			// Dialect-neutral OpenAI-shaped envelope (usable by most SDKs).
			w.Write([]byte(`{"error":{"message":"invalid or missing edge credentials","type":"authentication_error","code":"invalid_edge_auth"}}`))
			return
		}
		h.ServeHTTP(w, r)
	})
}

func edgeCredential(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); auth != "" {
		const p = "Bearer "
		if strings.HasPrefix(auth, p) {
			return strings.TrimSpace(auth[len(p):])
		}
		// Also accept raw Authorization value if not Bearer-prefixed.
		return strings.TrimSpace(auth)
	}
	if k := r.Header.Get("x-api-key"); k != "" {
		return strings.TrimSpace(k)
	}
	return ""
}

func edgeKeyOK(cred string, keys []string) bool {
	cb := []byte(cred)
	ok := false
	for _, k := range keys {
		kb := []byte(k)
		// subtle.ConstantTimeCompare requires equal length; pad comparison
		// by checking length in constant fashion then comparing when equal.
		if subtle.ConstantTimeEq(int32(len(cb)), int32(len(kb))) == 1 &&
			subtle.ConstantTimeCompare(cb, kb) == 1 {
			ok = true
		}
	}
	return ok
}
