package tool

import "encoding/json"

const MaxResultSize = 64 * 1024 // 64 KB

// ToolResult is the structured outcome of a tool execution.
type ToolResult struct {
	OK        bool            `json:"ok"`
	Content   string          `json:"content,omitempty"`
	Error     string          `json:"error,omitempty"`
	Truncated bool            `json:"truncated,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

// LimitResult truncates content if it exceeds MaxResultSize.
func LimitResult(r ToolResult) ToolResult {
	if len(r.Content) > MaxResultSize {
		r.Content = r.Content[:MaxResultSize]
		r.Truncated = true
	}
	return r
}
