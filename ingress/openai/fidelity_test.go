package openai

import (
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
)

func TestParseResponseFormatJSONObject(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"m","response_format":{"type":"json_object"},
		"messages":[]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.ResponseFormat == nil || req.ResponseFormat.Kind != canonical.ResponseFormatJSONObject {
		t.Fatalf("%+v", req.ResponseFormat)
	}
}

func TestParseResponseFormatJSONSchema(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"m",
		"response_format":{
			"type":"json_schema",
			"json_schema":{
				"name":"person",
				"description":"a person",
				"schema":{"type":"object","properties":{"name":{"type":"string"}}},
				"strict":true
			}
		},
		"messages":[]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	rf := req.ResponseFormat
	if rf == nil || rf.Kind != canonical.ResponseFormatJSONSchema {
		t.Fatalf("%+v", rf)
	}
	if rf.Name != "person" || rf.Description != "a person" {
		t.Fatalf("name/desc: %+v", rf)
	}
	if rf.Strict == nil || !*rf.Strict {
		t.Fatal("strict")
	}
	if !strings.Contains(string(rf.Schema), `"name"`) {
		t.Fatalf("schema: %s", rf.Schema)
	}
}

func TestParseResponseFormatUnknown(t *testing.T) {
	_, err := ParseRequest([]byte(`{"model":"m","response_format":{"type":"xml"},"messages":[]}`))
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("%v", err)
	}
}

