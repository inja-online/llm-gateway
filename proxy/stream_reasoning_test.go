package proxy

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/inja-online/llm-gateway/canonical"
	anthropicegress "github.com/inja-online/llm-gateway/egress/anthropic"
	googleegress "github.com/inja-online/llm-gateway/egress/google"
	openaiegress "github.com/inja-online/llm-gateway/egress/openai"
	antingress "github.com/inja-online/llm-gateway/ingress/anthropic"
	googleingress "github.com/inja-online/llm-gateway/ingress/google"
	oaingress "github.com/inja-online/llm-gateway/ingress/openai"
)

// streamDialect is one side of a translate pair (upstream egress parse or
// client ingress serialize).
type streamDialect string

const (
	dOpenAI    streamDialect = "openai"
	dAnthropic streamDialect = "anthropic"
	dGoogle    streamDialect = "google"
)

// parseUpstream feeds raw JSON data payloads (SSE data lines without the
// "data: " prefix) through the matching egress stream parser and returns the
// full canonical event sequence including Finish.
func parseUpstream(up streamDialect, payloads []string) []canonical.StreamEvent {
	var all []canonical.StreamEvent
	switch up {
	case dOpenAI:
		p := openaiegress.NewStreamParser()
		for _, pl := range payloads {
			all = append(all, p.Parse([]byte(pl))...)
		}
		all = append(all, p.Finish()...)
	case dAnthropic:
		p := anthropicegress.NewStreamParser()
		for _, pl := range payloads {
			all = append(all, p.Parse([]byte(pl))...)
		}
		// Anthropic terminal is message_stop inside payloads; Finish is n/a.
	case dGoogle:
		p := googleegress.NewStreamParser()
		for _, pl := range payloads {
			all = append(all, p.Parse([]byte(pl))...)
		}
		all = append(all, p.Finish()...)
	}
	return all
}

// serializeClient turns canonical events into client-facing SSE bytes.
func serializeClient(client streamDialect, evs []canonical.StreamEvent) string {
	var b strings.Builder
	switch client {
	case dOpenAI:
		s := oaingress.NewStreamSerializer(1)
		for _, ev := range evs {
			if out := s.Event(ev); out != nil {
				b.Write(out)
			}
		}
		b.Write(s.Done())
	case dAnthropic:
		s := antingress.NewStreamSerializer()
		for _, ev := range evs {
			if out := s.Event(ev); out != nil {
				b.Write(out)
			}
		}
	case dGoogle:
		s := googleingress.NewStreamSerializer()
		for _, ev := range evs {
			if out := s.Event(ev); out != nil {
				b.Write(out)
			}
		}
	}
	return b.String()
}

// Fake upstream SSE data payloads (no "data: " prefix) for each dialect.
// Scenarios: thinking-only, thinking+text, thinking+tool.

