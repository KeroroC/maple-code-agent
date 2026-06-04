package tui

import "strconv"

// fmtInt and fmtFloat1 are tiny formatters so markdown.go doesn't import "fmt"
// just for Sprintf calls. They are unexported and only used inside the tui
// package.
func fmtInt(i int) string     { return strconv.Itoa(i) }
func fmtFloat1(f float64) string { return strconv.FormatFloat(f, 'f', 1, 64) }
