package google

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/inja-online/llm-gateway/canonical"
)

// BuildRequest converts a canonical request into a Gemini generateContent body.
// The model id is not placed in the body — it belongs in the URL path (see Path).
//
// Image / media policy (#36, #35):
//  1. Kind base64 (or data: URL) → inline_data
//  2. Kind url / http(s) / Files API URI → file_data.file_uri pass-through (no fetch, no SSRF)
//  3. Unusable image sources are omitted (no silent "[image: …]" text placeholder)
//
// Document policy (#32): BlockDocument (PDF) maps the same way as images —
// inline_data or file_data with application/pdf.
func BuildRequest(req *canonical.Request, _ string) ([]byte, error) {
	out := generateRequest{}
	if sys := concatText(req.System); sys != "" {
		out.SystemInstruction = &content{Parts: []part{{Text: sys}}}
	}
	if req.CachedContent != "" {
		out.CachedContent = req.CachedContent
	}
	if len(req.SafetySettings) > 0 {
		out.SafetySettings = req.SafetySettings
	}
	if gc := buildGenerationConfig(req); gc != nil {
		out.GenerationConfig = gc
	}
	if len(req.Tools) > 0 {
		var decls []functionDeclaration
		for _, t := range req.Tools {
			kind := t.Kind
			if kind == "" {
				kind = canonical.ToolKindFunction
			}
			if kind != canonical.ToolKindFunction {
				return nil, fmt.Errorf("google translation does not support tool kind %q; use function tools or OpenAI-family passthrough", kind)
			}
			params := t.Schema
			if len(params) == 0 {
				params = json.RawMessage(`{"type":"object"}`)
			}
			decls = append(decls, functionDeclaration{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			})
		}
		out.Tools = []tool{{FunctionDeclarations: decls}}
	}
	if req.ToolChoice != nil {
		out.ToolConfig = &toolConfig{FunctionCallingConfig: buildToolChoice(req.ToolChoice)}
	}
	// Map tool_use IDs → names for function responses (Gemini keys by name).
	toolNames := map[string]string{}
	for _, m := range req.Messages {
		for _, b := range m.Content {
			if b.Type == canonical.BlockToolUse {
				toolNames[b.ID] = b.Name
			}
		}
	}
	for _, m := range req.Messages {
		out.Contents = append(out.Contents, buildContent(m, toolNames))
	}
	return json.Marshal(out)
}

// buildGenerationConfig assembles generation_config from sampling, structured
// output (#28), thinking (#30), top_k (#37), and seed (#39).
func buildGenerationConfig(req *canonical.Request) *generationConfig {
	if req == nil {
		return nil
	}
	need := req.Temperature != nil || req.TopP != nil || req.MaxTokens > 0 ||
		len(req.StopSequences) > 0 || req.TopK != nil || req.Seed != nil ||
		req.ResponseFormat != nil || req.Thinking != nil
	if !need {
		return nil
	}
	gc := &generationConfig{
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		MaxOutputTokens: req.MaxTokens,
		StopSequences:   req.StopSequences,
		TopK:            req.TopK,
		Seed:            req.Seed,
	}
	applyResponseFormat(gc, req.ResponseFormat)
	if tc := buildThinkingConfig(req.Thinking); tc != nil {
		gc.ThinkingConfig = tc
	}
	return gc
}

// applyResponseFormat maps canonical ResponseFormat → response_mime_type +
// response_schema (#28).
//
//	json_object → application/json (no schema)
//	json_schema → application/json + response_schema body
//	text        → text/plain (explicit free-form)
func applyResponseFormat(gc *generationConfig, rf *canonical.ResponseFormat) {
	if gc == nil || rf == nil {
		return
	}
	switch rf.Kind {
	case canonical.ResponseFormatJSONObject:
		gc.ResponseMIMEType = "application/json"
	case canonical.ResponseFormatJSONSchema:
		gc.ResponseMIMEType = "application/json"
		if len(rf.Schema) > 0 {
			gc.ResponseSchema = rf.Schema
		}
	case canonical.ResponseFormatText:
		gc.ResponseMIMEType = "text/plain"
	}
}

