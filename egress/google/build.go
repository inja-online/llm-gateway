package google

import (
	"encoding/json"
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
func BuildRequest(req *canonical.Request, _ string) ([]byte, error) {
	out := generateRequest{}
	if sys := concatText(req.System); sys != "" {
		out.SystemInstruction = &content{Parts: []part{{Text: sys}}}
	}
	if len(req.SafetySettings) > 0 {
		out.SafetySettings = req.SafetySettings
	}
	if req.Temperature != nil || req.TopP != nil || req.MaxTokens > 0 || len(req.StopSequences) > 0 {
		out.GenerationConfig = &generationConfig{
			Temperature:     req.Temperature,
			TopP:            req.TopP,
			MaxOutputTokens: req.MaxTokens,
			StopSequences:   req.StopSequences,
		}
	}
	if len(req.Tools) > 0 {
		var decls []functionDeclaration
		for _, t := range req.Tools {
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
			c.Parts = append(c.Parts, part{Text: b.Text, Thought: true})
		case canonical.BlockImage:
			if p, ok := buildImagePart(b.Image); ok {
				c.Parts = append(c.Parts, p)
			}
			// Unusable image: omit part (no text placeholder).
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
	// data: URLs (sometimes carried as Kind "url") → inline_data, no network I/O.
	if mt, data, ok := parseDataURL(img.Data); ok {
		if mt == "" {
			mt = img.MediaType
		}
		if mt == "" {
			mt = "application/octet-stream"
		}
		return part{InlineData: &blob{MIMEType: mt, Data: data}}, true
	}
	if img.Kind == "base64" {
		mt := img.MediaType
		if mt == "" {
			mt = "application/octet-stream"
		}
		return part{InlineData: &blob{MIMEType: mt, Data: img.Data}}, true
	}
	// URL / Files API URI → file_data pass-through (preferred over fetch).
	uri := img.Data
	mt := img.MediaType
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
