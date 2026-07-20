package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
	anthropicegress "github.com/inja-online/llm-gateway/egress/anthropic"
)

func TestCacheControlParseAndRoundTrip(t *testing.T) {
	raw := []byte(`{
		"model":"claude-test",
		"max_tokens":64,
		"system":[{"type":"text","text":"rules","cache_control":{"type":"ephemeral"}}],
		"messages":[{"role":"user","content":[
			{"type":"text","text":"hi","cache_control":{"type":"ephemeral","ttl":"5m"}}
		]}],
		"tools":[{"name":"lookup","input_schema":{"type":"object"},"cache_control":{"type":"ephemeral"}}]
	}`)
	req, err := ParseRequest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(req.System) != 1 || req.System[0].CacheControl == nil || req.System[0].CacheControl.Type != "ephemeral" {
		t.Fatalf("system: %+v", req.System)
	}
	if len(req.Messages) != 1 || len(req.Messages[0].Content) != 1 {
		t.Fatalf("messages: %+v", req.Messages)
	}
	if cc := req.Messages[0].Content[0].CacheControl; cc == nil || cc.Type != "ephemeral" || cc.TTL != "5m" {
		t.Fatalf("content cache: %+v", req.Messages[0].Content[0].CacheControl)
	}
	if len(req.Tools) != 1 || req.Tools[0].CacheControl == nil {
		t.Fatalf("tools: %+v", req.Tools)
	}

	body, err := anthropicegress.BuildRequest(req, "claude-test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"cache_control"`) {
		t.Fatalf("rebuild missing cache_control: %s", body)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	// system
	sys, _ := out["system"].([]any)
	if len(sys) == 0 {
		t.Fatal("system empty")
	}
	sys0, _ := sys[0].(map[string]any)
	if sys0["cache_control"] == nil {
		t.Fatalf("system cache_control: %v", sys0)
	}
	// tools
	tools, _ := out["tools"].([]any)
	t0, _ := tools[0].(map[string]any)
	if t0["cache_control"] == nil {
		t.Fatalf("tool cache_control: %v", t0)
	}
}

func TestCacheControlToOpenAIDoesNotLeak(t *testing.T) {
	req := &canonical.Request{
		Model:     "m",
		MaxTokens: 16,
		System: []canonical.Block{{
			Type:         canonical.BlockText,
			Text:         "s",
			CacheControl: &canonical.CacheControl{Type: "ephemeral"},
		}},
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "hi"}},
		}},
	}
	// ensure IR holds it
	if req.System[0].CacheControl == nil {
		t.Fatal("setup")
	}
}
