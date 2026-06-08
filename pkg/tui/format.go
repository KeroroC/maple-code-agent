package tui

import (
	"fmt"
	"strconv"
)

// fmtInt and fmtFloat1 are tiny formatters so markdown.go doesn't import "fmt"
// just for Sprintf calls. They are unexported and only used inside the tui
// package.
func fmtInt(i int) string       { return strconv.Itoa(i) }
func fmtFloat1(f float64) string { return strconv.FormatFloat(f, 'f', 1, 64) }

func formatToolStatus(name string, done, failed bool) string {
	if done {
		if failed {
			return fmt.Sprintf("tool: %s ... failed", name)
		}
		return fmt.Sprintf("tool: %s ... done", name)
	}
	return fmt.Sprintf("tool: %s ... running", name)
}
