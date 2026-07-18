package canonical

import "encoding/json"

// Video job status values (canonical set).
const (
	VideoStatusQueued     = "queued"
	VideoStatusProcessing = "processing"
	VideoStatusCompleted  = "completed"
	VideoStatusFailed     = "failed"
)

// Video operation kinds.
const (
	VideoOpCreate = "create"
	VideoOpGet    = "get"
)

// VideoGenRequest is the dialect-neutral video generation request.
type VideoGenRequest struct {
	Model      string
	Prompt     string
	Duration   float64 // seconds when known
	Resolution string
	Aspect     string
	// References are optional source images/videos (best-effort across vendors).
	References []ImageSource
	Operation  string // create | get
	JobID      string // for poll
	Extra      map[string]json.RawMessage
}

// VideoResult holds completed video assets.
type VideoResult struct {
	URL       string
	B64       string
	MediaType string
}

// VideoGenResponse is create/poll status for a video job.
type VideoGenResponse struct {
	ID       string
	Model    string
	Status   string
	Progress float64 // 0–100 when known
	Result   *VideoResult
	Error    string
	Usage    Usage
}
