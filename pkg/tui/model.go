// Package tui 实现了 MapleCode 基于 Bubble Tea 的终端 UI。
//
// Model 类型拥有：
//   - 对话消息列表（每条消息有自己的状态机）
//   - 当前会话元数据
//   - 活动 provider.Streamer 的引用
//
// 纯逻辑操作（handleChunk、userSubmitted、toggleThinking、clearConversation、
// renderStatusBar）被暴露，以便单元测试可以在不启动真实 bubbletea 程序的情况下驱动模型。
// 面向 bubbletea 的 Init/Update/View 粘合层位于 update.go。
package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"maplecode/pkg/provider"
	"maplecode/pkg/session"
)

// 用于渲染消息的角色样式。
var (
	roleUserStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")) // cyan
	roleAssistantStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2")) // green
	roleSystemStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3")) // yellow
	thinkingStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))            // gray
	errorStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))            // red
)

// msgState 跟踪单条消息在其生命周期中的位置。
type msgState int

const (
	statePending msgState = iota
	stateStreaming
	stateDone
	stateInterrupted
	stateError
)

func (s msgState) String() string {
	switch s {
	case statePending:
		return "pending"
	case stateStreaming:
		return "streaming"
	case stateDone:
		return "done"
	case stateInterrupted:
		return "interrupted"
	case stateError:
		return "error"
	}
	return "unknown"
}

// toolStatus 跟踪进行中或已完成的工具执行以供显示。
type toolStatus struct {
	name    string
	done    bool
	failed  bool
	summary string
}

// message 是在对话区域中渲染的一条用户或助手轮次。
type message struct {
	role             string
	content          string
	thinking         string
	thinkingTokens   int // approximate token count (chars/4)
	thinkingExpanded bool
	state            msgState
	toolCall         *toolStatus // non-nil when this message has a tool call
}

// Model 是顶层 TUI 模型。它被导出是因为 main.go 和 bubbletea 程序必须构造它，
// 但只有本包中定义的方法应该用于修改它。
type Model struct {
	streamer provider.Streamer
	session  *session.Session

	model           string // current model name, used in the status bar
	messages        []message
	thinkingEnabled bool
	thinkingBudget  int
}

// newTestModel 构建一个带有脚本化流式传输器且无后端会话的 Model。
// 用于单元测试；生产代码调用 New。
func newTestModel() *Model {
	return &Model{
		streamer: provider.NewScriptedStreamer(nil),
		model:    "test-model",
	}
}

// New 构建绑定到真实会话和流式传输器的 Model。
func New(s *session.Session, streamer provider.Streamer, modelName string, thinkingEnabled bool, thinkingBudget int) *Model {
	m := &Model{
		streamer:        streamer,
		session:         s,
		model:           modelName,
		thinkingEnabled: thinkingEnabled,
		thinkingBudget:  thinkingBudget,
		messages:        []message{},
	}
	// 如果会话有之前的轮次，从它们填充消息列表。
	for _, t := range s.Snapshot() {
		m.messages = append(m.messages, message{role: t.Role, content: t.Content, state: stateDone})
	}
	m.hydrateToolStatus(s)
	return m
}

// Snapshot 将内存中的消息投影为 session.Turn 值返回，
// 以便 tui 包外部的调用者（例如 main.go 中的 bubbletea View）可以遍历它们而不依赖内部类型。
func (m *Model) Snapshot() []session.Turn {
	out := make([]session.Turn, len(m.messages))
	for i, msg := range m.messages {
		out[i] = session.Turn{Role: msg.role, Content: msg.content}
	}
	return out
}

// UserSubmitted 将给定文本作为用户轮次追加，并创建一个流式助手消息占位符。
// 它不会实际调用流式传输器；调用者（bubbletea Update 路径）应在此返回后异步启动流。
func (m *Model) UserSubmitted(text string) {
	m.messages = append(m.messages, message{role: "user", content: text, state: stateDone})
	if m.session != nil {
		_ = m.session.Append(session.Turn{Role: "user", Content: text})
	}
	m.messages = append(m.messages, message{role: "assistant", content: "", state: stateStreaming})
}

