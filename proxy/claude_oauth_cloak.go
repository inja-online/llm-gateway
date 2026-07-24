package proxy

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	xxHash64 "github.com/pierrec/xxHash/xxHash64"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const claudeCCHSeed uint64 = 0x6E52736AC806831E

var (
	claudeBillingHeaderCCHPattern = regexp.MustCompile(`\bcch=([0-9a-f]{5});`)
	// user_[64hex]_account_[uuid]_session_[uuid]
	userIDPattern = regexp.MustCompile(`^user_[a-fA-F0-9]{64}_account_[0-9a-fA-F-]{36}_session_[0-9a-fA-F-]{36}$`)
)

const fingerprintSalt = "59cf53e54c78"

// isClaudeOAuthToken detects Anthropic consumer OAuth access tokens.
func isClaudeOAuthToken(token string) bool {
	return strings.Contains(token, "sk-ant-oat")
}

// shouldCloakClaude: auto = cloak unless client is already Claude Code CLI.
func shouldCloakClaude(mode, userAgent string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "never", "off", "false":
		return false
	case "always", "on", "true":
		return true
	default: // auto
		return !strings.HasPrefix(userAgent, "claude-cli")
	}
}

func generateFakeUserID() string {
	hexBytes := make([]byte, 32)
	_, _ = rand.Read(hexBytes)
	return "user_" + hex.EncodeToString(hexBytes) + "_account_" + newUUIDv4() + "_session_" + newUUIDv4()
}

func isValidUserID(id string) bool {
	return userIDPattern.MatchString(id)
}

func newUUIDv4() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func injectFakeUserID(payload []byte) []byte {
	existing := gjson.GetBytes(payload, "metadata.user_id").String()
	if existing != "" && isValidUserID(existing) {
		return payload
	}
	payload, _ = sjson.SetBytes(payload, "metadata.user_id", generateFakeUserID())
	return payload
}

func computeFingerprint(messageText, version string) string {
	indices := [3]int{4, 7, 20}
	runes := []rune(messageText)
	var sb strings.Builder
	for _, idx := range indices {
		if idx < len(runes) {
			sb.WriteRune(runes[idx])
		} else {
			sb.WriteRune('0')
		}
	}
	h := sha256.Sum256([]byte(fingerprintSalt + sb.String() + version))
	return hex.EncodeToString(h[:])[:3]
}

func generateBillingHeader(version, messageText, entrypoint string, placeholderCCH bool) string {
	if entrypoint == "" {
		entrypoint = "cli"
	}
	buildHash := computeFingerprint(messageText, version)
	cch := "00000"
	if !placeholderCCH {
		// Temporary hash; final cch is rewritten by signAnthropicMessagesBody over full body.
		h := sha256.Sum256([]byte(messageText + version))
		cch = hex.EncodeToString(h[:])[:5]
	}
	return fmt.Sprintf("x-anthropic-billing-header: cc_version=%s.%s; cc_entrypoint=%s; cch=%s;",
		version, buildHash, entrypoint, cch)
}

func buildTextBlockJSON(text string) string {
	block := []byte(`{"type":"text"}`)
	block, _ = sjson.SetBytes(block, "text", text)
	return string(block)
}

// injectClaudeCodeSystem replaces system with Claude Code–shaped blocks and
// moves original system text into the first user message (sanitized for OAuth).
func injectClaudeCodeSystem(payload []byte, oauthMode bool) []byte {
	system := gjson.GetBytes(payload, "system")
	firstText := gjson.GetBytes(payload, "system.0.text").String()
	if strings.HasPrefix(firstText, "x-anthropic-billing-header:") {
		return payload
	}

	messageText := ""
	if system.IsArray() {
		system.ForEach(func(_, part gjson.Result) bool {
			if part.Get("type").String() == "text" {
				messageText = part.Get("text").String()
				return false
			}
			return true
		})
	} else if system.Type == gjson.String {
		messageText = system.String()
	}

	// Use placeholder cch=00000 then sign over full body.
	billingText := generateBillingHeader(claudeCodeVersion, messageText, "cli", true)
	billingBlock := buildTextBlockJSON(billingText)
	agentBlock := buildTextBlockJSON(claudeCodeAgentID)
	staticPrompt := strings.Join([]string{
		claudeCodeIntro,
		claudeCodeSystem,
		claudeCodeDoingTasks,
		claudeCodeToneAndStyle,
		claudeCodeOutputEfficiency,
	}, "\n\n")
	staticBlock := buildTextBlockJSON(staticPrompt)
	systemJSON := "[" + billingBlock + "," + agentBlock + "," + staticBlock + "]"
	payload, _ = sjson.SetRawBytes(payload, "system", []byte(systemJSON))

	// Move original system text into first user message.
	var userSystemParts []string
	if system.IsArray() {
		system.ForEach(func(_, part gjson.Result) bool {
			if part.Get("type").String() == "text" {
				if t := strings.TrimSpace(part.Get("text").String()); t != "" {
					userSystemParts = append(userSystemParts, t)
				}
			}
			return true
		})
	} else if system.Type == gjson.String {
		if t := strings.TrimSpace(system.String()); t != "" {
			userSystemParts = append(userSystemParts, t)
		}
	}
	if len(userSystemParts) > 0 {
		combined := strings.Join(userSystemParts, "\n\n")
		if oauthMode {
			combined = sanitizeForwardedSystemPrompt(combined)
		}
		if strings.TrimSpace(combined) != "" {
			payload = prependToFirstUserMessage(payload, combined)
		}
	}
	return signAnthropicMessagesBody(payload)
}

