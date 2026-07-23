package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/inja-online/llm-gateway/config"
	googleegress "github.com/inja-online/llm-gateway/egress/google"
	"github.com/inja-online/llm-gateway/hooks"
)

// ModalityEmbedding is the UsageEvent.Modality value for embedding requests.
const ModalityEmbedding = "embedding"

// handleEmbeddings serves POST /v1/embeddings (OpenAI dialect).
//
//	openai / openai_compat → passthrough to {base}/embeddings (model rewrite)
//	google                 → translate to :embedContent / :batchEmbedContents
func (s *Server) handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	x := s.newExchange(w, r, DialectOpenAI, writeOpenAIError)
	x.ev.Modality = ModalityEmbedding
	defer x.emit()

	body, ok := x.readBody()
	if !ok {
		return
	}

	var head struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &head) != nil || head.Model == "" {
		x.fail(http.StatusBadRequest, "invalid_request_error", "missing or invalid required field: model", hooks.StatusBadRequest)
		return
	}
	x.ev.Model = head.Model

	route, err := Resolve(s.cfg, DialectOpenAI, head.Model)
	if err != nil {
		x.fail(http.StatusNotFound, "invalid_request_error", err.Error(), hooks.StatusBadRequest)
		return
	}
	x.ev.Provider = route.ProviderName
	x.ev.UpstreamModel = route.UpstreamModel

	switch providerKind(route.Provider) {
	case config.KindOpenAI, config.KindOpenAICompat:
		s.embeddingsPassthrough(x, route, body)
	case config.KindGoogle:
		s.embeddingsOpenAIToGoogle(x, route, body)
	default:
		x.fail(http.StatusNotImplemented, "invalid_request_error",
			"embeddings require an openai, openai_compat, or google provider (got "+route.Provider.Kind+")",
			hooks.StatusBadRequest)
	}
}

// embeddingsPassthrough rewrites model and posts to {base}/embeddings.
func (s *Server) embeddingsPassthrough(x *exchange, route Route, body []byte) {
	var req map[string]any
	if json.Unmarshal(body, &req) != nil {
		x.fail(http.StatusBadRequest, "invalid_request_error", "request body is not valid JSON", hooks.StatusBadRequest)
		return
	}
	req["model"] = route.UpstreamModel
	upstreamBody, _ := json.Marshal(req)

	resp, ok := x.sendUpstream(route, "/embeddings", upstreamBody)
	if !ok {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		x.forwardErrorResponse(resp)
		return
	}
	s.forwardEmbeddingsJSON(x, resp)
}

