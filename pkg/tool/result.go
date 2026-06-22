package tool

import "encoding/json"

const MaxResultSize = 64 * 1024 // 64 KB

// ToolResult 是工具执行的结构化结果。
type ToolResult struct {
	OK        bool            `json:"ok"`
	Content   string          `json:"content,omitempty"`
	Error     string          `json:"error,omitempty"`
	Truncated bool            `json:"truncated,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

// LimitResult 如果内容超过 MaxResultSize 则进行截断。
func LimitResult(r ToolResult) ToolResult {
	if len(r.Content) > MaxResultSize {
		r.Content = r.Content[:MaxResultSize]
		r.Truncated = true
	}
	return r
}
