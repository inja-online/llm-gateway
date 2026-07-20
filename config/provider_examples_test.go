package config

import (
	"strings"
	"testing"
)

// Hermetic snippets lock operator docs for regional openai_compat providers
// (docs/providers/*.md + gateway.example.yaml). No network.

func TestXAISnippetCapabilitiesAndAliasParse(t *testing.T) {
	t.Parallel()
	yaml := `
providers:
  xai:
    kind: openai_compat
    base_url: "https://api.x.ai/v1"
    api_key_env: XAI_API_KEY
    capabilities:
      text: true
      image_gen: true
defaults:
  openai_dialect: xai
aliases:
  grok: xai/grok-3
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	p, ok := cfg.Providers["xai"]
	if !ok || p.Kind != KindOpenAICompat {
		t.Fatalf("xai: %+v ok=%v", p, ok)
	}
	if p.BaseURL != "https://api.x.ai/v1" {
		t.Fatalf("base_url=%q", p.BaseURL)
	}
	if p.Capabilities == nil || !p.Capabilities.ImageGen {
		t.Fatalf("image_gen should be true for Imagine docs sample: %+v", p.Capabilities)
	}
	if !p.Supports(ModalityImageGen) {
		t.Fatal("Supports(xai, image_gen) = false")
	}
	if cfg.Aliases["grok"] != "xai/grok-3" {
		t.Fatalf("alias grok=%q", cfg.Aliases["grok"])
	}
}

func TestQwenRegionalSnippetsAndAliasesParse(t *testing.T) {
	t.Parallel()
	yaml := `
providers:
  qwen:
    kind: openai_compat
    base_url: "https://dashscope.aliyuncs.com/compatible-mode/v1"
    api_key_env: DASHSCOPE_API_KEY
  qwen_intl:
    kind: openai_compat
    base_url: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
    api_key_env: DASHSCOPE_API_KEY
defaults:
  openai_dialect: qwen
aliases:
  qwen-turbo: qwen/qwen-turbo
  qwen-plus: qwen/qwen-plus
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	cn, ok := cfg.Providers["qwen"]
	if !ok || cn.Kind != KindOpenAICompat {
		t.Fatalf("qwen provider: %+v ok=%v", cn, ok)
	}
	if !strings.Contains(cn.BaseURL, "compatible-mode") {
		t.Fatalf("CN base must include compatible-mode: %q", cn.BaseURL)
	}
	intl, ok := cfg.Providers["qwen_intl"]
	if !ok || !strings.Contains(intl.BaseURL, "dashscope-intl") {
		t.Fatalf("qwen_intl base=%q ok=%v", intl.BaseURL, ok)
	}
	if cfg.Aliases["qwen-turbo"] != "qwen/qwen-turbo" {
		t.Fatalf("alias qwen-turbo=%q", cfg.Aliases["qwen-turbo"])
	}
	if cfg.Aliases["qwen-plus"] != "qwen/qwen-plus" {
		t.Fatalf("alias qwen-plus=%q", cfg.Aliases["qwen-plus"])
	}
}

func TestZAIRegionalSnippetsParse(t *testing.T) {
	t.Parallel()
	intl := `
providers:
  zai:
    kind: openai_compat
    base_url: "https://api.z.ai/api/paas/v4"
    api_key_env: ZAI_API_KEY
defaults:
  openai_dialect: zai
`
	cn := `
providers:
  zai_cn:
    kind: openai_compat
    base_url: "https://open.bigmodel.cn/api/paas/v4"
    api_key_env: ZAI_API_KEY
defaults:
  openai_dialect: zai_cn
`
	for name, yaml := range map[string]string{"intl": intl, "cn": cn} {
		t.Run(name, func(t *testing.T) {
			cfg, err := Parse([]byte(yaml))
			if err != nil {
				t.Fatal(err)
			}
			var p Provider
			var ok bool
			if name == "intl" {
				p, ok = cfg.Providers["zai"]
			} else {
				p, ok = cfg.Providers["zai_cn"]
			}
			if !ok {
				t.Fatal("provider missing")
			}
			if p.Kind != KindOpenAICompat {
				t.Fatalf("kind=%q want openai_compat", p.Kind)
			}
			if !strings.Contains(p.BaseURL, "z.ai") && !strings.Contains(p.BaseURL, "bigmodel.cn") {
				t.Fatalf("unexpected base_url %q", p.BaseURL)
			}
			if p.APIKeyEnv != "ZAI_API_KEY" {
				t.Fatalf("api_key_env=%q", p.APIKeyEnv)
			}
		})
	}
}
