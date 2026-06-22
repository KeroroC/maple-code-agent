# CLAUDE.md

本文件为 Claude Code (claude.ai/code) 在此代码仓库中工作时提供指导。

## 项目概述

MapleCode 是一个终端 AI 编程助手（类似 Claude Code），使用 Go 语言编写。第一阶段实现"对话内核"——基于 TUI 的多轮聊天，支持 LLM 后端、流式响应、会话持久化和 Markdown 渲染。工具使用和文件操作不在本阶段范围内。

## 命令

```bash
# 构建
go build ./...

# 运行
go run ./cmd/maplecode
go run ./cmd/maplecode --config <路径> --debug --resume <会话ID>

# 运行所有测试
go test -race ./...

# 测试单个包
go test ./pkg/provider/
go test -run TestAnthropicSSE ./pkg/provider/

# 静态检查
go vet ./...
```

CLI 参数：`--config`（默认 `~/.maplecode/config.yaml`），`--debug`，`--resume <会话ID>`

## 架构

四层设计：

1. **Provider** (`pkg/provider/`) — `Streamer` 接口，配合 `Chunk` 密封接口（可辨识联合：`TextDelta`、`ThinkingDelta`、`Done`、`StreamError`）。实现：`AnthropicStreamer`、`OpenAIStreamer`（通过自定义 base URL 也支持 OpenAI 兼容协议）。工厂函数在 `factory.go` 中根据协议分发。哨兵错误：`ErrCanceled`、`ErrContextLength`、`ErrAuth`、`ErrRateLimit`。

2. **Session** (`pkg/session/`) — 基于 JSONL 的持久化。`Session` 结构体使用互斥锁保护追加操作。`Compact()` 对对话进行摘要并创建新会话。会话存储在 `~/.maplecode/sessions/<时间戳>-<标识>.jsonl`。

3. **TUI** (`pkg/tui/`) — Bubble Tea 模型。每条消息的状态机：`pending → streaming → done | interrupted | error`。斜杠命令：`/clear`、`/compact`、`/thinking`、`/model`、`/help`、`/quit`、`/sessions`。通过 glamour 渲染 Markdown（仅针对已完成的消息）。

4. **Application** (`cmd/maplecode/`) — `app` 结构体嵌入 `*tui.Model`，串联配置/日志/Provider/会话。`startStream()` 启动 goroutine 驱动流式传输，通过 `program.Send()` 发送 chunk。`chunkSender` 接口封装 `*tea.Program` 以支持可测试性。

数据流：用户输入 → `onKey()` → `UserSubmitted()` → `startStream()` → goroutine 读取 `streamer.Stream()` 通道 → 发送 `chunkMsg` → `HandleChunk()` 更新状态 → `View()` 重新渲染。

## 测试模式

- 使用标准 `testing` 包，无外部测试框架
- 表驱动测试用于验证/解析
- `httptest.NewServer` 模拟 Provider SSE
- `t.TempDir()` 隔离文件系统
- 手动 fake：`fakeSender`、`scriptedStreamer`、`erroringStreamer`
- 测试文件与源码同目录，后缀 `_test.go`

## 开发流程

收到新想法或功能需求时：

1. 提出澄清问题以细化需求并探索边界情况
2. 在 `docs/` 中创建三个文档：

**spec.md** — 解决什么问题、提供什么能力、什么是范围外、设计骨架。不要包含函数名、参数名、默认值或错误文本（这些是实现细节，容易过时）。

**tasks.md** — 5-15 个任务，每个任务可在一次会话中完成。每个任务列出：受影响的文件、依赖的任务、参考位置（可用精确的函数/行号）。最后两个任务必须是"接入主流程"和"端到端验证"。

**checklist.md** — 每个条目必须可检查、可观察。不要写"实现完成"或"质量良好"。使用 spec 中的具体值作为验收标准。示例："`grep -r X` 返回 ≥3 条结果"、"输入 Y 显示输出 Z"。至少包含一个端到端验收条目。
