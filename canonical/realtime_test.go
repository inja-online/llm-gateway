package canonical

import "testing"

func TestUnsupportedRealtimeBridgeCodeStable(t *testing.T) {
	// Contract: fail-closed code must remain stable for clients and docs.
	if UnsupportedRealtimeBridge != "unsupported_realtime_bridge" {
		t.Fatalf("UnsupportedRealtimeBridge = %q", UnsupportedRealtimeBridge)
	}
}

func TestRealtimePlaceholderTypesConstructible(t *testing.T) {
	cfg := RealtimeSessionConfig{
		Model:      "gpt-4o-realtime-preview",
		Voice:      "alloy",
		Modalities: []string{"text", "audio"},
		Extra:      map[string]any{"note": "placeholder"},
	}
	ev := RealtimeEvent{
		Type:      RealtimeEventSessionUpdate,
		Session:   &cfg,
		Text:      "",
		MediaType: "",
	}
	if ev.Session.Model != cfg.Model {
		t.Fatalf("session model = %q", ev.Session.Model)
	}
	if RealtimeEventError != "error" {
		t.Fatalf("RealtimeEventError = %q", RealtimeEventError)
	}
}
