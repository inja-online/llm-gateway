package anthropic

import (
	"encoding/json"

	"github.com/mamad/llm-gateway/canonical"
)

// SerializeResponse renders a canonical response as an Anthropic Messages
// JSON body.
func SerializeResponse(resp *canonical.Response) ([]byte, error) {
	out := messagesResponse{
		ID:         responseID(resp.ID),
		Type:       "message",
		Role:       canonical.RoleAssistant,
		Model:      resp.Model,
		StopReason: stopReason(resp.StopReason),
		Usage: outUsage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
		},
	}
	for _, b := range resp.Content {
		if ob, ok := outBlockFor(b); ok {
			out.Content = append(out.Content, ob)
		}
	}
	if out.Content == nil {
		out.Content = []outBlock{}
	}
	return json.Marshal(out)
}

func outBlockFor(b canonical.Block) (outBlock, bool) {
	switch b.Type {
	case canonical.BlockText:
		return outBlock{Type: "text", Text: b.Text}, true
	case canonical.BlockThinking:
		return outBlock{Type: "thinking", Thinking: b.Text, Signature: b.Signature}, true
	case canonical.BlockToolUse:
		input := b.Input
		if len(input) == 0 {
			input = json.RawMessage(`{}`)
		}
		return outBlock{Type: "tool_use", ID: b.ID, Name: b.Name, Input: input}, true
	}
	return outBlock{}, false
}

func stopReason(sr string) string {
	if sr == "" {
		return canonical.StopEndTurn
	}
	return sr
}

func responseID(id string) string {
	if id == "" {
		return "msg_gateway"
	}
	return id
}
