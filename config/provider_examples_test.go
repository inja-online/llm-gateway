package config

import (
	"strings"
	"testing"
)

// Hermetic snippets lock operator docs for regional openai_compat providers
// (docs/providers/*.md + gateway.example.yaml). No network.

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
