package google

import (
	"encoding/json"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestEffortToBudgetAllLevels(t *testing.T) {
	cases := []struct {
		effort string
		want   int
		nilOut bool
	}{
		{"minimal", 1024, false},
		{"LOW", 1024, false},
		{"  medium ", 8192, false},
		{"high", 16384, false},
		{"xhigh", 16384, false},
		{"max", 16384, false},
		{"", 0, true},
		{"unknown", 0, true},
		{"  ", 0, true},
	}
	for _, tc := range cases {
		got := effortToBudget(tc.effort)
		if tc.nilOut {
			if got != nil {
				t.Fatalf("effort %q: want nil, got %v", tc.effort, *got)
			}
			continue
		}
		if got == nil || *got != tc.want {
			t.Fatalf("effort %q: got %v want %d", tc.effort, got, tc.want)
		}
	}
}

func TestBuildThinkingConfigBranches(t *testing.T) {
	if buildThinkingConfig(nil) != nil {
		t.Fatal("nil")
	}
	if buildThinkingConfig(&canonical.ThinkingConfig{}) != nil {
		t.Fatal("empty should omit")
	}
	// disabled → budget 0
	tc := buildThinkingConfig(&canonical.ThinkingConfig{Type: "disabled"})
	if tc == nil || tc.ThinkingBudget == nil || *tc.ThinkingBudget != 0 {
		t.Fatalf("disabled: %+v", tc)
	}
	// include only
	yes := true
	tc = buildThinkingConfig(&canonical.ThinkingConfig{IncludeThoughts: &yes})
	if tc == nil || tc.IncludeThoughts == nil || !*tc.IncludeThoughts || tc.ThinkingBudget != nil {
		t.Fatalf("include-only: %+v", tc)
	}
	// effort only (no type)
	tc = buildThinkingConfig(&canonical.ThinkingConfig{Effort: "low"})
	if tc == nil || tc.ThinkingBudget == nil || *tc.ThinkingBudget != 1024 {
		t.Fatalf("effort-only: %+v", tc)
	}
	// budget wins over effort
	b := 99
	tc = buildThinkingConfig(&canonical.ThinkingConfig{Type: "enabled", BudgetTokens: &b, Effort: "high"})
	if tc == nil || tc.ThinkingBudget == nil || *tc.ThinkingBudget != 99 {
		t.Fatalf("budget wins: %+v", tc)
	}
	// unknown type with no budget/include → omit
	if buildThinkingConfig(&canonical.ThinkingConfig{Type: "weird"}) != nil {
		t.Fatal("weird type with nothing concrete should omit")
	}
	// unknown effort alone → omit
	if buildThinkingConfig(&canonical.ThinkingConfig{Effort: "nope"}) != nil {
		t.Fatal("unknown effort should omit")
	}
}

func TestApplyResponseFormatBranches(t *testing.T) {
	// nils
	applyResponseFormat(nil, nil)
	gc := &generationConfig{}
	applyResponseFormat(gc, nil)
	if gc.ResponseMIMEType != "" {
		t.Fatal("nil rf")
	}
	applyResponseFormat(gc, &canonical.ResponseFormat{Kind: canonical.ResponseFormatText})
	if gc.ResponseMIMEType != "text/plain" {
		t.Fatalf("text: %q", gc.ResponseMIMEType)
	}
	applyResponseFormat(gc, &canonical.ResponseFormat{Kind: canonical.ResponseFormatJSONObject})
	if gc.ResponseMIMEType != "application/json" || len(gc.ResponseSchema) > 0 {
		t.Fatalf("json_object: %+v", gc)
	}
	// json_schema without schema body
	gc = &generationConfig{}
	applyResponseFormat(gc, &canonical.ResponseFormat{Kind: canonical.ResponseFormatJSONSchema})
	if gc.ResponseMIMEType != "application/json" || len(gc.ResponseSchema) > 0 {
		t.Fatalf("json_schema empty: %+v", gc)
	}
	// json_schema with body
	schema := json.RawMessage(`{"type":"object"}`)
	applyResponseFormat(gc, &canonical.ResponseFormat{Kind: canonical.ResponseFormatJSONSchema, Schema: schema})
	if string(gc.ResponseSchema) != string(schema) {
		t.Fatalf("schema %s", gc.ResponseSchema)
	}
	// unknown kind: no change beyond prior
	gc2 := &generationConfig{}
	applyResponseFormat(gc2, &canonical.ResponseFormat{Kind: "xml"})
	if gc2.ResponseMIMEType != "" {
		t.Fatalf("unknown: %+v", gc2)
	}
}

func TestBuildGenerationConfigNilAndEmpty(t *testing.T) {
	if buildGenerationConfig(nil) != nil {
		t.Fatal("nil req")
	}
	if buildGenerationConfig(&canonical.Request{}) != nil {
		t.Fatal("empty req")
	}
	// only temperature
	f := 0.5
	gc := buildGenerationConfig(&canonical.Request{Temperature: &f})
	if gc == nil || gc.Temperature == nil || *gc.Temperature != 0.5 {
		t.Fatalf("%+v", gc)
	}
}

func TestGuessMIMEFromURIAllExts(t *testing.T) {
	cases := map[string]string{
		"https://x/a.PNG":          "image/png",
		"https://x/a.jpg?q=1":      "image/jpeg",
		"https://x/a.jpeg#frag":    "image/jpeg",
		"https://x/a.gif":          "image/gif",
		"https://x/a.webp":         "image/webp",
		"https://x/a.pdf":          "application/pdf",
		"https://x/a.mp3":          "audio/mpeg",
		"https://x/a.wav":          "audio/wav",
		"https://x/a.mp4":          "video/mp4",
		"https://x/a.bin":          "",
		"https://x/noext":          "",
		"https://x/a.unknown?x=1":  "",
	}
	for uri, want := range cases {
		if got := guessMIMEFromURI(uri); got != want {
			t.Fatalf("%s: got %q want %q", uri, got, want)
		}
	}
}

func TestBuildDocumentPartEdges(t *testing.T) {
	if _, ok := buildDocumentPart(nil); ok {
		t.Fatal("nil")
	}
	if _, ok := buildDocumentPart(&canonical.DocumentSource{}); ok {
		t.Fatal("empty data")
	}
	// default media type PDF
	p, ok := buildDocumentPart(&canonical.DocumentSource{Kind: "base64", Data: "QQ=="})
	if !ok || p.InlineData == nil || p.InlineData.MIMEType != "application/pdf" {
		t.Fatalf("%+v ok=%v", p, ok)
	}
	// url without mime → guess
	p, ok = buildDocumentPart(&canonical.DocumentSource{
		Kind: "url", Data: "https://cdn.example/doc.pdf",
	})
	if !ok || p.FileData == nil || p.FileData.MIMEType != "application/pdf" {
		t.Fatalf("%+v", p)
	}
}

func TestParseDataURLEdges(t *testing.T) {
	// not data
	if _, _, ok := parseDataURL("https://x"); ok {
		t.Fatal("https")
	}
	// no comma
	if _, _, ok := parseDataURL("data:image/png;base64"); ok {
		t.Fatal("no comma")
	}
	// non-base64 with mime
	if _, _, ok := parseDataURL("data:text/plain,hello"); ok {
		t.Fatal("non-base64")
	}
	// non-base64 with charset
	if _, _, ok := parseDataURL("data:text/plain;charset=utf-8,hello"); ok {
		t.Fatal("charset")
	}
	// valid base64
	mt, data, ok := parseDataURL("data:image/png;base64,abc")
	if !ok || mt != "image/png" || data != "abc" {
		t.Fatalf("%q %q %v", mt, data, ok)
	}
	// empty media type with base64
	mt, data, ok = parseDataURL("data:;base64,xyz")
	if !ok || data != "xyz" {
		t.Fatalf("%q %q %v", mt, data, ok)
	}
}

func TestBuildMediaPartDefaultMIME(t *testing.T) {
	// base64 without media type
	p, ok := buildMediaPart("base64", "", "ZZ")
	if !ok || p.InlineData == nil || p.InlineData.MIMEType != "application/octet-stream" {
		t.Fatalf("%+v", p)
	}
	// data URL with empty mt falls back to param
	p, ok = buildMediaPart("url", "image/gif", "data:;base64,xx")
	if !ok || p.InlineData == nil || p.InlineData.MIMEType != "image/gif" {
		t.Fatalf("%+v", p)
	}
	// data URL empty mt and empty param → octet-stream
	p, ok = buildMediaPart("url", "", "data:;base64,yy")
	if !ok || p.InlineData == nil || p.InlineData.MIMEType != "application/octet-stream" {
		t.Fatalf("%+v", p)
	}
}

func TestBuildRequestWithSeedTopKFormat(t *testing.T) {
	seed := int64(123)
	topK := 8
	body, err := BuildRequest(&canonical.Request{
		Seed: &seed,
		TopK: &topK,
		ResponseFormat: &canonical.ResponseFormat{Kind: canonical.ResponseFormatText},
		Thinking:       &canonical.ThinkingConfig{Effort: "minimal"},
		Messages: []canonical.Message{{
			Role:    canonical.RoleUser,
			Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		}},
	}, "m")
	if err != nil {
		t.Fatal(err)
	}
	var out generateRequest
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	gc := out.GenerationConfig
	if gc == nil || gc.Seed == nil || *gc.Seed != 123 || gc.TopK == nil || *gc.TopK != 8 {
		t.Fatalf("%+v", gc)
	}
	if gc.ResponseMIMEType != "text/plain" {
		t.Fatalf("mime %q", gc.ResponseMIMEType)
	}
	if gc.ThinkingConfig == nil || gc.ThinkingConfig.ThinkingBudget == nil || *gc.ThinkingConfig.ThinkingBudget != 1024 {
		t.Fatalf("thinking %+v", gc.ThinkingConfig)
	}
}
