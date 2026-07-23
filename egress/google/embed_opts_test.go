package google

import (
	"encoding/json"
	"testing"
)

func TestBuildEmbedContentOptsTaskType(t *testing.T) {
	d := 256
	body, err := BuildEmbedContentOpts("hi", "text-embedding-004", EmbedOptions{Dimensions: &d, TaskType: "RETRIEVAL_QUERY"})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal(body, &m)
	if m["taskType"] != "RETRIEVAL_QUERY" || m["outputDimensionality"].(float64) != 256 {
		t.Fatalf("%v", m)
	}
	body, _ = BuildBatchEmbedContentsOpts([]string{"a", "b"}, "m", EmbedOptions{TaskType: "SEMANTIC_SIMILARITY"})
	json.Unmarshal(body, &m)
	reqs := m["requests"].([]any)
	if len(reqs) != 2 {
		t.Fatal(reqs)
	}
}
