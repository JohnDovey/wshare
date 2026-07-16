package about

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestWrapRespectsWidth(t *testing.T) {
	text := ProjectBlurb()
	for _, width := range []int{28, 36, 42, 48} {
		out := Wrap(text, width)
		for i, line := range strings.Split(out, "\n") {
			if utf8.RuneCountInString(line) > width {
				t.Fatalf("width %d line %d too long (%d): %q",
					width, i, utf8.RuneCountInString(line), line)
			}
		}
	}
}

func TestWrapIndent(t *testing.T) {
	out := WrapIndent("Original author created wShare for easy intranet file sharing on the local network", 36, "    ")
	for i, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "    ") {
			t.Fatalf("line %d missing indent: %q", i, line)
		}
		if utf8.RuneCountInString(line) > 36 {
			t.Fatalf("line %d too long: %q", i, line)
		}
	}
}

func TestWrapLongURL(t *testing.T) {
	url := "https://github.com/thewhitetulip/wshare/with/a/very/long/path/that/must/break"
	out := WrapIndent(url, 30, "  ")
	for i, line := range strings.Split(out, "\n") {
		if utf8.RuneCountInString(line) > 30 {
			t.Fatalf("line %d too long: %q", i, line)
		}
	}
	joined := strings.ReplaceAll(out, "\n", "")
	joined = strings.ReplaceAll(joined, "  ", "")
	// Reconstruct without indents
	var rebuilt strings.Builder
	for _, line := range strings.Split(out, "\n") {
		rebuilt.WriteString(strings.TrimPrefix(line, "  "))
	}
	if rebuilt.String() != url {
		t.Fatalf("url mangled:\n got %q\nwant %q", rebuilt.String(), url)
	}
}

func TestPlainTextFits(t *testing.T) {
	const width = 42
	out := PlainText(AppAdmin, width)
	for i, line := range strings.Split(out, "\n") {
		if utf8.RuneCountInString(line) > width {
			t.Fatalf("line %d exceeds %d (%d): %q",
				i, width, utf8.RuneCountInString(line), line)
		}
	}
}
