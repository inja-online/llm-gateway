package proxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
	anthropicegress "github.com/inja-online/llm-gateway/egress/anthropic"
	googleegress "github.com/inja-online/llm-gateway/egress/google"
	openaiegress "github.com/inja-online/llm-gateway/egress/openai"
	antingress "github.com/inja-online/llm-gateway/ingress/anthropic"
	googleingress "github.com/inja-online/llm-gateway/ingress/google"
	oaingress "github.com/inja-online/llm-gateway/ingress/openai"
)

func fixtureDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// proxy/ → repo root → testdata/fixtures/chat_translate
	return filepath.Join(filepath.Dir(file), "..", "testdata", "fixtures", "chat_translate")
}

func readFixture(t *testing.T, parts ...string) []byte {
	t.Helper()
	p := filepath.Join(append([]string{fixtureDir(t)}, parts...)...)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return b
}

// TestChatTranslateFixtures loads kitchen-sink requests and asserts preserved
// fields appear on egress while known drops (cache_control, etc.) do not.
func TestChatTranslateFixtures(t *testing.T) {
	t.Run("openai_to_anthropic", func(t *testing.T) {
		req, err := oaingress.ParseRequest(readFixture(t, "openai", "kitchen_sink.json"))
		if err != nil {
			t.Fatal(err)
		}
		assertPreservedOpenAI(t, req)
		body, err := anthropicegress.BuildRequest(req, "claude-test")
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		if out["model"] != "claude-test" {
			t.Fatalf("model: %v", out["model"])
		}
		if out["max_tokens"].(float64) != 256 {
			t.Fatalf("max_tokens: %v", out["max_tokens"])
		}
		// tools preserved
		if out["tools"] == nil {
			t.Fatal("tools dropped")
		}
		// cache_control must not appear (OpenAI ingress has none; ensure clean)
		if strings.Contains(string(body), "cache_control") {
			t.Fatalf("unexpected cache_control on Anthropic egress: %s", body)
		}
		// service_tier is OpenAI-only; must not appear on Anthropic wire
		if _, ok := out["service_tier"]; ok {
			t.Fatal("service_tier leaked to Anthropic")
		}
	})

	t.Run("openai_to_google", func(t *testing.T) {
		req, err := oaingress.ParseRequest(readFixture(t, "openai", "kitchen_sink.json"))
		if err != nil {
			t.Fatal(err)
		}
		body, err := googleegress.BuildRequest(req, "gemini-test")
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		json.Unmarshal(body, &out)
		if out["contents"] == nil {
			t.Fatal("contents missing")
		}
		// OpenAI has no safetySettings → must not invent
		if _, ok := out["safety_settings"]; ok {
			t.Fatal("invented safety_settings")
		}
	})

	t.Run("anthropic_to_openai", func(t *testing.T) {
		raw := readFixture(t, "anthropic", "kitchen_sink.json")
		// Ingress fixture includes cache_control on system/tools/content.
		if !strings.Contains(string(raw), "cache_control") {
			t.Fatal("fixture must include cache_control for drop test")
		}
		req, err := antingress.ParseRequest(raw)
		if err != nil {
			t.Fatal(err)
		}
		// cache_control is PT-only: not modeled on canonical → stripped on rebuild
		body, err := openaiegress.BuildRequest(req, "gpt-test")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "cache_control") {
			t.Fatalf("cache_control must be dropped on translate: %s", body)
		}
		var out map[string]any
		json.Unmarshal(body, &out)
		if out["model"] != "gpt-test" {
			t.Fatalf("model: %v", out["model"])
		}
		// system + tools + tool result preserved as messages
		msgs, _ := out["messages"].([]any)
		if len(msgs) < 2 {
			t.Fatalf("messages too short: %v", msgs)
		}
		if out["tools"] == nil {
			t.Fatal("tools dropped")
		}
	})

	t.Run("anthropic_to_anthropic_drops_cache_control", func(t *testing.T) {
		// Even Anthropic→Anthropic *translate* (not passthrough) strips cache_control.
		req, err := antingress.ParseRequest(readFixture(t, "anthropic", "kitchen_sink.json"))
		if err != nil {
			t.Fatal(err)
		}
		body, err := anthropicegress.BuildRequest(req, "claude-test")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "cache_control") {
			t.Fatalf("cache_control PT-only: must not reappear on translate egress: %s", body)
		}
		// System text still present
		if !strings.Contains(string(body), "You are concise") {
			t.Fatalf("system text lost: %s", body)
		}
	})

	t.Run("anthropic_to_google", func(t *testing.T) {
		req, err := antingress.ParseRequest(readFixture(t, "anthropic", "kitchen_sink.json"))
		if err != nil {
			t.Fatal(err)
		}
		body, err := googleegress.BuildRequest(req, "gemini-test")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "cache_control") {
			t.Fatal("cache_control leaked to Google")
		}
		var out map[string]any
		json.Unmarshal(body, &out)
		if out["contents"] == nil {
			t.Fatal("contents missing")
		}
	})

	t.Run("google_to_openai", func(t *testing.T) {
		req, err := googleingress.ParseRequest(readFixture(t, "google", "kitchen_sink.json"), "")
		if err != nil {
			t.Fatal(err)
		}
		if len(req.SafetySettings) == 0 {
			t.Fatal("fixture safety_settings not parsed")
		}
		body, err := openaiegress.BuildRequest(req, "gpt-test")
		if err != nil {
			t.Fatal(err)
		}
		// safety_settings are Google-only; must not appear on OpenAI wire
		if strings.Contains(string(body), "safety") {
			t.Fatalf("safety settings leaked: %s", body)
		}
		var out map[string]any
		json.Unmarshal(body, &out)
		if out["tools"] == nil {
			t.Fatal("tools dropped")
		}
	})

	t.Run("google_to_anthropic", func(t *testing.T) {
		req, err := googleingress.ParseRequest(readFixture(t, "google", "kitchen_sink.json"), "")
		if err != nil {
			t.Fatal(err)
		}
		body, err := anthropicegress.BuildRequest(req, "claude-test")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "safety") {
			t.Fatalf("safety settings leaked: %s", body)
		}
	})

	t.Run("google_to_google_keeps_safety", func(t *testing.T) {
		req, err := googleingress.ParseRequest(readFixture(t, "google", "kitchen_sink.json"), "")
		if err != nil {
			t.Fatal(err)
		}
		body, err := googleegress.BuildRequest(req, "gemini-test")
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		json.Unmarshal(body, &out)
		if out["safety_settings"] == nil {
			t.Fatalf("safety_settings not re-emitted: %s", body)
		}
	})
}