// HandleChunk 处理流式传输器发出的单个 Chunk，并相应地更新当前（最后一个）助手消息。
// 取消错误进入 stateInterrupted 状态；任何其他错误进入 stateError 状态。
func (m *Model) HandleChunk(c provider.Chunk) {
	if len(m.messages) == 0 {
		return
	}
	last := &m.messages[len(m.messages)-1]
	switch v := c.(type) {
	case provider.TextDelta:
		last.content += v.Text
	case provider.ThinkingDelta:
		last.thinking += v.Text
		last.thinkingTokens = len(last.thinking) / 4
	case provider.Done:
		last.state = stateDone
		if m.session != nil {
			_ = m.session.Append(session.Turn{Role: "assistant", Content: last.content})
		}
	case provider.ToolCallDelta:
		last.toolCall = &toolStatus{name: v.ToolName, done: false}
	case provider.StreamError:
		if errors.Is(v.Err, provider.ErrCanceled) {
			last.state = stateInterrupted
			if m.session != nil {
				_ = m.session.Append(session.Turn{Role: "assistant", Content: last.content, Interrupted: true})
			}
		} else {
			last.state = stateError
		}
	}
}

// SetToolResult 将具有给定名称的最近工具调用标记为完成或失败。
func (m *Model) SetToolResult(toolName string, ok bool, summary string) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].toolCall != nil && m.messages[i].toolCall.name == toolName && !m.messages[i].toolCall.done {
			m.messages[i].toolCall.done = true
			m.messages[i].toolCall.failed = !ok
			m.messages[i].toolCall.summary = summary
			return
		}
	}
}

// AppendSystemMessage 向对话区域添加合成的系统消息。
func (m *Model) AppendSystemMessage(text string) {
	m.messages = append(m.messages, message{role: "system", content: text, state: stateDone})
}

// AppendSystemError 添加一条合成消息，在行内显示给定的错误文本。
// 当 /clear、/help 等命令出现问题时由 bubbletea 粘合层使用。
func (m *Model) AppendSystemError(text string) {
	m.messages = append(m.messages, message{role: "system", content: "[error] " + text, state: stateDone})
}

// toggleThinking 翻转第 i 条消息的思考展开标志。
func (m *Model) toggleThinking(i int) {
	if i < 0 || i >= len(m.messages) {
		return
	}
	m.messages[i].thinkingExpanded = !m.messages[i].thinkingExpanded
}

// SetSession 替换当前会话并从中重新加载消息。
// 用于 /resume 和 /compact，它们会替换底层会话文件。
func (m *Model) SetSession(s *session.Session) {
	m.session = s
	m.messages = []message{}
	for _, t := range s.Snapshot() {
		m.messages = append(m.messages, message{role: t.Role, content: t.Content, state: stateDone})
	}
	m.hydrateToolStatus(s)
}

// hydrateToolStatus 将会话中的工具结果与助手消息关联，
// 为每个工具结果在第一个未分配的助手消息上设置 toolCall 字段（按顺序匹配）。
func (m *Model) hydrateToolStatus(s *session.Session) {
	for _, tr := range s.ToolResults() {
		ok := true
		var result map[string]any
		if json.Unmarshal(tr.Result, &result) == nil {
			if v, exists := result["ok"]; exists {
				if b, isBool := v.(bool); isBool {
					ok = b
				}
			}
		}
		// 查找第一个没有 toolCall 的助手消息。
		for i := 0; i < len(m.messages); i++ {
			if m.messages[i].role == "assistant" && m.messages[i].toolCall == nil {
				m.messages[i].toolCall = &toolStatus{
					name:    tr.ToolName,
					done:    true,
					failed:  !ok,
					summary: tr.Summary,
				}
				break
			}
		}
	}
}

// clearConversation 清空内存中的消息列表并重置会话。
func (m *Model) clearConversation() {
	m.messages = []message{}
}

