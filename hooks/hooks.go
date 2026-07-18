// Package hooks defines the usage-event contract — the gateway's only
// metering output. There is no database: consumers observe usage through
// a Hook (in-process), the JSONL sink, or the webhook sink.
package hooks

import (
	"context"
	"time"
)

// Status values for UsageEvent.Status.
const (
	StatusOK            = "ok"
	StatusUpstreamError = "upstream_error"
	StatusClientAbort   = "client_abort"
	StatusBadRequest    = "bad_request"
)

// Transport values for UsageEvent.Transport.
const (
	TransportHTTP      = "http"
	TransportWebSocket = "websocket"
)

// MediaUnitKind values for MediaUsage.UnitKind (billing hooks; not prices).
const (
	MediaUnitImage          = "image"
	MediaUnitVideoSecond    = "video_second"
	MediaUnitAudioCharacter = "audio_character"
	MediaUnitAudioMinute    = "audio_minute"
	MediaUnitSessionMinute  = "session_minute"
)

// MediaUsage carries non-token billable dimensions for media/realtime requests.
// Omitted from JSON when empty (all zero / blank).
type MediaUsage struct {
	Units      int    `json:"units,omitempty"`
	UnitKind   string `json:"unit_kind,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
	Size       string `json:"size,omitempty"`
	Format     string `json:"format,omitempty"`
}

// UsageEvent is emitted exactly once per proxied request (or realtime session end).
type UsageEvent struct {
	RequestID     string    `json:"request_id"`
	Time          time.Time `json:"time"`
	DialectIn     string    `json:"dialect_in"`
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`          // public id as the client sent it
	UpstreamModel string    `json:"upstream_model"` // id sent upstream
	// Modality is text|image_gen|video_gen|audio_speech|audio_transcribe|realtime.
	// Empty means legacy text chat (treat as text).
	Modality string `json:"modality,omitempty"`
	// Transport is http|websocket. Empty means http.
	Transport string `json:"transport,omitempty"`
	TokensIn  int    `json:"tokens_in"`
	TokensOut int    `json:"tokens_out"`
	Estimated bool   `json:"estimated"` // true when upstream reported no usage
	// Media is set for image/video/audio/realtime metering when known.
	Media      *MediaUsage `json:"media,omitempty"`
	Stream     bool        `json:"stream"`
	Status     string      `json:"status"`
	HTTPStatus int         `json:"http_status"`
	LatencyMS  int64       `json:"latency_ms"`
	TTFTMS     int64       `json:"ttft_ms,omitempty"`
	// KeyHash is a short sha256 prefix of the forwarded credential — enough to
	// correlate usage per key without ever storing the key itself.
	KeyHash string `json:"key_hash,omitempty"`
}

// Hook receives usage events. Implementations must not block: the proxy calls
// OnUsage synchronously after the response completes.
type Hook interface {
	OnUsage(ctx context.Context, ev UsageEvent)
}

// Multi fans one event out to several hooks.
type Multi []Hook

func (m Multi) OnUsage(ctx context.Context, ev UsageEvent) {
	for _, h := range m {
		h.OnUsage(ctx, ev)
	}
}

// Func adapts a function to the Hook interface.
type Func func(ctx context.Context, ev UsageEvent)

func (f Func) OnUsage(ctx context.Context, ev UsageEvent) { f(ctx, ev) }
