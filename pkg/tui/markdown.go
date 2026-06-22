package tui

import (
	"bytes"
	"strings"

	"github.com/charmbracelet/glamour"
)

// renderMarkdown 将 markdown 源转换为 ANSI 样式的终端字符串。
// 对于任何 glamour 错误（实践中极为罕见），我们回退到原始源码，
// 以便用户仍能看到无样式的内容。
//
// 渲染器每次调用时创建；成本是可接受的，因为我们只渲染消息一次
// （在它们达到完成状态后），以避免在每次按键时重新渲染导致的 ANSI 闪烁问题。
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
	// glamour 总是追加一个尾随换行符；修剪它以便消息与视口中紧随其后的内容齐平。
	return strings.TrimRight(out, "\n")
}

// formatThinkingLine 生成在思考块折叠的消息上方显示的占位符行：
// "▶ thinking (~1.2k tokens, press Tab to expand)"。
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

func itoa(i int) string      { return fmtInt(i) }
func ftoa1(f float64) string { return fmtFloat1(f) }
