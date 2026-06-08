package tool

import "encoding/json"

// ParamSchema describes a JSON Schema for tool parameters.
type ParamSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

// Property is a single property in a JSON Schema.
type Property struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// ToolMeta holds the immutable metadata for a registered tool.
type ToolMeta struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Params      ParamSchema `json:"params"`
}

// Tool is the interface every built-in tool must implement.
type Tool interface {
	// Meta returns the tool's immutable metadata.
	Meta() ToolMeta
	// Execute runs the tool with the given JSON arguments and returns a ToolResult.
	Execute(args json.RawMessage) ToolResult
}
