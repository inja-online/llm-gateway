package google

import (
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestFinishToStopCatalog(t *testing.T) {
	cases := map[string]string{
		"STOP":                     canonical.StopEndTurn,
		"MAX_TOKENS":               canonical.StopMaxTokens,
		"SAFETY":                   canonical.StopRefusal,
		"MALFORMED_FUNCTION_CALL":  canonical.StopToolUse,
		"OTHER":                    canonical.StopEndTurn,
		"CONTENT_FILTER":           canonical.StopRefusal,
	}
	for fr, want := range cases {
		if got := finishToStop(fr, nil); got != want {
			t.Fatalf("%s: %s want %s", fr, got, want)
		}
	}
	// tool use overrides
	if finishToStop("STOP", []canonical.Block{{Type: canonical.BlockToolUse}}) != canonical.StopToolUse {
		t.Fatal("tool override")
	}
}
