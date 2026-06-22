package tool

import "encoding/json"

// ParamSchema 描述工具参数的 JSON Schema。
type ParamSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

// Property 是 JSON Schema 中的单个属性。
type Property struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// ToolMeta 持有已注册工具的不可变元数据。
type ToolMeta struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Params      ParamSchema `json:"params"`
}

// Tool 是每个内置工具必须实现的接口。
type Tool interface {
	// Meta 返回工具的不可变元数据。
	Meta() ToolMeta
	// Execute 使用给定的 JSON 参数运行工具并返回 ToolResult。
	Execute(args json.RawMessage) ToolResult
}
