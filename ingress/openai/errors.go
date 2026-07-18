package openai

import (
	"strings"

	"github.com/inja-online/llm-gateway/canonical"
)

// ValidationError marks a client request problem (HTTP 400 class).
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// parseImageURL splits a data: URL into media type + base64 payload, or treats
// it as a remote URL. Format: data:image/png;base64,<data>
func parseImageURL(u string) *canonical.ImageSource {
	const dataPrefix = "data:"
	if !strings.HasPrefix(u, dataPrefix) {
		return &canonical.ImageSource{Kind: "url", Data: u}
	}
	rest := u[len(dataPrefix):]
	meta, data, ok := strings.Cut(rest, ",")
	if !ok {
		return &canonical.ImageSource{Kind: "url", Data: u}
	}
	mediaType, enc, _ := strings.Cut(meta, ";")
	if enc != "base64" {
		return &canonical.ImageSource{Kind: "url", Data: u}
	}
	return &canonical.ImageSource{Kind: "base64", MediaType: mediaType, Data: data}
}
