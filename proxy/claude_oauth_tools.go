package proxy

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// oauthToolRenameMap maps third-party (often lowercase) tool names to Claude Code
// TitleCase names. Anthropic fingerprints OAuth traffic by tool naming patterns.
var oauthToolRenameMap = map[string]string{
	"bash":         "Bash",
	"read":         "Read",
	"write":        "Write",
	"edit":         "Edit",
	"glob":         "Glob",
	"grep":         "Grep",
	"task":         "Task",
	"webfetch":     "WebFetch",
	"todowrite":    "TodoWrite",
	"question":     "Question",
	"skill":        "Skill",
	"ls":           "LS",
	"todoread":     "TodoRead",
	"notebookedit": "NotebookEdit",
}

// remapOAuthToolNames renames tools / tool_choice / tool_use references for OAuth.
// reverseMap is keyed by upstream (TitleCase) name → original client name.
func remapOAuthToolNames(body []byte) ([]byte, map[string]string) {
	reverseMap := make(map[string]string, len(oauthToolRenameMap))
	record := func(original, renamed string) {
		if _, exists := reverseMap[renamed]; !exists {
			reverseMap[renamed] = original
		}
	}

	tools := gjson.GetBytes(body, "tools")
	if tools.Exists() && tools.IsArray() {
		need := false
		tools.ForEach(func(_, tool gjson.Result) bool {
			if tool.Get("type").Exists() && tool.Get("type").String() != "" {
				return true
			}
			name := tool.Get("name").String()
			if newName, ok := oauthToolRenameMap[name]; ok && newName != name {
				need = true
				return false
			}
			return true
		})
		if need {
			var b strings.Builder
			b.WriteByte('[')
			n := 0
			tools.ForEach(func(_, tool gjson.Result) bool {
				if tool.Get("type").Exists() && tool.Get("type").String() != "" {
					if n > 0 {
						b.WriteByte(',')
					}
					b.WriteString(tool.Raw)
					n++
					return true
				}
				toolJSON := tool.Raw
				name := tool.Get("name").String()
				if newName, ok := oauthToolRenameMap[name]; ok && newName != name {
					if updated, err := sjson.Set(toolJSON, "name", newName); err == nil {
						toolJSON = updated
						record(name, newName)
					}
				}
				if n > 0 {
					b.WriteByte(',')
				}
				b.WriteString(toolJSON)
				n++
				return true
			})
			b.WriteByte(']')
			body, _ = sjson.SetRawBytes(body, "tools", []byte(b.String()))
		}
	}

	if gjson.GetBytes(body, "tool_choice.type").String() == "tool" {
		tcName := gjson.GetBytes(body, "tool_choice.name").String()
		if newName, ok := oauthToolRenameMap[tcName]; ok && newName != tcName {
			body, _ = sjson.SetBytes(body, "tool_choice.name", newName)
			record(tcName, newName)
		}
	}

	messages := gjson.GetBytes(body, "messages")
	if messages.Exists() && messages.IsArray() {
		messages.ForEach(func(msgIndex, msg gjson.Result) bool {
			content := msg.Get("content")
			if !content.Exists() || !content.IsArray() {
				return true
			}
			content.ForEach(func(contentIndex, part gjson.Result) bool {
				switch part.Get("type").String() {
				case "tool_use":
					name := part.Get("name").String()
					if newName, ok := oauthToolRenameMap[name]; ok && newName != name {
						path := fmt.Sprintf("messages.%d.content.%d.name", msgIndex.Int(), contentIndex.Int())
						body, _ = sjson.SetBytes(body, path, newName)
						record(name, newName)
					}
				case "tool_reference":
					toolName := part.Get("tool_name").String()
					if newName, ok := oauthToolRenameMap[toolName]; ok && newName != toolName {
						path := fmt.Sprintf("messages.%d.content.%d.tool_name", msgIndex.Int(), contentIndex.Int())
						body, _ = sjson.SetBytes(body, path, newName)
						record(toolName, newName)
					}
				}
				return true
			})
			return true
		})
	}
	return body, reverseMap
}

// reverseRemapOAuthToolNames restores client tool names in a non-stream response.
func reverseRemapOAuthToolNames(body []byte, reverseMap map[string]string) []byte {
	if len(reverseMap) == 0 {
		return body
	}
	content := gjson.GetBytes(body, "content")
	if !content.Exists() || !content.IsArray() {
		return body
	}
	content.ForEach(func(index, part gjson.Result) bool {
		switch part.Get("type").String() {
		case "tool_use":
			name := part.Get("name").String()
			if orig, ok := reverseMap[name]; ok {
				path := fmt.Sprintf("content.%d.name", index.Int())
				body, _ = sjson.SetBytes(body, path, orig)
			}
		case "tool_reference":
			toolName := part.Get("tool_name").String()
			if orig, ok := reverseMap[toolName]; ok {
				path := fmt.Sprintf("content.%d.tool_name", index.Int())
				body, _ = sjson.SetBytes(body, path, orig)
			}
		}
		return true
	})
	return body
}

// reverseRemapOAuthToolNamesFromStreamLine restores tool names in one SSE data line.
func reverseRemapOAuthToolNamesFromStreamLine(line []byte, reverseMap map[string]string) []byte {
	if len(reverseMap) == 0 {
		return line
	}
	// data: {...}\n or bare JSON
	payload := line
	if idx := strings.Index(string(line), "{"); idx >= 0 {
		payload = line[idx:]
		// strip trailing newline for parse; re-wrap later
		payload = []byte(strings.TrimRight(string(payload), "\r\n"))
	}
	if !gjson.ValidBytes(payload) {
		return line
	}
	cb := gjson.GetBytes(payload, "content_block")
	if !cb.Exists() {
		return line
	}
	var updated []byte
	var err error
	switch cb.Get("type").String() {
	case "tool_use":
		name := cb.Get("name").String()
		if orig, ok := reverseMap[name]; ok {
			updated, err = sjson.SetBytes(payload, "content_block.name", orig)
		}
	case "tool_reference":
		name := cb.Get("tool_name").String()
		if orig, ok := reverseMap[name]; ok {
			updated, err = sjson.SetBytes(payload, "content_block.tool_name", orig)
		}
	}
	if err != nil || updated == nil {
		return line
	}
	// Preserve SSE "data: " prefix if present.
	s := string(line)
	if strings.HasPrefix(s, "data:") {
		prefix := s[:strings.Index(s, "{")]
		suffix := ""
		if strings.HasSuffix(s, "\n") {
			suffix = "\n"
		}
		return []byte(prefix + string(updated) + suffix)
	}
	return updated
}
