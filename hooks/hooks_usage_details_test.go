package hooks

import (
	"encoding/json"
	"testing"
	"time"
)

func TestUsageEventDetailsJSON(t *testing.T) {
	ev := UsageEvent{
		RequestID:        "r1",
		Time:             time.Unix(0, 0).UTC(),
		DialectIn:        "openai",
		Provider:         "p",
		Model:            "m",
		UpstreamModel:    "um",
		TokensIn:         100,
		TokensOut:        50,
		CachedTokens:     80,
		CacheWriteTokens: 10,
		ReasoningTokens:  12,
		Status:           StatusOK,
	}
	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{
		`"cached_tokens":80`,
		`"cache_write_tokens":10`,
		`"reasoning_tokens":12`,
		`"tokens_in":100`,
	} {
		if !jsonContains(s, want) {
			t.Fatalf("missing %s in %s", want, s)
		}
	}
}

func TestUsageEventOmitsZeroDetails(t *testing.T) {
	ev := UsageEvent{RequestID: "r", Status: StatusOK, TokensIn: 1, TokensOut: 1}
	raw, _ := json.Marshal(ev)
	s := string(raw)
	for _, ban := range []string{"cached_tokens", "cache_write_tokens", "reasoning_tokens"} {
		if jsonContains(s, ban) {
			t.Fatalf("should omit %s: %s", ban, s)
		}
	}
}

func jsonContains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}
