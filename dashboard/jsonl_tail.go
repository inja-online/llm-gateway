package dashboard

import (
	"bufio"
	"encoding/json"
	"io"
	"os"

	"github.com/inja-online/llm-gateway/hooks"
)

const jsonlTailBytes = 2 << 20

func jsonlFile(path string) bool {
	switch path {
	case "", "stdout", "stderr":
		return false
	default:
		return true
	}
}

func tailJSONL(path string) []hooks.UsageEvent {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil
	}
	var skipPartial bool
	if st.Size() > jsonlTailBytes {
		if _, err := f.Seek(st.Size()-jsonlTailBytes, io.SeekStart); err != nil {
			return nil
		}
		skipPartial = true
	}
	r := bufio.NewReader(f)
	if skipPartial {
		_, _ = r.ReadBytes('\n')
	}
	var out []hooks.UsageEvent
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			var ev hooks.UsageEvent
			if json.Unmarshal(line, &ev) == nil {
				out = append(out, ev)
			}
		}
		if err != nil {
			break
		}
	}
	return out
}
