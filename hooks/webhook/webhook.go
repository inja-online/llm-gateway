// Package webhook posts usage events as JSON to an HTTP endpoint.
// Delivery is asynchronous so the request path never blocks on the sink.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/inja-online/llm-gateway/hooks"
)

const defaultTimeout = 3 * time.Second

// Sink POSTs each usage event to URL.
type Sink struct {
	url    string
	client *http.Client
}

// New builds a webhook sink. timeout <= 0 uses a 3s default.
func New(url string, timeout time.Duration) *Sink {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Sink{
		url: url,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// OnUsage queues a non-blocking POST. Failures are logged; they never
// affect the client response (the usage event was already recorded on the
// request path).
func (s *Sink) OnUsage(ctx context.Context, ev hooks.UsageEvent) {
	// Detach from the request context so a client disconnect cannot cancel delivery.
	body, err := json.Marshal(ev)
	if err != nil {
		return
	}
	go s.post(body)
}

func (s *Sink) post(body []byte) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		log.Printf("hooks.webhook: build request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		log.Printf("hooks.webhook: post %s: %v", s.url, err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Printf("hooks.webhook: post %s: status %d", s.url, resp.StatusCode)
	}
}