// forwardEmbeddingsJSON relays a successful OpenAI-shaped embeddings response
// and records prompt token usage when present.
func (s *Server) forwardEmbeddingsJSON(x *exchange, resp *http.Response) {
	body, err := readAll(resp)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to read upstream response", hooks.StatusUpstreamError)
		return
	}
	var parsed struct {
		Usage *struct {
			PromptTokens int `json:"prompt_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Usage != nil {
		x.ev.TokensIn = parsed.Usage.PromptTokens
		if x.ev.TokensIn == 0 {
			x.ev.TokensIn = parsed.Usage.TotalTokens
		}
	} else {
		x.ev.Estimated = true
	}
	x.ev.Status = hooks.StatusOK
	x.ev.HTTPStatus = resp.StatusCode
	x.w.Header().Set("Content-Type", "application/json")
	x.w.WriteHeader(resp.StatusCode)
	x.w.Write(body)
}

// embeddingsOpenAIToGoogle maps an OpenAI embeddings request onto Gemini
// embedContent (single input) or batchEmbedContents (multiple inputs) and
// maps the response back to OpenAI list shape.
func (s *Server) embeddingsOpenAIToGoogle(x *exchange, route Route, body []byte) {
	var req struct {
		Model          string          `json:"model"`
		Input          json.RawMessage `json:"input"`
		Dimensions     *int            `json:"dimensions"`
		EncodingFormat string          `json:"encoding_format"` // float|base64 — Google returns float only
		// TaskType maps to Gemini taskType when present (#148).
		TaskType string `json:"task_type"`
	}
	if json.Unmarshal(body, &req) != nil {
		x.fail(http.StatusBadRequest, "invalid_request_error", "request body is not valid JSON", hooks.StatusBadRequest)
		return
	}
	if req.EncodingFormat != "" && req.EncodingFormat != "float" {
		x.fail(http.StatusBadRequest, "invalid_request_error",
			"encoding_format "+req.EncodingFormat+" is not supported on google embed translate (use float or openai-family passthrough)",
			hooks.StatusBadRequest)
		return
	}
	texts, err := parseOpenAIEmbeddingInput(req.Input)
	if err != nil {
		x.fail(http.StatusBadRequest, "invalid_request_error", err.Error(), hooks.StatusBadRequest)
		return
	}
	if len(texts) == 0 {
		x.fail(http.StatusBadRequest, "invalid_request_error", "missing or empty required field: input", hooks.StatusBadRequest)
		return
	}

	opts := googleegress.EmbedOptions{Dimensions: req.Dimensions, TaskType: req.TaskType}
	var (
		upstreamPath string
		upstreamBody []byte
	)
	if len(texts) == 1 {
		upstreamPath = googleegress.EmbedPath(route.UpstreamModel)
		upstreamBody, err = googleegress.BuildEmbedContentOpts(texts[0], route.UpstreamModel, opts)
	} else {
		upstreamPath = googleegress.BatchEmbedPath(route.UpstreamModel)
		upstreamBody, err = googleegress.BuildBatchEmbedContentsOpts(texts, route.UpstreamModel, opts)
	}
	if err != nil {
		x.fail(http.StatusBadRequest, "invalid_request_error", "failed to build upstream request", hooks.StatusBadRequest)
		return
	}

	resp, ok := x.sendUpstream(route, upstreamPath, upstreamBody)
	if !ok {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		s.translateGoogleErrorToOpenAI(x, resp)
		return
	}
	respBody, err := readAll(resp)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to read upstream response", hooks.StatusUpstreamError)
		return
	}

	vectors, promptTokens, hasUsage, err := googleegress.ParseEmbedResponse(respBody, len(texts) > 1)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to parse upstream embedding response", hooks.StatusUpstreamError)
		return
	}
	if hasUsage {
		x.ev.TokensIn = promptTokens
	} else {
		x.ev.Estimated = true
	}

	out := openAIEmbeddingsResponse(route.UpstreamModel, vectors, promptTokens)
	raw, _ := json.Marshal(out)
	x.ev.Status = hooks.StatusOK
	x.ev.HTTPStatus = http.StatusOK
	x.w.Header().Set("Content-Type", "application/json")
	x.w.WriteHeader(http.StatusOK)
	x.w.Write(raw)
}

// parseOpenAIEmbeddingInput accepts a JSON string or array of strings.
// Token-id arrays are rejected with a clear error (not representable on Google).
func parseOpenAIEmbeddingInput(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, fmt.Errorf("missing or empty required field: input")
	}
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		return []string{one}, nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		return many, nil
	}
	var anyArr []json.RawMessage
	if err := json.Unmarshal(raw, &anyArr); err == nil {
		out := make([]string, 0, len(anyArr))
		for i, el := range anyArr {
			var s string
			if json.Unmarshal(el, &s) != nil {
				return nil, fmt.Errorf("input[%d]: only string inputs are supported", i)
			}
			out = append(out, s)
		}
		return out, nil
	}
	return nil, fmt.Errorf("input must be a string or array of strings")
}

func openAIEmbeddingsResponse(model string, vectors [][]float64, promptTokens int) map[string]any {
	data := make([]map[string]any, len(vectors))
	for i, v := range vectors {
		data[i] = map[string]any{
			"object":    "embedding",
			"index":     i,
			"embedding": v,
		}
	}
	return map[string]any{
		"object": "list",
		"data":   data,
		"model":  model,
		"usage": map[string]any{
			"prompt_tokens": promptTokens,
			"total_tokens":  promptTokens,
		},
	}
}

// googleEmbedPassthrough forwards native Gemini embedContent / batchEmbedContents.
func (s *Server) googleEmbedPassthrough(x *exchange, route Route, body []byte, method string) {
	upstreamBody := rewriteGoogleEmbedBody(body, route.UpstreamModel, method)
	path := googleegress.EmbedActionPath(route.UpstreamModel, method)
	resp, ok := x.sendUpstream(route, path, upstreamBody)
	if !ok {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		x.forwardErrorResponse(resp)
		return
	}
	respBody, err := readAll(resp)
	if err != nil {
		x.fail(http.StatusBadGateway, "api_error", "failed to read upstream response", hooks.StatusUpstreamError)
		return
	}
	batch := method == "batchEmbedContents"
	if _, tokens, hasUsage, err := googleegress.ParseEmbedResponse(respBody, batch); err == nil && hasUsage {
		x.ev.TokensIn = tokens
	} else {
		x.ev.Estimated = true
	}
	x.ev.Status = hooks.StatusOK
	x.ev.HTTPStatus = resp.StatusCode
	x.w.Header().Set("Content-Type", "application/json")
	x.w.WriteHeader(resp.StatusCode)
	x.w.Write(respBody)
}

// rewriteGoogleEmbedBody rewrites optional model fields to models/{upstream}.
func rewriteGoogleEmbedBody(body []byte, upstreamModel, method string) []byte {
	modelRef := googleegress.ModelResource(upstreamModel)
	switch method {
	case "embedContent":
		var req map[string]any
		if json.Unmarshal(body, &req) != nil {
			return body
		}
		if _, ok := req["model"]; ok {
			req["model"] = modelRef
		}
		out, err := json.Marshal(req)
		if err != nil {
			return body
		}
		return out
	case "batchEmbedContents":
		var req map[string]any
		if json.Unmarshal(body, &req) != nil {
			return body
		}
		reqs, _ := req["requests"].([]any)
		for _, item := range reqs {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if _, has := m["model"]; has {
				m["model"] = modelRef
			}
		}
		out, err := json.Marshal(req)
		if err != nil {
			return body
		}
		return out
	default:
		return body
	}
}

// isGoogleEmbedMethod reports whether method is a Gemini embeddings action.
func isGoogleEmbedMethod(method string) bool {
	return method == "embedContent" || method == "batchEmbedContents"
}
