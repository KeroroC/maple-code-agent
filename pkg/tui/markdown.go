package tui

import (
	"bytes"
	"strings"

	"github.com/charmbracelet/glamour"
)

// renderMarkdown converts markdown source to an ANSI-styled terminal string.
// On any glamour error (extremely rare in practice) we fall back to the raw
// source so the user still sees the content without styling.
//
// Renderers are created per-call; the cost is acceptable because we only
// render messages once (after they reach the done state) to avoid the
// ANSI-flashing problem that occurs when re-rendering on every keystroke.
func renderMarkdown(src string) string {
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(120),
	)
	if err != nil {
		return src
	}
	out, err := r.Render(src)
	if err != nil {
		return src
	}
	// glamour always appends a trailing newline; trim it so the message sits
	// flush against whatever follows in the viewport.
	return strings.TrimRight(out, "\n")
}

// formatThinkingLine produces the placeholder line shown above a message whose
// thinking block is collapsed: "▶ thinking (~1.2k tokens, press Tab to expand)".
func formatThinkingLine(tokens int) string {
	if tokens < 1 {
		tokens = 1
	}
	if tokens < 1000 {
		return formatBytes("▶ thinking (~", tokens, " tokens, press Tab to expand)")
	}
	kilo := float64(tokens) / 1000.0
	return formatBytes("▶ thinking (~", kilo, "k tokens, press Tab to expand)")
}

func formatBytes(parts ...any) string {
	var b bytes.Buffer
	for _, p := range parts {
		switch v := p.(type) {
		case string:
			b.WriteString(v)
		case int:
			b.WriteString(itoa(v))
		case float64:
			b.WriteString(ftoa1(v))
		}
	}
	return b.String()
}

func itoa(i int) string   { return fmtInt(i) }
func ftoa1(f float64) string { return fmtFloat1(f) }
