package tui

import (
	"strconv"
)

// fmtInt 和 fmtFloat1 是小型格式化器，这样 markdown.go 就不需要仅为 Sprintf 调用而导入 "fmt"。
// 它们未导出，仅在 tui 包内部使用。
func fmtInt(i int) string        { return strconv.Itoa(i) }
func fmtFloat1(f float64) string { return strconv.FormatFloat(f, 'f', 1, 64) }
