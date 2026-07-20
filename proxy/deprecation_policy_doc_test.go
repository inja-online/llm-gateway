package proxy

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Locks docs/deprecation-policy.md decisions for #103 so the policy cannot
// silently drift away from accepted choices (passthrough never drops; no Warning
// header; x-gateway-dropped-fields deferred; semver table present).

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// proxy/ → repo root
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

func TestDeprecationPolicyDocDecisions(t *testing.T) {
	root := repoRoot(t)
	docPath := filepath.Join(root, "docs", "deprecation-policy.md")
	raw, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	needles := []string{
		"Passthrough",
		"Never drop",
		"Warning",
		"Not used",
		"x-gateway-dropped-fields",
		"Not implemented in v1",
		"MAJOR",
		"MINOR",
		"PATCH",
		"common_drops.txt",
		"#103",
	}
	for _, n := range needles {
		if !strings.Contains(doc, n) {
			t.Errorf("deprecation-policy.md missing required needle %q", n)
		}
	}
}

func TestCommonDropsFixtureExists(t *testing.T) {
	root := repoRoot(t)
	p := filepath.Join(root, "testdata", "fixtures", "chat_translate", "drops", "common_drops.txt")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	// Anchors that translation fidelity work must keep documenting.
	for _, n := range []string{"openai.logprobs", "openai.stream_options", "cache_control"} {
		if !strings.Contains(body, n) {
			t.Errorf("common_drops.txt missing %q", n)
		}
	}
}
