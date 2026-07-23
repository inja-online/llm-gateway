package google

import (
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestThoughtSignatureRoundTrip(t *testing.T) {
	body := []byte(`{"candidates":[{"content":{"parts":[{"text":"plan","thought":true,"thoughtSignature":"sig123"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1}}`)
	resp, err := ParseResponse(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Content) != 1 || resp.Content[0].Signature != "sig123" {
		t.Fatalf("%+v", resp.Content)
	}
	out, err := BuildRequest(&canonical.Request{
		Messages: []canonical.Message{{
			Role: canonical.RoleAssistant,
			Content: []canonical.Block{{Type: canonical.BlockThinking, Text: "plan", Signature: "sig123"}},
		}},
	}, "m")
	if err != nil {
		t.Fatal(err)
	}
	if !containsAll(string(out), "thoughtSignature", "sig123") {
		t.Fatalf("%s", out)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !contains(s, p) {
			return false
		}
	}
	return true
}
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
