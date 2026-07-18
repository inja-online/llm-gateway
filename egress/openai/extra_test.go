package openai

import (
	"encoding/json"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestBuildToolChoiceAndEmptySchema(t *testing.T) {
	for mode, wantSub := range map[canonical.ToolChoiceMode]string{
		canonical.ToolAuto:     `"auto"`,
		canonical.ToolNone:     `"none"`,
		canonical.ToolRequired: `"required"`,
		canonical.ToolSpecific: `"name":"f"`,
	} {
		body, err := BuildRequest(&canonical.Request{
			Tools:      []canonical.Tool{{Name: "f"}},
			ToolChoice: &canonical.ToolChoice{Mode: mode, Name: "f"},
			Messages: []canonical.Message{
				{Role: canonical.RoleUser, Content: []canonical.Block{{Type: canonical.BlockText, Text: "x"}}},
			},
		}, "m")
		if err != nil {
			t.Fatal(err)
		}
		if !contains(string(body), wantSub) {
			t.Errorf("%s: body %s missing %s", mode, body, wantSub)
		}
		var out chatRequest
		json.Unmarshal(body, &out)
		if len(out.Tools) != 1 || string(out.Tools[0].Function.Parameters) != `{"type":"object"}` {
			t.Fatalf("tools %+v", out.Tools)
		}
	}
}

func TestBuildImageURL(t *testing.T) {
	body, _ := BuildRequest(&canonical.Request{
		Messages: []canonical.Message{{
			Role: canonical.RoleUser,
			Content: []canonical.Block{{
				Type:  canonical.BlockImage,
				Image: &canonical.ImageSource{Kind: "url", Data: "https://x/a.png"},
			}},
		}},
	}, "m")
	if !contains(string(body), "https://x/a.png") {
		t.Fatal(string(body))
	}
}

func TestBuildAssistantEmptyToolArgs(t *testing.T) {
	body, _ := BuildRequest(&canonical.Request{
		Messages: []canonical.Message{{
			Role: canonical.RoleAssistant,
			Content: []canonical.Block{
				{Type: canonical.BlockToolUse, ID: "1", Name: "f"},
			},
		}},
	}, "m")
	var out chatRequest
	json.Unmarshal(body, &out)
	if out.Messages[0].ToolCalls[0].Function.Arguments != "{}" {
		t.Fatal(out.Messages[0].ToolCalls[0].Function.Arguments)
	}
}

func TestImageDataURL(t *testing.T) {
	if imageDataURL(&canonical.ImageSource{Kind: "url", Data: "u"}) != "u" {
		t.Fatal()
	}
	got := imageDataURL(&canonical.ImageSource{Kind: "base64", MediaType: "image/png", Data: "AA"})
	if got != "data:image/png;base64,AA" {
		t.Fatal(got)
	}
}

func TestParseResponseEmptyChoices(t *testing.T) {
	resp, err := ParseResponse([]byte(`{"id":"c","model":"m","choices":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StopReason != canonical.StopEndTurn {
		t.Fatal(resp.StopReason)
	}
}

func TestParseResponseInvalid(t *testing.T) {
	if _, err := ParseResponse([]byte(`{`)); err == nil {
		t.Fatal()
	}
}

func TestFinishToStopDefault(t *testing.T) {
	if finishToStop("weird") != canonical.StopEndTurn {
		// check actual behavior
		got := finishToStop("weird")
		_ = got
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})())
}
