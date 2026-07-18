package google

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ModelResource returns the Gemini resource name "models/{id}".
// If model already starts with "models/", it is returned unchanged.
func ModelResource(model string) string {
	if strings.HasPrefix(model, "models/") {
		return model
	}
	return "models/" + model
}

// EmbedPath is the relative path for models.embedContent.
// base_url should be e.g. https://generativelanguage.googleapis.com/v1beta
func EmbedPath(model string) string {
	return "/models/" + stripModelsPrefix(model) + ":embedContent"
}

// BatchEmbedPath is the relative path for models.batchEmbedContents.
func BatchEmbedPath(model string) string {
	return "/models/" + stripModelsPrefix(model) + ":batchEmbedContents"
}

// EmbedActionPath builds the path for embedContent or batchEmbedContents.
func EmbedActionPath(model, method string) string {
	switch method {
	case "batchEmbedContents":
		return BatchEmbedPath(model)
	default:
		return EmbedPath(model)
	}
}

func stripModelsPrefix(model string) string {
	return strings.TrimPrefix(model, "models/")
}

// BuildEmbedContent builds a single-text EmbedContentRequest body.
func BuildEmbedContent(text, model string, dimensions *int) ([]byte, error) {
	req := map[string]any{
		"model": ModelResource(model),
		"content": map[string]any{
			"parts": []map[string]any{{"text": text}},
		},
	}
	if dimensions != nil && *dimensions > 0 {
		req["outputDimensionality"] = *dimensions
	}
	return json.Marshal(req)
}

// BuildBatchEmbedContents builds a BatchEmbedContentsRequest body.
func BuildBatchEmbedContents(texts []string, model string, dimensions *int) ([]byte, error) {
	ref := ModelResource(model)
	reqs := make([]map[string]any, len(texts))
	for i, t := range texts {
		item := map[string]any{
			"model": ref,
			"content": map[string]any{
				"parts": []map[string]any{{"text": t}},
			},
		}
		if dimensions != nil && *dimensions > 0 {
			item["outputDimensionality"] = *dimensions
		}
		reqs[i] = item
	}
	return json.Marshal(map[string]any{"requests": reqs})
}

// ParseEmbedResponse extracts embedding vectors and optional usage from a
// Gemini embedContent (batch=false) or batchEmbedContents (batch=true) body.
func ParseEmbedResponse(body []byte, batch bool) (vectors [][]float64, promptTokens int, hasUsage bool, err error) {
	if batch {
		var resp struct {
			Embeddings []struct {
				Values []float64 `json:"values"`
			} `json:"embeddings"`
			UsageMetadata *struct {
				PromptTokenCount int `json:"promptTokenCount"`
			} `json:"usageMetadata"`
			UsageSnake *struct {
				PromptTokenCount int `json:"prompt_token_count"`
			} `json:"usage_metadata"`
		}
		if e := json.Unmarshal(body, &resp); e != nil {
			return nil, 0, false, fmt.Errorf("batch embed response: %w", e)
		}
		if len(resp.Embeddings) == 0 {
			return nil, 0, false, fmt.Errorf("batch embed response: no embeddings")
		}
		vectors = make([][]float64, len(resp.Embeddings))
		for i, e := range resp.Embeddings {
			vectors[i] = e.Values
		}
		if resp.UsageMetadata != nil && resp.UsageMetadata.PromptTokenCount > 0 {
			return vectors, resp.UsageMetadata.PromptTokenCount, true, nil
		}
		if resp.UsageSnake != nil && resp.UsageSnake.PromptTokenCount > 0 {
			return vectors, resp.UsageSnake.PromptTokenCount, true, nil
		}
		return vectors, 0, false, nil
	}

	var resp struct {
		Embedding *struct {
			Values []float64 `json:"values"`
		} `json:"embedding"`
		// Some SDKs/docs also return embeddings[] for multi-content single call.
		Embeddings []struct {
			Values []float64 `json:"values"`
		} `json:"embeddings"`
		UsageMetadata *struct {
			PromptTokenCount int `json:"promptTokenCount"`
		} `json:"usageMetadata"`
		UsageSnake *struct {
			PromptTokenCount int `json:"prompt_token_count"`
		} `json:"usage_metadata"`
	}
	if e := json.Unmarshal(body, &resp); e != nil {
		return nil, 0, false, fmt.Errorf("embed response: %w", e)
	}
	switch {
	case resp.Embedding != nil && len(resp.Embedding.Values) > 0:
		vectors = [][]float64{resp.Embedding.Values}
	case len(resp.Embeddings) > 0:
		vectors = make([][]float64, len(resp.Embeddings))
		for i, e := range resp.Embeddings {
			vectors[i] = e.Values
		}
	default:
		return nil, 0, false, fmt.Errorf("embed response: no embedding values")
	}
	if resp.UsageMetadata != nil && resp.UsageMetadata.PromptTokenCount > 0 {
		return vectors, resp.UsageMetadata.PromptTokenCount, true, nil
	}
	if resp.UsageSnake != nil && resp.UsageSnake.PromptTokenCount > 0 {
		return vectors, resp.UsageSnake.PromptTokenCount, true, nil
	}
	return vectors, 0, false, nil
}
