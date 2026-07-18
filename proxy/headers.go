package proxy

import (
	"net/http"
	"strings"

	"github.com/inja-online/llm-gateway/config"
)

// OpenAI org/project request headers forwarded to openai / openai_compat only.
var openAIRequestHeaders = []string{
	"OpenAI-Organization",
	"OpenAI-Project",
	"OpenAI-Beta",
}

// Exact response headers always considered safe to relay.
var responseHeaderExact = map[string]bool{
	"Content-Type":              true,
	"Content-Length":            true,
	"Content-Encoding":          true,
	"Retry-After":               true,
	"X-Request-Id":              true,
	"Request-Id":                true,
	"Openai-Organization":       true,
	"Openai-Processing-Ms":      true,
	"Openai-Version":            true,
	"Anthropic-Organization-Id": true,
}

// Prefixes for rate-limit / quota response headers (case-insensitive match on canonical form).
var responseHeaderPrefixes = []string{
	"X-Ratelimit-",
	"Anthropic-Ratelimit-",
	"X-Goog-",
}

// hopByHopHeaders must never be blindly copied (RFC 9110).
var hopByHopHeaders = map[string]bool{
	"Connection":          true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailers":            true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
	"Set-Cookie":          true,
}

// forwardOpenAIRequestHeaders copies org/project/beta headers from the client
// onto an OpenAI-family upstream request. No-op for other provider kinds.
func forwardOpenAIRequestHeaders(up *http.Request, client *http.Request, p config.Provider) {
	if !isOpenAIFamily(p) {
		return
	}
	for _, h := range openAIRequestHeaders {
		if v := client.Header.Get(h); v != "" {
			up.Header.Set(h, v)
		}
	}
}

// copyAllowlistedResponseHeaders relays safe upstream response headers to the client.
// Content-Type is included so callers need not set it separately (but may overwrite).
func copyAllowlistedResponseHeaders(dst http.Header, src http.Header) {
	for k, vals := range src {
		if hopByHopHeaders[http.CanonicalHeaderKey(k)] {
			continue
		}
		if !allowResponseHeader(k) {
			continue
		}
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
}

func allowResponseHeader(name string) bool {
	can := http.CanonicalHeaderKey(name)
	if responseHeaderExact[can] {
		return true
	}
	// Prefix match against canonicalized name (Title-Case).
	lower := strings.ToLower(can)
	for _, p := range responseHeaderPrefixes {
		if strings.HasPrefix(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// setGatewayRequestID exposes the exchange request id without clobbering an
// upstream x-request-id already copied from the provider.
func setGatewayRequestID(w http.ResponseWriter, id string) {
	if id == "" {
		return
	}
	w.Header().Set("X-Gateway-Request-Id", id)
}
