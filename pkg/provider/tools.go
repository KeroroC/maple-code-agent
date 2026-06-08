// Package provider defines the Streamer interface and the chunk types that flow through it.
// Each implementation (anthropic, openai, openai-compatible) is responsible for translating
// provider-specific events into the normalized Chunk values defined here.
package provider

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go"
)

// ToolMeta mirrors tool.ToolMeta to avoid circular imports.
type ToolMeta struct {
	Name        string
	Description string
	InputSchema map[string]any // raw JSON Schema
}

// ToAnthropicTools converts tool metadata into Anthropic API tool definitions.
func ToAnthropicTools(metas []ToolMeta) []anthropic.ToolParam {
	out := make([]anthropic.ToolParam, len(metas))
	for i, m := range metas {
		out[i] = anthropic.ToolParam{
			Name:        m.Name,
			Description: anthropic.String(m.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: m.InputSchema,
			},
		}
	}
	return out
}

// ToOpenAITools converts tool metadata into OpenAI API tool definitions.
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