// RenderStatusBar 生成单行状态字符串。它包含当前模型名称，以便用户始终知道哪个后端处于活动状态。
func (m *Model) RenderStatusBar() string {
	var b strings.Builder
	fmt.Fprintf(&b, "model: %s", m.model)
	if m.thinkingEnabled {
		fmt.Fprintf(&b, "  thinking: %d", m.thinkingBudget)
	}
	return b.String()
}

// MessageView 是单条消息的只读快照，导出以便 main.go 可以在不依赖内部类型的情况下判断消息状态。
type MessageView struct {
	Role             string
	Content          string
	Thinking         string
	ThinkingTokens   int
	ThinkingExpanded bool
	State            string // "pending", "streaming", "done", "interrupted", "error"
}

// MessageViews 返回所有消息的快照，作为导出的 MessageView 值。
func (m *Model) MessageViews() []MessageView {
	out := make([]MessageView, len(m.messages))
	for i, msg := range m.messages {
		out[i] = MessageView{
			Role:             msg.role,
			Content:          msg.content,
			Thinking:         msg.thinking,
			ThinkingTokens:   msg.thinkingTokens,
			ThinkingExpanded: msg.thinkingExpanded,
			State:            msg.state.String(),
		}
	}
	return out
}

// LastMessageHasThinking 报告最后一条消息是否有思考内容。
func (m *Model) LastMessageHasThinking() bool {
	if len(m.messages) == 0 {
		return false
	}
	return m.messages[len(m.messages)-1].thinking != ""
}

// ToggleThinkingExported 翻转第 i 条消息的思考展开标志。
// 这是 toggleThinking 的导出版本，供 app 层使用。
func (m *Model) ToggleThinkingExported(i int) {
	m.toggleThinking(i)
}

// RenderMessage 将单条消息渲染为带样式的字符串。
// 对于已完成的助手消息，内容通过 glamour（markdown）渲染。
// 思考块根据消息状态显示为折叠或展开。
func (m *Model) RenderMessage(i int) string {
	if i < 0 || i >= len(m.messages) {
		return ""
	}
	msg := &m.messages[i]

	var b strings.Builder

	// 角色前缀。
	roleLabel := msg.role
	switch msg.role {
	case "user":
		roleLabel = roleUserStyle.Render("[You]")
	case "assistant":
		roleLabel = roleAssistantStyle.Render("[Assistant]")
	case "system":
		roleLabel = roleSystemStyle.Render("[System]")
	default:
		roleLabel = fmt.Sprintf("[%s]", msg.role)
	}
	b.WriteString(roleLabel)
	b.WriteByte('\n')

	// 思考块（如果有）。
	if msg.thinking != "" {
		if msg.thinkingExpanded {
			b.WriteString(thinkingStyle.Render(msg.thinking))
			b.WriteString("\n\n")
		} else {
			b.WriteString(thinkingStyle.Render(formatThinkingLine(msg.thinkingTokens)))
			b.WriteByte('\n')
		}
	}

	// 内容。
	content := msg.content
	if msg.state == stateDone && msg.role == "assistant" && content != "" {
		rendered := renderMarkdown(content)
		if rendered != "" {
			content = rendered
		}
	}

	// 状态指示器。
	switch msg.state {
	case stateStreaming:
		if content == "" {
			content = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("...")
		}
	case stateInterrupted:
		if content != "" {
			content += " "
		}
		content += lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("[interrupted]")
	case stateError:
		content = errorStyle.Render("[error] ") + content
	}

	b.WriteString(content)

	// 工具状态行。
	if msg.toolCall != nil {
		b.WriteString("\n")
		if msg.toolCall.done {
			if msg.toolCall.failed {
				b.WriteString(errorStyle.Render(fmt.Sprintf("tool: %s ... failed", msg.toolCall.name)))
			} else {
				b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(
					fmt.Sprintf("tool: %s ... done", msg.toolCall.name)))
			}
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render(
				fmt.Sprintf("tool: %s ... running", msg.toolCall.name)))
		}
	}

	return b.String()
}

// RenderMessages 将所有消息渲染为适合视口的单个字符串。
func (m *Model) RenderMessages() string {
	var b strings.Builder
	for i := range m.messages {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(m.RenderMessage(i))
	}
	return b.String()
}
