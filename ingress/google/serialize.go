package google

import (
	"encoding/json"

	"github.com/inja-online/llm-gateway/canonical"
)

// SerializeResponse renders a canonical response as a Gemini generateContent body.
func SerializeResponse(resp *canonical.Response) ([]byte, error) {
	out := generateResponse{
		ResponseID:   responseID(resp.ID),
		ModelVersion: resp.Model,
		Candidates: []candidate{{
			Index:        0,
			Content:      blocksToContent(resp.Content),
			FinishReason: stopToFinish(resp.StopReason),
		}},
	}
	if resp.Usage.HasUsage {
		out.UsageMetadata = &usageMetadata{
			PromptTokenCount:        resp.Usage.InputTokens,
			CandidatesTokenCount:    resp.Usage.OutputTokens,
			TotalTokenCount:         resp.Usage.InputTokens + resp.Usage.OutputTokens,
			CachedContentTokenCount: resp.Usage.CacheReadTokens,
			ThoughtsTokenCount:      resp.Usage.ReasoningTokens,
		}
	}
	return json.Marshal(out)
}

func blocksToContent(blocks []canonical.Block) *content {
	c := &content{Role: "model"}
	for _, b := range blocks {
		switch b.Type {
		case canonical.BlockText:
			c.Parts = append(c.Parts, part{Text: b.Text})
		case canonical.BlockThinking:
			c.Parts = append(c.Parts, part{Text: b.Text, Thought: true})
		case canonical.BlockToolUse:
			args := b.Input
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			c.Parts = append(c.Parts, part{FunctionCall: &functionCall{Name: b.Name, Args: args}})
		}
	}
	if len(c.Parts) == 0 {
		c.Parts = []part{{Text: ""}}
	}
	return c
}

func stopToFinish(sr string) string {
	switch sr {
	case canonical.StopEndTurn:
		return "STOP"
	case canonical.StopMaxTokens:
		return "MAX_TOKENS"
	case canonical.StopToolUse:
		return "STOP" // Gemini uses STOP with functionCall parts
	case canonical.StopRefusal:
		return "SAFETY"
	default:
		return "STOP"
	}
}

func responseID(id string) string {
	if id == "" {
		return "resp_gateway"
	}
	return id
}
