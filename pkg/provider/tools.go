// Package provider defines the Streamer interface and the chunk types that flow through it.
// Each implementation (anthropic, openai, openai-compatible) is responsible for translating
// provider-specific events into the normalized Chunk values defined here.
package provider

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go"
)

// ToolMeta 镜像 tool.ToolMeta 以避免循环导入。
type ToolMeta struct {
	Name        string
	Description string
	InputSchema map[string]any // raw JSON Schema
}

// ToAnthropicTools 将工具元数据转换为 Anthropic API 工具定义。
func ToAnthropicTools(metas []ToolMeta) []anthropic.ToolParam {
	out := make([]anthropic.ToolParam, len(metas))
	for i, m := range metas {
		// 提取 schema 中的 properties 部分
		var properties any
		if props, ok := m.InputSchema["properties"]; ok {
			properties = props
		}
		// 提取 required 字段
		var required []string
		if req, ok := m.InputSchema["required"]; ok {
			if reqSlice, ok := req.([]string); ok {
				required = reqSlice
			}
		}
		out[i] = anthropic.ToolParam{
			Name:        m.Name,
			Description: anthropic.String(m.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: properties,
				Required:   required,
			},
		}
	}
	return out
}

// ToOpenAITools 将工具元数据转换为 OpenAI API 工具定义。
func ToOpenAITools(metas []ToolMeta) []openai.ChatCompletionToolParam {
	out := make([]openai.ChatCompletionToolParam, len(metas))
	for i, m := range metas {
		out[i] = openai.ChatCompletionToolParam{
			Type: "function",
			Function: openai.FunctionDefinitionParam{
				Name:        m.Name,
				Description: openai.String(m.Description),
				Parameters:  openai.FunctionParameters(m.InputSchema),
			},
		}
	}
	return out
}
