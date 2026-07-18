package sse

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestScanLinesIncludingNewlines(t *testing.T) {
	input := "data: one\n\ndata: two\r\n"
	var got []string
	err := Scan(strings.NewReader(input), func(line []byte) error {
		got = append(got, string(line))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"data: one\n", "\n", "data: two\r\n"}
	if len(got) != len(want) {
		t.Fatalf("got %q want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %q want %q", i, got[i], want[i])
		}
	}
}

func TestScanCallbackError(t *testing.T) {
	boom := errors.New("stop")
	err := Scan(strings.NewReader("a\nb\n"), func(line []byte) error {
		if bytes.Contains(line, []byte("b")) {
			return boom
		}
		return nil
	})
	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}

func TestScanReadError(t *testing.T) {
	err := Scan(errReader{err: errors.New("disk")}, func([]byte) error { return nil })
	if err == nil || err.Error() != "disk" {
		t.Fatalf("got %v", err)
	}
}

func TestScanEmpty(t *testing.T) {
	n := 0
	if err := Scan(strings.NewReader(""), func([]byte) error { n++; return nil }); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("callbacks = %d", n)
	}
}

func TestData(t *testing.T) {
	cases := []struct {
		line string
		want string
		nil  bool
	}{
		{"data: hello\n", "hello", false},
		{"data:hello\n", "hello", false},
		{"data:  spaced\r\n", " spaced", false},
		{": comment\n", "", true},
		{"event: message\n", "", true},
		{"\n", "", true},
	}
	for _, c := range cases {
		got := Data([]byte(c.line))
		if c.nil {
			if got != nil {
				t.Errorf("%q: want nil, got %q", c.line, got)
			}
			continue
		}
		if string(got) != c.want {
			t.Errorf("%q: got %q want %q", c.line, got, c.want)
		}
	}
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

// ensure io.EOF path is silent
var _ io.Reader = errReader{}
