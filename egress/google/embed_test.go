package google

import (
	"encoding/json"
	"testing"
)

func TestModelResourceAndPaths(t *testing.T) {
	if got := ModelResource("gemini-embedding-001"); got != "models/gemini-embedding-001" {
		t.Fatalf("%s", got)
	}
	if got := ModelResource("models/x"); got != "models/x" {
		t.Fatalf("%s", got)
	}
	if EmbedPath("gemini-embedding-001") != "/models/gemini-embedding-001:embedContent" {
		t.Fatal(EmbedPath("gemini-embedding-001"))
	}
	if BatchEmbedPath("models/gemini-embedding-001") != "/models/gemini-embedding-001:batchEmbedContents" {
		t.Fatal(BatchEmbedPath("models/gemini-embedding-001"))
	}
	if EmbedActionPath("m", "batchEmbedContents") != "/models/m:batchEmbedContents" {
		t.Fatal(EmbedActionPath("m", "batchEmbedContents"))
	}
}

func TestBuildEmbedContent(t *testing.T) {
	dim := 8
	raw, err := BuildEmbedContent("hello", "gemini-embedding-001", &dim)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["model"] != "models/gemini-embedding-001" {
		t.Fatalf("%v", m["model"])
	}
	if m["outputDimensionality"] != float64(8) {
		t.Fatalf("%v", m["outputDimensionality"])
	}
	content := m["content"].(map[string]any)
	parts := content["parts"].([]any)
	if parts[0].(map[string]any)["text"] != "hello" {
		t.Fatalf("%v", parts)
	}
}

func TestBuildBatchEmbedContents(t *testing.T) {
	raw, err := BuildBatchEmbedContents([]string{"a", "b"}, "emb", nil)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal(raw, &m)
	reqs := m["requests"].([]any)
	if len(reqs) != 2 {
		t.Fatalf("%d", len(reqs))
	}
	if reqs[0].(map[string]any)["model"] != "models/emb" {
		t.Fatalf("%v", reqs[0])
	}
}

func TestParseEmbedResponseSingle(t *testing.T) {
	body := []byte(`{"embedding":{"values":[0.1,0.2]},"usageMetadata":{"promptTokenCount":4}}`)
	vecs, tok, has, err := ParseEmbedResponse(body, false)
	if err != nil || !has || tok != 4 || len(vecs) != 1 || len(vecs[0]) != 2 {
		t.Fatalf("%v %d %v %v", vecs, tok, has, err)
	}
}

func TestParseEmbedResponseBatch(t *testing.T) {
	body := []byte(`{"embeddings":[{"values":[1]},{"values":[2,3]}],"usage_metadata":{"prompt_token_count":9}}`)
	vecs, tok, has, err := ParseEmbedResponse(body, true)
	if err != nil || !has || tok != 9 || len(vecs) != 2 {
		t.Fatalf("%v %d %v %v", vecs, tok, has, err)
	}
	if len(vecs[1]) != 2 {
		t.Fatalf("%v", vecs[1])
	}
}

func TestParseEmbedResponseNoUsage(t *testing.T) {
	body := []byte(`{"embedding":{"values":[1]}}`)
	_, tok, has, err := ParseEmbedResponse(body, false)
	if err != nil || has || tok != 0 {
		t.Fatalf("%d %v %v", tok, has, err)
	}
}
