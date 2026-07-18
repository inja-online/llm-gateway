package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPassthroughPreservesReasoningContentToolLoop (#53): openai_compat
// passthrough must keep assistant reasoning_content after model rewrite across
// a multi-turn tool loop (≥2 assistant turns with reasoning + tool_calls).
func TestPassthroughPreservesReasoningContentToolLoop(t *testing.T) {
	var gotBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"cmpl-1","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	gw, _ := newTestGateway(t, upstream)

	// ≥2-iteration tool loop with reasoning_content on each assistant turn.
	reqBody := `{
		"model": "up/deepseek-reasoner",
		"messages": [
			{"role": "user", "content": "calc"},
			{"role": "assistant", "content": null,
			 "reasoning_content": "think-1",
			 "tool_calls": [{"id":"c1","type":"function","function":{"name":"add","arguments":"{\"a\":1}"}}]},
			{"role": "tool", "tool_call_id": "c1", "content": "1"},
			{"role": "assistant", "content": null,
			 "reasoning_content": "think-2",
			 "tool_calls": [{"id":"c2","type":"function","function":{"name":"add","arguments":"{\"a\":2}"}}]},
			{"role": "tool", "tool_call_id": "c2", "content": "2"},
			{"role": "user", "content": "done"}
		]
	}`
	req, _ := http.NewRequest("POST", gw.URL+"/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer sk-test")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, b)
	}

	var forwarded map[string]any
	if err := json.Unmarshal(gotBody, &forwarded); err != nil {
		t.Fatal(err)
	}
	if forwarded["model"] != "deepseek-reasoner" {
		t.Fatalf("model rewrite: %v", forwarded["model"])
	}
	msgs, _ := forwarded["messages"].([]any)
	var reasoningCount int
	for _, m := range msgs {
		mm, _ := m.(map[string]any)
		if mm["role"] != "assistant" {
			continue
		}
		rc, ok := mm["reasoning_content"]
		if !ok || rc == nil {
			t.Fatalf("assistant message missing reasoning_content: %+v", mm)
		}
		if s, _ := rc.(string); s != "think-1" && s != "think-2" {
			t.Fatalf("reasoning_content value: %v", rc)
		}
		reasoningCount++
	}
	if reasoningCount < 2 {
		t.Fatalf("want ≥2 reasoning_content fields, got %d body=%s", reasoningCount, gotBody)
	}
}

// TestTranslateToOpenAICompatKeepsReasoningContent: OpenAI ingress → openai
// egress (simulate via parse+build already covered; here proxy translate path
// when default routes to openai_compat is passthrough). Cross-family translate
// is covered by ingress/openai roundtrip_test. This test locks that model
// rewrite alone does not strip reasoning_content.
func TestTranslateModelRewriteKeepsNestedReasoning(t *testing.T) {
	var gotBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"x","choices":[{"message":{"role":"assistant","content":"y"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	gw, _ := newTestGateway(t, upstream)

	body := `{"model":"up/kimi-k2","messages":[
		{"role":"assistant","content":"a","reasoning_content":"nested-reason","tool_calls":[
			{"id":"t1","type":"function","function":{"name":"f","arguments":"{}"}}
		]},
		{"role":"tool","tool_call_id":"t1","content":"r"}
	]}`
	req, _ := http.NewRequest("POST", gw.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !strings.Contains(string(gotBody), `"reasoning_content":"nested-reason"`) &&
		!strings.Contains(string(gotBody), `"reasoning_content": "nested-reason"`) {
		// map marshal may compact without space
		if !strings.Contains(string(gotBody), "nested-reason") {
			t.Fatalf("reasoning_content stripped: %s", gotBody)
		}
	}
}
