package openai

import (
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestValidationErrorString(t *testing.T) {
	err := &ValidationError{Msg: "boom"}
	if err.Error() != "boom" {
		t.Fatal(err.Error())
	}
}

func TestParseImageURLEdgeCases(t *testing.T) {
	// no comma after data:
	img := parseImageURL("data:image/png;base64")
	if img.Kind != "url" {
		t.Fatalf("%+v", img)
	}
	// non-base64 encoding
	img = parseImageURL("data:text/plain;charset=utf-8,hello")
	if img.Kind != "url" {
		t.Fatalf("%+v", img)
	}
	// proper base64
	img = parseImageURL("data:image/jpeg;base64,/9j/")
	if img.Kind != "base64" || img.MediaType != "image/jpeg" || img.Data != "/9j/" {
		t.Fatalf("%+v", img)
	}
}

func TestParseBadStop(t *testing.T) {
	_, err := ParseRequest([]byte(`{"model":"m","stop":{"x":1},"messages":[]}`))
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("%v", err)
	}
}

func TestParseUnknownRole(t *testing.T) {
	_, err := ParseRequest([]byte(`{"model":"m","messages":[{"role":"function","content":"x"}]}`))
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("%v", err)
	}
}

func TestParseToolChoiceUnknown(t *testing.T) {
	_, err := ParseRequest([]byte(`{"model":"m","tool_choice":"maybe","messages":[]}`))
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("%v", err)
	}
	_, err = ParseRequest([]byte(`{"model":"m","tool_choice":{"type":"other"},"messages":[]}`))
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("%v", err)
	}
}

func TestParseNonFunctionToolsRejected(t *testing.T) {
	// Policy: non-function tool types are bad_request (not silently skipped).
	_, err := ParseRequest([]byte(`{"model":"m","tools":[
		{"type":"custom","function":{"name":"x"}},
		{"type":"function","function":{"name":"keep"}}
	],"messages":[]}`))
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("want ValidationError for non-function tool, got %v", err)
	}
}

func TestParseFunctionToolsOK(t *testing.T) {
	req, err := ParseRequest([]byte(`{"model":"m","tools":[
		{"type":"function","function":{"name":"keep"}},
		{"function":{"name":"also"}}
	],"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Tools) != 2 {
		t.Fatalf("%+v", req.Tools)
	}
}

func TestParseNPolicy(t *testing.T) {
	// n omitted / n=1 OK
	req, err := ParseRequest([]byte(`{"model":"m","n":1,"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.N != 1 {
		t.Fatalf("n: %d", req.N)
	}
	_, err = ParseRequest([]byte(`{"model":"m","n":2,"messages":[]}`))
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("want ValidationError for n>1, got %v", err)
	}
}

func TestParseServiceTier(t *testing.T) {
	req, err := ParseRequest([]byte(`{"model":"m","service_tier":"auto","messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.ServiceTier != "auto" {
		t.Fatalf("service_tier: %q", req.ServiceTier)
	}
}

func TestParseToolMissingName(t *testing.T) {
	_, err := ParseRequest([]byte(`{"model":"m","tools":[{"type":"function","function":{}}],"messages":[]}`))
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("%v", err)
	}
}

func TestParseDeveloperRole(t *testing.T) {
	req, err := ParseRequest([]byte(`{"model":"m","messages":[{"role":"developer","content":"rules"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(req.System) != 1 || req.System[0].Text != "rules" {
		t.Fatalf("%+v", req.System)
	}
}

func TestParseEmptyAssistantToolArgs(t *testing.T) {
	req, err := ParseRequest([]byte(`{"model":"m","messages":[
		{"role":"assistant","tool_calls":[{"id":"c","type":"function","function":{"name":"f","arguments":""}}]}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(req.Messages[0].Content[0].Input) != "{}" {
		t.Fatalf("%s", req.Messages[0].Content[0].Input)
	}
}

func TestParseNullContentParts(t *testing.T) {
	req, err := ParseRequest([]byte(`{"model":"m","messages":[{"role":"user","content":null}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Messages) != 1 || len(req.Messages[0].Content) != 0 {
		t.Fatalf("%+v", req.Messages)
	}
}

func TestParseImageURLNil(t *testing.T) {
	req, err := ParseRequest([]byte(`{"model":"m","messages":[{"role":"user","content":[
		{"type":"image_url"}
	]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	// nil image_url skipped
	if len(req.Messages[0].Content) != 0 {
		t.Fatalf("%+v", req.Messages[0].Content)
	}
}

func TestSerializeEmptyID(t *testing.T) {
	body, err := SerializeResponse(&canonical.Response{
		Model:      "m",
		Content:    []canonical.Block{{Type: canonical.BlockText, Text: "x"}},
		StopReason: "",
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(body), "chatcmpl_") && !contains(string(body), "chat.completion") {
		t.Fatalf("%s", body)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && (func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})()))
}