func sanitizeForwardedSystemPrompt(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return strings.TrimSpace(`Use the available tools when needed to help with software engineering tasks.
Keep responses concise and focused on the user's request.
Prefer acting on the user's task over describing product-specific workflows.`)
}

func prependToFirstUserMessage(payload []byte, text string) []byte {
	messages := gjson.GetBytes(payload, "messages")
	if !messages.IsArray() {
		return payload
	}
	found := false
	messages.ForEach(func(i, msg gjson.Result) bool {
		if !strings.EqualFold(msg.Get("role").String(), "user") {
			return true
		}
		found = true
		content := msg.Get("content")
		block := buildTextBlockJSON(text)
		if content.IsArray() {
			// Prepend text block
			raw := content.Raw
			if strings.HasPrefix(raw, "[") {
				inner := strings.TrimPrefix(raw, "[")
				newArr := "[" + block + "," + strings.TrimSpace(inner)
				if strings.HasPrefix(strings.TrimSpace(inner), "]") {
					newArr = "[" + block + "]"
				}
				path := fmt.Sprintf("messages.%d.content", i.Int())
				payload, _ = sjson.SetRawBytes(payload, path, []byte(newArr))
			}
		} else if content.Type == gjson.String {
			merged := text + "\n\n" + content.String()
			path := fmt.Sprintf("messages.%d.content", i.Int())
			payload, _ = sjson.SetBytes(payload, path, merged)
		} else {
			path := fmt.Sprintf("messages.%d.content", i.Int())
			payload, _ = sjson.SetRawBytes(payload, path, []byte("["+block+"]"))
		}
		return false
	})
	if !found {
		// Insert a user message at start
		userMsg := fmt.Sprintf(`{"role":"user","content":[%s]}`, buildTextBlockJSON(text))
		if messages.Raw == "[]" || !messages.Exists() {
			payload, _ = sjson.SetRawBytes(payload, "messages", []byte("["+userMsg+"]"))
		} else {
			inner := strings.TrimPrefix(messages.Raw, "[")
			payload, _ = sjson.SetRawBytes(payload, "messages", []byte("["+userMsg+","+inner))
		}
	}
	return payload
}

// signAnthropicMessagesBody sets cch to xxHash64 of body with cch placeholder 00000.
func signAnthropicMessagesBody(body []byte) []byte {
	billingHeader := gjson.GetBytes(body, "system.0.text").String()
	if !strings.HasPrefix(billingHeader, "x-anthropic-billing-header:") {
		return body
	}
	if !claudeBillingHeaderCCHPattern.MatchString(billingHeader) {
		return body
	}
	unsignedBilling := claudeBillingHeaderCCHPattern.ReplaceAllString(billingHeader, "cch=00000;")
	unsignedBody, err := sjson.SetBytes(body, "system.0.text", unsignedBilling)
	if err != nil {
		return body
	}
	cch := fmt.Sprintf("%05x", xxHash64.Checksum(unsignedBody, claudeCCHSeed)&0xFFFFF)
	signedBilling := claudeBillingHeaderCCHPattern.ReplaceAllString(unsignedBilling, "cch="+cch+";")
	signedBody, err := sjson.SetBytes(unsignedBody, "system.0.text", signedBilling)
	if err != nil {
		return unsignedBody
	}
	return signedBody
}