// buildThinkingConfig maps ThinkingConfig → thinking_config (#30).
//
//	disabled → thinking_budget: 0
//	budget set → thinking_budget
//	effort only → best-effort budget table (same as Anthropic egress)
//	include_thoughts passthrough
//
// Nil / empty config yields nil (omit; no accidental enable).
func buildThinkingConfig(tc *canonical.ThinkingConfig) *thinkingConfigWire {
	if tc == nil {
		return nil
	}
	if tc.Type == "" && tc.BudgetTokens == nil && tc.IncludeThoughts == nil && tc.Effort == "" {
		return nil
	}
	out := &thinkingConfigWire{
		IncludeThoughts: tc.IncludeThoughts,
	}
	switch tc.Type {
	case "disabled":
		zero := 0
		out.ThinkingBudget = &zero
	default:
		budget := tc.BudgetTokens
		if budget == nil && tc.Effort != "" {
			budget = effortToBudget(tc.Effort)
		}
		out.ThinkingBudget = budget
	}
	// If we still have nothing concrete, omit rather than send {}.
	if out.ThinkingBudget == nil && out.IncludeThoughts == nil {
		return nil
	}
	return out
}

// effortToBudget is the documented best-effort OpenAI/Anthropic effort → token
// budget table shared with Anthropic egress.
func effortToBudget(effort string) *int {
	var n int
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "minimal", "low":
		n = 1024
	case "medium":
		n = 8192
	case "high", "xhigh", "max":
		n = 16384
	default:
		return nil
	}
	return &n
}

func buildContent(m canonical.Message, toolNames map[string]string) content {
	role := "user"
	if m.Role == canonical.RoleAssistant {
		role = "model"
	}
	c := content{Role: role}
	for _, b := range m.Content {
		switch b.Type {
		case canonical.BlockText:
			c.Parts = append(c.Parts, part{Text: b.Text})
		case canonical.BlockThinking:
			c.Parts = append(c.Parts, part{Text: b.Text, Thought: true, ThoughtSignature: b.Signature})
		case canonical.BlockImage:
			if p, ok := buildImagePart(b.Image); ok {
				c.Parts = append(c.Parts, p)
			}
			// Unusable image: omit part (no text placeholder).
		case canonical.BlockDocument:
			if p, ok := buildDocumentPart(b.Document); ok {
				c.Parts = append(c.Parts, p)
			}
		case canonical.BlockToolUse:
			args := b.Input
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			c.Parts = append(c.Parts, part{FunctionCall: &functionCall{Name: b.Name, Args: args}})
		case canonical.BlockToolResult:
			name := b.ToolUseID
			if n, ok := toolNames[b.ToolUseID]; ok {
				name = n
			}
			c.Parts = append(c.Parts, part{FunctionResponse: &functionResponse{
				Name:     name,
				Response: toolResultAsJSON(b),
			}})
		}
	}
	if len(c.Parts) == 0 {
		c.Parts = []part{{Text: ""}}
	}
	return c
}

// buildImagePart maps a canonical ImageSource to a Gemini part.
// Returns ok=false when the source is empty/unusable (caller omits the part).
func buildImagePart(img *canonical.ImageSource) (part, bool) {
	if img == nil || img.Data == "" {
		return part{}, false
	}
	return buildMediaPart(img.Kind, img.MediaType, img.Data)
}

// buildDocumentPart maps BlockDocument (primarily PDF) to inline_data / file_data (#32).
func buildDocumentPart(doc *canonical.DocumentSource) (part, bool) {
	if doc == nil || doc.Data == "" {
		return part{}, false
	}
	mt := doc.MediaType
	if mt == "" {
		mt = "application/pdf"
	}
	return buildMediaPart(doc.Kind, mt, doc.Data)
}

