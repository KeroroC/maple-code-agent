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

	"maplecode/pkg/provider"
	"maplecode/pkg/session"
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

// message is one user or assistant turn rendered in the conversation area.
type message struct {
	role            string
	content         string
	thinking        string
	thinkingTokens  int // approximate token count (chars/4)
	thinkingExpanded bool
	state           msgState
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