func TestParseReasoningEffort(t *testing.T) {
	req, err := ParseRequest([]byte(`{"model":"m","reasoning_effort":"high","messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Thinking == nil || req.Thinking.Effort != "high" || req.Thinking.Type != "enabled" {
		t.Fatalf("%+v", req.Thinking)
	}
}

func TestParsePenaltiesAndSeed(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"m","frequency_penalty":0.3,"presence_penalty":0.1,"seed":99,"messages":[]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.FrequencyPenalty == nil || *req.FrequencyPenalty != 0.3 {
		t.Fatalf("freq %+v", req.FrequencyPenalty)
	}
	if req.PresencePenalty == nil || *req.PresencePenalty != 0.1 {
		t.Fatalf("pres %+v", req.PresencePenalty)
	}
	if req.Seed == nil || *req.Seed != 99 {
		t.Fatalf("seed %+v", req.Seed)
	}
}

func TestParseParallelToolCalls(t *testing.T) {
	req, err := ParseRequest([]byte(`{"model":"m","parallel_tool_calls":false,"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.ParallelToolCalls == nil || *req.ParallelToolCalls {
		t.Fatalf("%+v", req.ParallelToolCalls)
	}
	req2, err := ParseRequest([]byte(`{"model":"m","messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if req2.ParallelToolCalls != nil {
		t.Fatal("unset must be nil")
	}
}

func TestParseMaxTokensSource(t *testing.T) {
	req, err := ParseRequest([]byte(`{"model":"m","max_tokens":10,"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.MaxTokens != 10 || req.MaxTokensField != canonical.MaxTokensFieldMaxTokens {
		t.Fatalf("%d %q", req.MaxTokens, req.MaxTokensField)
	}
	req, err = ParseRequest([]byte(`{"model":"m","max_completion_tokens":42,"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.MaxTokens != 42 || req.MaxTokensField != canonical.MaxTokensFieldMaxCompletionTokens {
		t.Fatalf("%d %q", req.MaxTokens, req.MaxTokensField)
	}
	// both set: max_tokens wins (documented precedence)
	req, err = ParseRequest([]byte(`{"model":"m","max_tokens":1,"max_completion_tokens":99,"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.MaxTokens != 1 || req.MaxTokensField != canonical.MaxTokensFieldMaxTokens {
		t.Fatalf("precedence: %d %q", req.MaxTokens, req.MaxTokensField)
	}
}

func TestParseImageDetail(t *testing.T) {
	req, err := ParseRequest([]byte(`{"model":"m","messages":[{"role":"user","content":[
		{"type":"image_url","image_url":{"url":"https://ex.com/x.png","detail":"high"}}
	]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	img := req.Messages[0].Content[0].Image
	if img == nil || img.Detail != "high" || img.Kind != "url" {
		t.Fatalf("%+v", img)
	}
}

func TestParseInputAudio(t *testing.T) {
	req, err := ParseRequest([]byte(`{"model":"m","messages":[{"role":"user","content":[
		{"type":"text","text":"listen"},
		{"type":"input_audio","input_audio":{"data":"QUJD","format":"wav"}}
	]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	blocks := req.Messages[0].Content
	if len(blocks) != 2 {
		t.Fatalf("%d", len(blocks))
	}
	a := blocks[1]
	if a.Type != canonical.BlockAudio || a.Audio == nil || a.Audio.Data != "QUJD" || a.Audio.MediaType != "audio/wav" && a.Audio.MediaType != "wav" {
		t.Fatalf("%+v", a)
	}
	if a.Audio.MediaType != "audio/wav" {
		t.Fatalf("media %q", a.Audio.MediaType)
	}
}

func TestParseInputAudioIncomplete(t *testing.T) {
	_, err := ParseRequest([]byte(`{"model":"m","messages":[{"role":"user","content":[
		{"type":"input_audio","input_audio":{"data":"x"}}
	]}]}`))
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("%v", err)
	}
	_, err = ParseRequest([]byte(`{"model":"m","messages":[{"role":"user","content":[
		{"type":"input_audio"}
	]}]}`))
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("%v", err)
	}
}

func TestParseFileParts(t *testing.T) {
	req, err := ParseRequest([]byte(`{"model":"m","messages":[{"role":"user","content":[
		{"type":"file","file":{"file_id":"file-abc","filename":"a.pdf"}},
		{"type":"file","file":{"filename":"b.pdf","file_data":"data:application/pdf;base64,QUJD"}}
	]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	blocks := req.Messages[0].Content
	if len(blocks) != 2 {
		t.Fatalf("%d", len(blocks))
	}
	if blocks[0].Type != canonical.BlockDocument || blocks[0].Document.Kind != "file_id" ||
		blocks[0].Document.Data != "file-abc" {
		t.Fatalf("%+v", blocks[0].Document)
	}
	if blocks[1].Document.Kind != "base64" || blocks[1].Document.Data != "QUJD" ||
		blocks[1].Document.MediaType != "application/pdf" {
		t.Fatalf("%+v", blocks[1].Document)
	}
}

func TestParseFileIncomplete(t *testing.T) {
	_, err := ParseRequest([]byte(`{"model":"m","messages":[{"role":"user","content":[
		{"type":"file","file":{}}
	]}]}`))
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("%v", err)
	}
}

func TestParseAssistantReasoningContent(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"model":"m","messages":[
			{"role":"assistant","content":"hi","reasoning_content":"chain of thought",
			 "tool_calls":[{"id":"c1","type":"function","function":{"name":"f","arguments":"{}"}}]}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	blocks := req.Messages[0].Content
	if len(blocks) < 2 {
		t.Fatalf("%+v", blocks)
	}
	if blocks[0].Type != canonical.BlockThinking || blocks[0].Text != "chain of thought" {
		t.Fatalf("thinking: %+v", blocks[0])
	}
	// text then tool_use
	var sawText, sawTool bool
	for _, b := range blocks {
		if b.Type == canonical.BlockText && b.Text == "hi" {
			sawText = true
		}
		if b.Type == canonical.BlockToolUse && b.ID == "c1" {
			sawTool = true
		}
	}
	if !sawText || !sawTool {
		t.Fatalf("%+v", blocks)
	}
}

func TestSerializeOmitsRedactedThinking(t *testing.T) {
	body, err := SerializeResponse(&canonical.Response{
		ID:    "id1",
		Model: "m",
		Content: []canonical.Block{
			{Type: canonical.BlockThinking, Text: "secret", Redacted: true},
			{Type: canonical.BlockThinking, Text: "visible", Redacted: false},
			{Type: canonical.BlockText, Text: "answer"},
		},
		StopReason: canonical.StopEndTurn,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if strings.Contains(s, "secret") {
		t.Fatalf("redacted must be omitted: %s", s)
	}
	if !strings.Contains(s, "visible") || !strings.Contains(s, "reasoning_content") {
		t.Fatalf("non-redacted thinking must emit: %s", s)
	}
}
