package config

import "testing"

func TestDefaultCapabilitiesByKind(t *testing.T) {
	cases := []struct {
		kind     string
		wantText bool
		wantImg  bool
		wantRT   bool
	}{
		{KindOpenAI, true, true, true},
		{KindGoogle, true, true, true},
		{KindAnthropic, true, false, false},
		{KindOpenAICompat, true, false, false},
		{"unknown", false, false, false},
	}
	for _, tc := range cases {
		c := DefaultCapabilities(tc.kind)
		if c.Text != tc.wantText || c.ImageGen != tc.wantImg || c.Realtime != tc.wantRT {
			t.Errorf("kind %s: got text=%v img=%v rt=%v want text=%v img=%v rt=%v",
				tc.kind, c.Text, c.ImageGen, c.Realtime, tc.wantText, tc.wantImg, tc.wantRT)
		}
	}
}

func TestSupportsUsesDefaultsWhenNil(t *testing.T) {
	p := Provider{Kind: KindOpenAICompat, BaseURL: "https://x"}
	if !p.Supports(ModalityText) {
		t.Fatal("compat should support text by default")
	}
	if p.Supports(ModalityImageGen) {
		t.Fatal("compat must not support image_gen without opt-in")
	}
}

func TestSupportsExplicitOverride(t *testing.T) {
	p := Provider{
		Kind:    KindOpenAICompat,
		BaseURL: "https://x",
		Capabilities: &Capabilities{
			Text:     true,
			ImageGen: true,
			VideoGen: true,
		},
	}
	if !p.Supports(ModalityImageGen) || !p.Supports(ModalityVideoGen) {
		t.Fatal("explicit image/video should be allowed")
	}
	if p.Supports(ModalityRealtime) {
		t.Fatal("realtime still off")
	}
}

func TestParseCapabilitiesYAML(t *testing.T) {
	cfg, err := Parse([]byte(`
providers:
  google_openai:
    kind: openai_compat
    base_url: "https://example.com/v1"
    capabilities:
      text: true
      image_gen: true
      video_gen: true
      audio_speech: false
`))
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Providers["google_openai"]
	if p.Capabilities == nil {
		t.Fatal("capabilities not parsed")
	}
	if !p.Supports(ModalityImageGen) || p.Supports(ModalityAudioSpeech) {
		t.Fatalf("caps: %+v", p.EffectiveCapabilities())
	}
}

func TestParseRealtimeSection(t *testing.T) {
	cfg, err := Parse([]byte(`
providers:
  x: { kind: openai, base_url: "https://x" }
realtime:
  max_sessions: 10
  max_session_minutes: 5
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Realtime.MaxSessions != 10 || cfg.Realtime.MaxSessionMinutes != 5 {
		t.Fatalf("realtime: %+v", cfg.Realtime)
	}
}

func TestRealtimeDefaults(t *testing.T) {
	cfg, err := Parse([]byte(`providers: { x: { kind: openai, base_url: "https://x" } }`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Realtime.MaxSessions != 1024 {
		t.Fatalf("max_sessions default = %d", cfg.Realtime.MaxSessions)
	}
	if cfg.Realtime.MaxSessionMinutes != 60 {
		t.Fatalf("max_session_minutes default = %d", cfg.Realtime.MaxSessionMinutes)
	}
}

func TestUnknownModalityUnsupported(t *testing.T) {
	p := Provider{Kind: KindOpenAI, BaseURL: "https://x"}
	if p.Supports("nope") {
		t.Fatal("unknown modality must be false")
	}
}