// buildMediaPart shared path for images and documents.
func buildMediaPart(kind, mediaType, data string) (part, bool) {
	// data: URLs (sometimes carried as Kind "url") → inline_data, no network I/O.
	if mt, payload, ok := parseDataURL(data); ok {
		if mt == "" {
			mt = mediaType
		}
		if mt == "" {
			mt = "application/octet-stream"
		}
		return part{InlineData: &blob{MIMEType: mt, Data: payload}}, true
	}
	if kind == "base64" {
		mt := mediaType
		if mt == "" {
			mt = "application/octet-stream"
		}
		return part{InlineData: &blob{MIMEType: mt, Data: data}}, true
	}
	// URL / Files API URI / file_uri → file_data pass-through (preferred over fetch).
	uri := data
	mt := mediaType
	if mt == "" {
		mt = guessMIMEFromURI(uri)
	}
	return part{FileData: &fileData{MIMEType: mt, FileURI: uri}}, true
}

// parseDataURL parses data:[<mediatype>][;base64],<data>. Returns false if not a data URL.
func parseDataURL(u string) (mediaType, data string, ok bool) {
	const prefix = "data:"
	if !strings.HasPrefix(u, prefix) {
		return "", "", false
	}
	rest := u[len(prefix):]
	meta, payload, cut := strings.Cut(rest, ",")
	if !cut {
		return "", "", false
	}
	mediaType = meta
	if i := strings.Index(meta, ";"); i >= 0 {
		mediaType = meta[:i]
		// Only base64 payloads are accepted as inline (non-base64 data: is rare).
		if !strings.Contains(meta[i:], "base64") {
			return "", "", false
		}
	} else if mediaType != "" && !strings.Contains(meta, "base64") {
		// data:text/plain,hello — not base64; treat as unusable for Gemini inline.
		return "", "", false
	}
	return mediaType, payload, true
}

// guessMIMEFromURI infers a mime type from a URL path extension when clients omit it.
func guessMIMEFromURI(uri string) string {
	// Strip query/fragment for extension detection.
	p := uri
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	ext := strings.ToLower(path.Ext(p))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".mp4":
		return "video/mp4"
	default:
		return ""
	}
}

func buildToolChoice(tc *canonical.ToolChoice) *functionCallingConfig {
	switch tc.Mode {
	case canonical.ToolNone:
		return &functionCallingConfig{Mode: "NONE"}
	case canonical.ToolRequired:
		return &functionCallingConfig{Mode: "ANY"}
	case canonical.ToolSpecific:
		return &functionCallingConfig{Mode: "ANY", AllowedFunctionNames: []string{tc.Name}}
	default:
		return &functionCallingConfig{Mode: "AUTO"}
	}
}

func concatText(blocks []canonical.Block) string {
	var s string
	for _, b := range blocks {
		if b.Type == canonical.BlockText {
			s += b.Text
		}
	}
	return s
}

// toolResultAsJSON maps tool_result to Gemini functionResponse JSON.
// Multimodal ResultBlocks become {"content":[...]} best-effort; plain Result
// stays a JSON object/string as today.
func toolResultAsJSON(b canonical.Block) json.RawMessage {
	if len(b.ResultBlocks) > 0 {
		type contentItem struct {
			Type string `json:"type,omitempty"`
			Text string `json:"text,omitempty"`
		}
		var items []contentItem
		for _, rb := range b.ResultBlocks {
			if rb.Type == canonical.BlockText {
				items = append(items, contentItem{Type: "text", Text: rb.Text})
			}
			// Images in functionResponse are not first-class; skip with text note.
			if rb.Type == canonical.BlockImage && rb.Image != nil {
				items = append(items, contentItem{Type: "text", Text: "[image in tool_result]"})
			}
		}
		raw, _ := json.Marshal(map[string]any{"content": items})
		return raw
	}
	resp := json.RawMessage(b.Result)
	if !json.Valid(resp) {
		raw, _ := json.Marshal(b.Result)
		return raw
	}
	return resp
}
