// Package sse provides minimal helpers for reading Server-Sent Event streams
// line-by-line while preserving the exact bytes for passthrough.
package sse

import (
	"bufio"
	"bytes"
	"io"
)

// DataPrefix is the SSE data field prefix.
var DataPrefix = []byte("data:")

// Scan reads r line by line (including the trailing newline bytes) and calls
// fn with each raw line. fn receives the exact bytes to forward. Scan returns
// the first error from r other than io.EOF.
func Scan(r io.Reader, fn func(line []byte) error) error {
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			if cbErr := fn(line); cbErr != nil {
				return cbErr
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// Data extracts the payload of a "data:" line, or nil if the line is not a
// data field. Handles both "data: x" and "data:x". Trailing newlines trimmed.
func Data(line []byte) []byte {
	trimmed := bytes.TrimRight(line, "\r\n")
	if !bytes.HasPrefix(trimmed, DataPrefix) {
		return nil
	}
	payload := trimmed[len(DataPrefix):]
	return bytes.TrimPrefix(payload, []byte(" "))
}
