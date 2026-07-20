package google

import (
	"encoding/json"
	"testing"

	googleegress "github.com/inja-online/llm-gateway/egress/google"
)

func TestCachedContentRoundTrip(t *testing.T) {
	raw := []byte(`{
		"model":"gemini-test",
		"cachedContent":"cachedContents/abc123",
		"contents":[{"role":"user","parts":[{"text":"hi"}]}]
	}`)
	req, err := ParseRequest(raw, "")
	if err != nil {
		t.Fatal(err)
	}
	if req.CachedContent != "cachedContents/abc123" {
		t.Fatalf("IR CachedContent=%q", req.CachedContent)
	}
	body, err := googleegress.BuildRequest(req, "gemini-test")
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out["cachedContent"] != "cachedContents/abc123" {
		t.Fatalf("%s", body)
	}
}

func TestCachedContentSnakeCase(t *testing.T) {
	raw := []byte(`{
		"model":"gemini-test",
		"cached_content":"cachedContents/snake",
		"contents":[{"parts":[{"text":"x"}]}]
	}`)
	req, err := ParseRequest(raw, "")
	if err != nil {
		t.Fatal(err)
	}
	if req.CachedContent != "cachedContents/snake" {
		t.Fatalf("%q", req.CachedContent)
	}
}
