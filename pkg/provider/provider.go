// Package provider 定义了 Streamer 接口和流经它的 chunk 类型。
// 每个实现（anthropic、openai、openai-compatible）负责将特定于提供者的事件
// 转换为此处定义的标准化 Chunk 值。
package provider

import (
	"context"
	"encoding/json"
	"errors"
)

// Turn 是对话中的单条消息。Role 为 "user" 或 "assistant"。
// 系统指令单独传递给 Stream，不出现在 turns 切片中。
type Turn struct {
	Role    string
	Content string
}

// Chunk 是密封接口，由 Stream 通道上可能发出的每个事件实现。
// 调用者必须使用类型 switch 来提取具体载荷。
type Chunk interface {
	chunk()
}

// TextDelta 是助手最终回答的片段。
type TextDelta struct {
	Text string
}

// ThinkingDelta 是助手思维链的片段。
// 仅在启用扩展思考时发出。
type ThinkingDelta struct {
	Text string
}

// Usage 是提供者在流结束时报告的 token 用量统计。
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Done 表示提供者已成功完成流式传输。它携带最终的 Usage，以便调用者更新状态栏和统计。
type Done struct {
	Usage Usage
}

// StreamError 包装任何非成功状态：网络失败、认证失败、取消、上下文溢出等。
// 当失败模式被识别时，嵌入的错误是包级哨兵错误之一，否则是原始的提供者错误。
type StreamError struct {
	Err error
}

// ToolCallDelta 表示模型正在请求调用工具。
// 调用者应使用给定的 JSON 参数执行指定的工具。
type ToolCallDelta struct {
	CallID   string          // 提供者特定的调用 ID
	ToolName string          // 注册的工具名称（snake_case）
	ArgsJSON json.RawMessage // 完整的 JSON 参数
}

func (TextDelta) chunk()     {}
func (ThinkingDelta) chunk() {}
func (Done) chunk()          {}
func (StreamError) chunk()   {}
func (ToolCallDelta) chunk() {}

// 哨兵错误。使用 errors.Is 来分类流失败。
var (
	ErrCanceled      = errors.New("stream canceled")
	ErrContextLength = errors.New("context length exceeded")
	ErrAuth          = errors.New("authentication failed")
	ErrRateLimit     = errors.New("rate limited")
)

// Streamer 是每个提供者后端必须实现的接口。它打开一个流式补全请求
// 并返回一个 Chunk 值的通道。当流结束（成功、错误或上下文取消）时，
// 实现会关闭该通道。
//
// 实现必须尊重 ctx 取消：当 ctx.Done() 触发时，发出包装 ErrCanceled 的
// StreamError，然后关闭通道。
type Streamer interface {
	Stream(ctx context.Context, system string, turns []Turn) (<-chan Chunk, error)
}

// ThinkingConfig 传递给提供者构造函数，以便每个流式传输器知道是否启用扩展思考以及预算多少思考 token。
type ThinkingConfig struct {
	Enabled      bool
	BudgetTokens int
}
