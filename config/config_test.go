package config

import "testing"

func TestParseValid(t *testing.T) {
	cfg, err := Parse([]byte(`
listen: ":9000"
providers:
  anthropic: { kind: anthropic, base_url: "https://api.anthropic.com/" }
defaults:
  anthropic_dialect: anthropic
aliases:
  best: anthropic/claude-sonnet-5
hooks:
  jsonl: { output: stdout }
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":9000" {
		t.Errorf("listen = %q", cfg.Listen)
	}
	if cfg.Providers["anthropic"].BaseURL != "https://api.anthropic.com" {
		t.Errorf("trailing slash not trimmed: %q", cfg.Providers["anthropic"].BaseURL)
	}
}

func TestParseRejects(t *testing.T) {
	cases := map[string]string{
		"no providers":             `listen: ":1"`,
		"bad kind":                 "providers:\n  x: { kind: nope, base_url: \"https://x\" }",
		"missing base_url":         "providers:\n  x: { kind: openai }",
		"bad alias":                "providers:\n  x: { kind: openai, base_url: \"https://x\" }\naliases:\n  a: noslash",
		"alias unknown provider":   "providers:\n  x: { kind: openai, base_url: \"https://x\" }\naliases:\n  a: other/m",
		"default unknown provider": "providers:\n  x: { kind: openai, base_url: \"https://x\" }\ndefaults:\n  openai_dialect: other",
		"unknown field":            "providers:\n  x: { kind: openai, base_url: \"https://x\", nope: 1 }",
		"webhook no url":           "providers:\n  x: { kind: openai, base_url: \"https://x\" }\nhooks:\n  webhook: { timeout: 1s }",
	}
	for name, yaml := range cases {
		if _, err := Parse([]byte(yaml)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestDefaultListen(t *testing.T) {
	cfg, err := Parse([]byte("providers:\n  x: { kind: openai, base_url: \"https://x\" }"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":8787" {
		t.Errorf("default listen = %q", cfg.Listen)
	}
}