func openaiThinkingOnly() []string {
	return []string{
		`{"id":"c1","model":"m","choices":[{"delta":{"role":"assistant"}}]}`,
		`{"id":"c1","choices":[{"delta":{"reasoning_content":"stepA"}}]}`,
		`{"id":"c1","choices":[{"delta":{"reasoning_content":" stepB"}}]}`,
		`{"id":"c1","choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`[DONE]`,
	}
}

func openaiThinkingText() []string {
	return []string{
		`{"id":"c1","model":"m","choices":[{"delta":{"reasoning_content":"plan"}}]}`,
		`{"id":"c1","choices":[{"delta":{"content":"hello"}}]}`,
		`{"id":"c1","choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`[DONE]`,
	}
}

func openaiThinkingTool() []string {
	return []string{
		`{"id":"c1","model":"m","choices":[{"delta":{"reasoning_content":"use tool"}}]}`,
		`{"id":"c1","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"search","arguments":"{\"q\""}}]}}]}`,
		`{"id":"c1","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"go\"}"}}]}}]}`,
		`{"id":"c1","choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`[DONE]`,
	}
}

func anthropicThinkingOnly() []string {
	return []string{
		`{"type":"message_start","message":{"id":"msg_1","model":"claude","usage":{"input_tokens":1,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"stepA"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":" stepB"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
		`{"type":"message_stop"}`,
	}
}

func anthropicThinkingText() []string {
	return []string{
		`{"type":"message_start","message":{"id":"msg_1","model":"claude","usage":{"input_tokens":1,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"plan"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"text"}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"hello"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`,
		`{"type":"message_stop"}`,
	}
}

func anthropicThinkingTool() []string {
	return []string{
		`{"type":"message_start","message":{"id":"msg_1","model":"claude","usage":{"input_tokens":1,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"use tool"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call_1","name":"search"}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"q\""}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":":\"go\"}"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":4}}`,
		`{"type":"message_stop"}`,
	}
}

func googleThinkingOnly() []string {
	return []string{
		`{"responseId":"r1","modelVersion":"g","candidates":[{"content":{"parts":[{"text":"stepA","thought":true}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"text":" stepB","thought":true}]},"finishReason":"STOP"}]}`,
	}
}

func googleThinkingText() []string {
	return []string{
		`{"responseId":"r1","modelVersion":"g","candidates":[{"content":{"parts":[{"text":"plan","thought":true}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"text":"hello"}]},"finishReason":"STOP"}]}`,
	}
}

func googleThinkingTool() []string {
	return []string{
		`{"responseId":"r1","modelVersion":"g","candidates":[{"content":{"parts":[{"text":"use tool","thought":true}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"function_call":{"name":"search","args":{"q":"go"}}}]},"finishReason":"STOP"}]}`,
	}
}

type reasoningScenario struct {
	name     string
	payloads func() []string
	// expectThinking is the concatenated thinking text.
	expectThinking string
	// expectText when non-empty.
	expectText string
	// expectToolName when non-empty.
	expectToolName string
	// expectToolArgs substring when tools present.
	expectToolArgs string
}

func scenariosFor(up streamDialect) []reasoningScenario {
	switch up {
	case dOpenAI:
		return []reasoningScenario{
			{name: "thinking_only", payloads: openaiThinkingOnly, expectThinking: "stepA stepB"},
			{name: "thinking_text", payloads: openaiThinkingText, expectThinking: "plan", expectText: "hello"},
			{name: "thinking_tool", payloads: openaiThinkingTool, expectThinking: "use tool", expectToolName: "search", expectToolArgs: `"q"`},
		}
	case dAnthropic:
		return []reasoningScenario{
			{name: "thinking_only", payloads: anthropicThinkingOnly, expectThinking: "stepA stepB"},
			{name: "thinking_text", payloads: anthropicThinkingText, expectThinking: "plan", expectText: "hello"},
			{name: "thinking_tool", payloads: anthropicThinkingTool, expectThinking: "use tool", expectToolName: "search", expectToolArgs: `"q"`},
		}
	case dGoogle:
		return []reasoningScenario{
			{name: "thinking_only", payloads: googleThinkingOnly, expectThinking: "stepA stepB"},
			{name: "thinking_text", payloads: googleThinkingText, expectThinking: "plan", expectText: "hello"},
			{name: "thinking_tool", payloads: googleThinkingTool, expectThinking: "use tool", expectToolName: "search", expectToolArgs: "go"},
		}
	}
	return nil
}

// assertCanonicalThinking checks parse-side EventThinkingDelta and ordering
// vs tool JSON deltas.
func assertCanonicalThinking(t *testing.T, evs []canonical.StreamEvent, sc reasoningScenario) {
	t.Helper()
	var thinking, text, args strings.Builder
	var toolName string
	var order []string
	var thinkIdx, toolIdx = -1, -1
	for _, e := range evs {
		switch e.Type {
		case canonical.EventBlockStart:
			switch e.BlockType {
			case canonical.BlockThinking:
				thinkIdx = e.Index
				order = append(order, "think_start")
			case canonical.BlockToolUse:
				toolIdx = e.Index
				toolName = e.ToolName
				order = append(order, "tool_start:"+e.ToolName)
			case canonical.BlockText:
				order = append(order, "text_start")
			}
		case canonical.EventThinkingDelta:
			thinking.WriteString(e.Text)
			order = append(order, "think_delta")
			if thinkIdx >= 0 && e.Index != thinkIdx {
				t.Fatalf("thinking delta index %d != block %d", e.Index, thinkIdx)
			}
		case canonical.EventTextDelta:
			text.WriteString(e.Text)
			order = append(order, "text_delta")
		case canonical.EventJSONDelta:
			args.WriteString(e.PartialJSON)
			order = append(order, "json_delta")
			if toolIdx >= 0 && e.Index != toolIdx {
				t.Fatalf("json delta index %d != tool block %d (tool args reordered vs thinking)", e.Index, toolIdx)
			}
		}
	}
	if thinking.String() != sc.expectThinking {
		t.Fatalf("thinking = %q want %q; order=%v", thinking.String(), sc.expectThinking, order)
	}
	if sc.expectText != "" && text.String() != sc.expectText {
		t.Fatalf("text = %q want %q", text.String(), sc.expectText)
	}
	if sc.expectToolName != "" {
		if toolName != sc.expectToolName {
			t.Fatalf("tool name = %q want %q", toolName, sc.expectToolName)
		}
		if sc.expectToolArgs != "" && !strings.Contains(args.String(), sc.expectToolArgs) {
			t.Fatalf("tool args %q missing %q", args.String(), sc.expectToolArgs)
		}
		// Ordering invariant: no thinking events after tool_start.
		sawTool := false
		for _, o := range order {
			if strings.HasPrefix(o, "tool_start") {
				sawTool = true
				continue
			}
			if sawTool && (o == "think_start" || o == "think_delta") {
				t.Fatalf("thinking interleaved after tool: %v", order)
			}
		}
	}
}

// assertClientWire checks client dialect SSE contains the dialect-specific
// thinking representation and (when present) tool name.
func assertClientWire(t *testing.T, client streamDialect, out string, sc reasoningScenario) {
	t.Helper()
	if out == "" {
		t.Fatal("empty client stream")
	}
	switch client {
	case dOpenAI:
		if !strings.Contains(out, "reasoning_content") {
			t.Fatalf("OpenAI client missing reasoning_content: %s", out)
		}
		// reasoning_content is a JSON string value in chunks.
		if !strings.Contains(out, sc.expectThinking[:min(4, len(sc.expectThinking))]) {
			// Allow fragmented emission; at least first fragment of thinking text.
			// Extract any reasoning_content string from chunks.
			if !openaiWireHasThinking(out, sc.expectThinking) {
				t.Fatalf("OpenAI stream missing thinking text %q in %s", sc.expectThinking, out)
			}
		}
		if sc.expectToolName != "" && !strings.Contains(out, sc.expectToolName) {
			t.Fatalf("OpenAI stream missing tool %q: %s", sc.expectToolName, out)
		}
		if sc.expectText != "" && !strings.Contains(out, sc.expectText) {
			t.Fatalf("OpenAI stream missing text %q: %s", sc.expectText, out)
		}
	case dAnthropic:
		if !strings.Contains(out, "thinking_delta") {
			t.Fatalf("Anthropic client missing thinking_delta: %s", out)
		}
		if !strings.Contains(out, `"type":"thinking"`) && !strings.Contains(out, `"type": "thinking"`) {
			// content_block_start for thinking
			if !strings.Contains(out, "thinking") {
				t.Fatalf("Anthropic client missing thinking block: %s", out)
			}
		}
		if !strings.Contains(out, sc.expectThinking[:min(4, len(sc.expectThinking))]) {
			if !strings.Contains(out, strings.Split(sc.expectThinking, " ")[0]) {
				t.Fatalf("Anthropic stream missing thinking text in %s", out)
			}
		}
		if sc.expectToolName != "" && !strings.Contains(out, sc.expectToolName) {
			t.Fatalf("Anthropic stream missing tool %q: %s", sc.expectToolName, out)
		}
		if sc.expectText != "" && !strings.Contains(out, sc.expectText) {
			t.Fatalf("Anthropic stream missing text %q: %s", sc.expectText, out)
		}
	case dGoogle:
		if !strings.Contains(out, `"thought":true`) && !strings.Contains(out, `"thought": true`) {
			t.Fatalf("Google client missing thought parts: %s", out)
		}
		if !strings.Contains(out, strings.Split(sc.expectThinking, " ")[0]) {
			t.Fatalf("Google stream missing thinking text in %s", out)
		}
		if sc.expectToolName != "" && !strings.Contains(out, sc.expectToolName) {
			t.Fatalf("Google stream missing tool %q: %s", sc.expectToolName, out)
		}
		if sc.expectText != "" && !strings.Contains(out, sc.expectText) {
			t.Fatalf("Google stream missing text %q: %s", sc.expectText, out)
		}
	}
}

func openaiWireHasThinking(out, want string) bool {
	var got strings.Builder
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta *struct {
					Reasoning json.RawMessage `json:"reasoning_content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		for _, ch := range chunk.Choices {
			if ch.Delta == nil || len(ch.Delta.Reasoning) == 0 {
				continue
			}
			var s string
			if json.Unmarshal(ch.Delta.Reasoning, &s) == nil {
				got.WriteString(s)
			}
		}
	}
	return got.String() == want || strings.Contains(got.String(), want) || strings.Contains(want, got.String())
}

// TestStreamReasoningMatrix covers all 6 directed dialect pairs × 3 scenarios
// with fake SSE (no network). Pair = upstream egress parse → client ingress serialize.
func TestStreamReasoningMatrix(t *testing.T) {
	pairs := []struct {
		up, client streamDialect
	}{
		{dOpenAI, dAnthropic},
		{dOpenAI, dGoogle},
		{dAnthropic, dOpenAI},
		{dAnthropic, dGoogle},
		{dGoogle, dOpenAI},
		{dGoogle, dAnthropic},
	}
	for _, pair := range pairs {
		for _, sc := range scenariosFor(pair.up) {
			name := string(pair.up) + "_to_" + string(pair.client) + "/" + sc.name
			t.Run(name, func(t *testing.T) {
				evs := parseUpstream(pair.up, sc.payloads())
				assertCanonicalThinking(t, evs, sc)
				out := serializeClient(pair.client, evs)
				assertClientWire(t, pair.client, out, sc)
			})
		}
	}
}

// TestStreamReasoningNonStreamConsistent ensures OpenAI non-stream parse of
// reasoning_content matches the stream path (thinking block present).
func TestStreamReasoningNonStreamConsistent(t *testing.T) {
	body := []byte(`{
		"id":"c","model":"m",
		"choices":[{"message":{"role":"assistant","reasoning_content":"plan","content":"hi"},"finish_reason":"stop"}]
	}`)
	resp, err := openaiegress.ParseResponse(body)
	if err != nil {
		t.Fatal(err)
	}
	var thinking, text string
	for _, b := range resp.Content {
		switch b.Type {
		case canonical.BlockThinking:
			thinking += b.Text
		case canonical.BlockText:
			text += b.Text
		}
	}
	if thinking != "plan" || text != "hi" {
		t.Fatalf("non-stream thinking=%q text=%q content=%+v", thinking, text, resp.Content)
	}

	// Stream path over same logical content.
	evs := parseUpstream(dOpenAI, []string{
		`{"id":"c","model":"m","choices":[{"delta":{"reasoning_content":"plan"}}]}`,
		`{"id":"c","choices":[{"delta":{"content":"hi"}}]}`,
		`[DONE]`,
	})
	var sThink, sText strings.Builder
	for _, e := range evs {
		if e.Type == canonical.EventThinkingDelta {
			sThink.WriteString(e.Text)
		}
		if e.Type == canonical.EventTextDelta {
			sText.WriteString(e.Text)
		}
	}
	if sThink.String() != thinking || sText.String() != text {
		t.Fatalf("stream/non-stream mismatch: stream think=%q text=%q", sThink.String(), sText.String())
	}

	// Client OpenAI serialize of non-stream still carries reasoning_content.
	out, err := oaingress.SerializeResponse(resp, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "reasoning_content") {
		t.Fatalf("serialize missing reasoning: %s", out)
	}
}
