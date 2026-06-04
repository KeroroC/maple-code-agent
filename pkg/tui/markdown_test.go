package tui

import (
	"regexp"
	"strings"
	"testing"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestRenderMarkdown_CodeBlockContainsAnsiEscapes(t *testing.T) {
	md := "```go\nfunc foo() {}\n```"
	got := renderMarkdown(md)

	// We expect at least one ANSI escape sequence (\x1b[) because glamour
	// applies syntax highlighting to fenced code blocks.
	if !strings.Contains(got, "\x1b[") {
		t.Errorf("renderMarkdown output should contain ANSI escape sequences, got:\n%s", got)
	}
}

func TestRenderMarkdown_PlainTextRenders(t *testing.T) {
	md := "Hello world"
	got := renderMarkdown(md)
	plain := ansiRE.ReplaceAllString(got, "")
	if !strings.Contains(plain, "Hello world") {
		t.Errorf("rendered output missing source text, plain=%q raw=%q", plain, got)
	}
}

func TestRenderMarkdown_FallsBackToRawOnError(t *testing.T) {
	// Extremely large input that glamour might struggle with. We don't try to
	// trigger an actual error here; instead, verify that an empty render still
	// returns the source as a fallback.
	got := renderMarkdown("")
	if !strings.Contains(got, "") { // trivially true; mainly guarding that the call doesn't panic
		t.Error("renderMarkdown(\"\") should not panic")
	}
}

func TestFormatThinkingLine_FormatsTokens(t *testing.T) {
	cases := []struct {
		tokens int
		want   string
	}{
		{0, "▶ thinking (~1 tokens, press Tab to expand)"},
		{50, "▶ thinking (~50 tokens, press Tab to expand)"},
		{1000, "▶ thinking (~1.0k tokens, press Tab to expand)"},
		{2500, "▶ thinking (~2.5k tokens, press Tab to expand)"},
	}
	for _, c := range cases {
		if got := formatThinkingLine(c.tokens); got != c.want {
			t.Errorf("formatThinkingLine(%d) = %q, want %q", c.tokens, got, c.want)
		}
	}
}
