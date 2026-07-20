package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/config"
)

func TestConversationsStubNotImplemented(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  openai: { kind: openai, base_url: "http://127.0.0.1:9" }
defaults:
  openai_dialect: openai
`))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(NewServer(cfg, nil).Handler())
	t.Cleanup(gw.Close)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v1/conversations"},
		{http.MethodGet, "/v1/conversations"},
		{http.MethodGet, "/v1/conversations/conv_123"},
		{http.MethodPost, "/v1/conversations/conv_123"},
		{http.MethodDelete, "/v1/conversations/conv_123"},
		{http.MethodGet, "/v1/conversations/conv_123/items"},
		{http.MethodPost, "/v1/conversations/conv_123/items"},
		{http.MethodDelete, "/v1/conversations/conv_123/items/item_1"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, gw.URL+tc.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusNotImplemented {
				t.Fatalf("status %d body %s", resp.StatusCode, body)
			}
			var envelope struct {
				Error struct {
					Message string `json:"message"`
					Type    string `json:"type"`
					Code    string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(body, &envelope); err != nil {
				t.Fatalf("json: %v body %s", err, body)
			}
			if envelope.Error.Type != "not_implemented" {
				t.Fatalf("type = %q want not_implemented", envelope.Error.Type)
			}
			// Code may mirror type depending on writeOpenAIError; accept either.
			msg := envelope.Error.Message
			if !strings.Contains(msg, "Conversations") {
				t.Fatalf("message missing Conversations: %q", msg)
			}
			if !strings.Contains(msg, "/v1/responses") {
				t.Fatalf("message should point to Responses: %q", msg)
			}
			if !strings.Contains(strings.ToLower(msg), "files") {
				t.Fatalf("message should mention Files alternative: %q", msg)
			}
			if !strings.Contains(strings.ToLower(msg), "stateless") {
				t.Fatalf("message should mention stateless: %q", msg)
			}
		})
	}
}

func TestConversationsMessageStableGuidance(t *testing.T) {
	// Lock migration keywords so clients/string-matchers keep working (#67).
	for _, needle := range []string{
		"not implemented",
		"/v1/responses",
		"Files",
		"stateless",
	} {
		if !strings.Contains(conversationsNotImplementedMsg, needle) &&
			!strings.Contains(strings.ToLower(conversationsNotImplementedMsg), strings.ToLower(needle)) {
			t.Fatalf("conversationsNotImplementedMsg missing %q: %s", needle, conversationsNotImplementedMsg)
		}
	}
}
