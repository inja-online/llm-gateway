package proxy

import (
	"testing"

	"github.com/inja-online/llm-gateway/config"
)

func testConfig() *config.Config {
	cfg, err := config.Parse([]byte(`
listen: ":0"
providers:
  openai:   { kind: openai,        base_url: "https://api.openai.com/v1" }
  deepseek: { kind: openai_compat, base_url: "https://api.deepseek.com" }
  anthropic: { kind: anthropic,    base_url: "https://api.anthropic.com" }
defaults:
  openai_dialect: openai
aliases:
  fast: deepseek/deepseek-chat
`))
	if err != nil {
		panic(err)
	}
	return cfg
}

func TestResolve(t *testing.T) {
	cfg := testConfig()
	cases := []struct {
		model, wantProv, wantModel string
		wantErr                    bool
	}{
		{"deepseek/deepseek-chat", "deepseek", "deepseek-chat", false},
		{"anthropic/claude-sonnet-5", "anthropic", "claude-sonnet-5", false},
		{"gpt-4o", "openai", "gpt-4o", false},        // bare -> dialect default
		{"fast", "deepseek", "deepseek-chat", false}, // alias
		{"nosuch/model", "", "", true},               // unknown provider
	}
	for _, c := range cases {
		r, err := Resolve(cfg, DialectOpenAI, c.model)
		if c.wantErr {
			if err == nil {
				t.Errorf("Resolve(%q): want error, got %+v", c.model, r)
			}
			continue
		}
		if err != nil {
			t.Errorf("Resolve(%q): %v", c.model, err)
			continue
		}
		if r.ProviderName != c.wantProv || r.UpstreamModel != c.wantModel {
			t.Errorf("Resolve(%q) = (%s, %s), want (%s, %s)", c.model, r.ProviderName, r.UpstreamModel, c.wantProv, c.wantModel)
		}
	}
}

func TestResolveNoDefault(t *testing.T) {
	cfg := testConfig()
	if _, err := Resolve(cfg, DialectAnthropic, "claude-sonnet-5"); err == nil {
		t.Fatal("bare model with no anthropic default: want error")
	}
}

func TestCheckCapability(t *testing.T) {
	openai := config.Provider{Kind: config.KindOpenAI, BaseURL: "https://x"}
	if err := CheckCapability(openai, "openai", config.ModalityImageGen); err != nil {
		t.Fatalf("openai should allow image_gen: %v", err)
	}
	compat := config.Provider{Kind: config.KindOpenAICompat, BaseURL: "https://x"}
	if err := CheckCapability(compat, "deepseek", config.ModalityImageGen); err == nil {
		t.Fatal("openai_compat must deny image_gen without opt-in")
	}
	compat.Capabilities = &config.Capabilities{Text: true, ImageGen: true}
	if err := CheckCapability(compat, "deepseek", config.ModalityImageGen); err != nil {
		t.Fatalf("opt-in image_gen: %v", err)
	}
	if err := CheckCapability(compat, "deepseek", config.ModalityVideoGen); err == nil {
		t.Fatal("video_gen still off")
	}
}
