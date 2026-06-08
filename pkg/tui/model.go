// Package tui implements the Bubble Tea-based terminal UI for MapleCode.
//
// The Model type owns:
//   - the list of conversation messages (each with its own state machine)
//   - the current session metadata
//   - a reference to the active provider.Streamer
//
// Pure-logic operations (handleChunk, userSubmitted, toggleThinking, clearConversation,
// renderStatusBar) are exposed so unit tests can drive the model without spinning up
// a real bubbletea program. The bubbletea-facing Init/Update/View glue lives in update.go.
package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"maplecode/pkg/provider"
	"maplecode/pkg/session"
)

// Role styles for rendering messages.
var (
	roleUserStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))  // cyan
	roleAssistantStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))  // green
	roleSystemStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3"))  // yellow
	thinkingStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))             // gray
	errorStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))             // red
)

// msgState tracks where a single message is in its lifecycle.
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

// toolStatus tracks an in-progress or completed tool execution for display.
type toolStatus struct {
	name    string
	done    bool
	failed  bool
	summary string
}

// message is one user or assistant turn rendered in the conversation area.
type message struct {
	role             string
	content          string
	thinking         string
	thinkingTokens   int // approximate token count (chars/4)
	thinkingExpanded bool
	state            msgState
	toolCall         *toolStatus // non-nil when this message has a tool call
}

// Model is the top-level TUI model. It is exported because main.go and the
// bubbletea program must construct it, but only the methods defined in this
// package should be used to mutate it.
type Model struct {
	streamer provider.Streamer
	session  *session.Session

	model    string // current model name, used in the status bar
	messages []message
	thinkingEnabled bool
	thinkingBudget  int
}

// newTestModel builds a Model with a scripted streamer and no backing session.
// Used by unit tests; production code calls New instead.
func newTestModel() *Model {
	return &Model{
		streamer: provider.NewScriptedStreamer(nil),
		model:    "test-model",
	}
}

// New builds a Model bound to a real session and streamer.
func New(s *session.Session, streamer provider.Streamer, modelName string, thinkingEnabled bool, thinkingBudget int) *Model {
	m := &Model{
		streamer:        streamer,
		session:         s,
		model:           modelName,
		thinkingEnabled: thinkingEnabled,
		thinkingBudget:  thinkingBudget,
		messages:        []message{},
	}
	// If the session has prior turns, hydrate the message list from them.
	for _, t := range s.Snapshot() {
		m.messages = append(m.messages, message{role: t.Role, content: t.Content, state: stateDone})
	}
	return m
}

// Snapshot returns the in-memory messages projected as session.Turn values, so
// callers outside the tui package (e.g. the bubbletea View in main.go) can
// iterate them without depending on internal types.
func (m *Model) Snapshot() []session.Turn {
	out := make([]session.Turn, len(m.messages))
	for i, msg := range m.messages {
		out[i] = session.Turn{Role: msg.role, Content: msg.content}
	}
	return out
}

// UserSubmitted appends the given text as a user turn and creates a streaming
// assistant message placeholder. It does NOT actually call the streamer; the
// caller (the bubbletea Update path) is expected to kick off the stream
// asynchronously after this returns.
func (m *Model) UserSubmitted(text string) {
	m.messages = append(m.messages, message{role: "user", content: text, state: stateDone})
	if m.session != nil {
		_ = m.session.Append(session.Turn{Role: "user", Content: text})
	}
	m.messages = append(m.messages, message{role: "assistant", content: "", state: stateStreaming})
}

// HandleChunk processes a single Chunk emitted by the streamer and updates the
// current (last) assistant message accordingly. Canceled errors land in state
// stateInterrupted; any other error lands in stateError.
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

// SetToolResult marks the most recent tool call with the given name as done or failed.
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

// AppendSystemMessage adds a synthetic system message to the conversation area.
func (m *Model) AppendSystemMessage(text string) {
	m.messages = append(m.messages, message{role: "system", content: text, state: stateDone})
}

// AppendSystemError adds a synthetic message that displays the given error text
// inline. Used by the bubbletea glue when /clear / /help / etc surface a problem.
func (m *Model) AppendSystemError(text string) {
	m.messages = append(m.messages, message{role: "system", content: "[error] " + text, state: stateDone})
}

// toggleThinking flips the thinking expansion flag for the i-th message.
func (m *Model) toggleThinking(i int) {
	if i < 0 || i >= len(m.messages) {
		return
	}
	m.messages[i].thinkingExpanded = !m.messages[i].thinkingExpanded
}

// SetSession replaces the current session and reloads messages from it.
// Used by /resume and /compact which swap the underlying session file.
func (m *Model) SetSession(s *session.Session) {
	m.session = s
	m.messages = []message{}
	for _, t := range s.Snapshot() {
		m.messages = append(m.messages, message{role: t.Role, content: t.Content, state: stateDone})
	}
}

// clearConversation empties the in-memory message list and resets the session.
func (m *Model) clearConversation() {
	m.messages = []message{}
}

// RenderStatusBar produces the single-line status string. It includes the
// current model name so the user always knows which backend is active.
func (m *Model) RenderStatusBar() string {
	var b strings.Builder
	fmt.Fprintf(&b, "model: %s", m.model)
	if m.thinkingEnabled {
		fmt.Fprintf(&b, "  thinking: %d", m.thinkingBudget)
	}
	return b.String()
}

// MessageView is a read-only snapshot of a single message, exported so main.go
// can reason about message state without depending on internal types.
type MessageView struct {
	Role             string
	Content          string
	Thinking         string
	ThinkingTokens   int
	ThinkingExpanded bool
	State            string // "pending", "streaming", "done", "interrupted", "error"
}

// MessageViews returns a snapshot of all messages as exported MessageView values.
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

// LastMessageHasThinking reports whether the last message has thinking content.
func (m *Model) LastMessageHasThinking() bool {
	if len(m.messages) == 0 {
		return false
	}
	return m.messages[len(m.messages)-1].thinking != ""
}

// ToggleThinkingExported flips the thinking expansion flag for the i-th message.
// This is the exported version of toggleThinking for use by the app layer.
func (m *Model) ToggleThinkingExported(i int) {
	m.toggleThinking(i)
}

// RenderMessage renders a single message to a styled string.
// For done assistant messages, the content is rendered through glamour (markdown).
// Thinking blocks are shown collapsed or expanded based on the message state.
func (m *Model) RenderMessage(i int) string {
	if i < 0 || i >= len(m.messages) {
		return ""
	}
	msg := &m.messages[i]

	var b strings.Builder

	// Role prefix.
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

	// Thinking block (if any).
	if msg.thinking != "" {
		if msg.thinkingExpanded {
			b.WriteString(thinkingStyle.Render(msg.thinking))
			b.WriteString("\n\n")
		} else {
			b.WriteString(thinkingStyle.Render(formatThinkingLine(msg.thinkingTokens)))
			b.WriteByte('\n')
		}
	}

	// Content.
	content := msg.content
	if msg.state == stateDone && msg.role == "assistant" && content != "" {
		rendered := renderMarkdown(content)
		if rendered != "" {
			content = rendered
		}
	}

	// State indicators.
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

	// Tool status line.
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

// RenderMessages renders all messages into a single string suitable for a viewport.
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