func assertPreservedOpenAI(t *testing.T, req *canonical.Request) {
	t.Helper()
	if req.MaxTokens != 256 {
		t.Fatalf("MaxTokens=%d", req.MaxTokens)
	}
	if req.Temperature == nil || *req.Temperature != 0.2 {
		t.Fatalf("Temperature=%v", req.Temperature)
	}
	if req.ServiceTier != "auto" {
		t.Fatalf("ServiceTier=%q", req.ServiceTier)
	}
	if len(req.Tools) != 1 || req.Tools[0].Name != "lookup" {
		t.Fatalf("Tools=%+v", req.Tools)
	}
	if len(req.System) == 0 {
		t.Fatal("system empty")
	}
}

func TestChatTranslatePolicyN(t *testing.T) {
	_, err := oaingress.ParseRequest([]byte(`{"model":"m","n":3,"messages":[]}`))
	if err == nil {
		t.Fatal("want error for n=3")
	}
	_, err = googleingress.ParseRequest([]byte(`{
		"model":"g","contents":[{"parts":[{"text":"x"}]}],
		"generation_config":{"candidateCount":3}
	}`), "")
	if err == nil {
		t.Fatal("want error for candidateCount=3")
	}
}

func TestChatTranslatePolicyNonFunctionTools(t *testing.T) {
	_, err := oaingress.ParseRequest([]byte(`{"model":"m","tools":[{"type":"custom","function":{"name":"x"}}],"messages":[]}`))
	if err == nil {
		t.Fatal("want error for non-function tools")
	}
}

func TestChatTranslateDropListFilePresent(t *testing.T) {
	raw := string(readFixture(t, "drops", "common_drops.txt"))
	for _, want := range []string{
		"openai.n>1",
		"cache_control",
		"openai.logprobs",
		"openai.seed",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("drop list missing %q", want)
		}
	}
}

// TestChatTranslateThinkingStreamRoundTrip is a minimal stream thinking
// fixture for one dialect pair (Anthropic thinking → OpenAI reasoning_content).
func TestChatTranslateThinkingStreamRoundTrip(t *testing.T) {
	// Non-stream response path: thinking block serializes to OpenAI reasoning_content.
	resp := &canonical.Response{
		ID:    "m1",
		Model: "m",
		Content: []canonical.Block{
			{Type: canonical.BlockThinking, Text: "let me think"},
			{Type: canonical.BlockText, Text: "answer"},
		},
		StopReason: canonical.StopEndTurn,
		Usage:      canonical.Usage{InputTokens: 1, OutputTokens: 2, HasUsage: true},
	}
	body, err := oaingress.SerializeResponse(resp, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "reasoning_content") {
		t.Fatalf("thinking not mapped: %s", body)
	}
	// Stream: thinking delta → reasoning_content chunk
	ser := oaingress.NewStreamSerializer(1)
	chunk := ser.Event(canonical.StreamEvent{Type: canonical.EventThinkingDelta, Index: 0, Text: "hmm"})
	if chunk == nil || !strings.Contains(string(chunk), "reasoning_content") {
		t.Fatalf("stream thinking: %s", chunk)
	}
}
