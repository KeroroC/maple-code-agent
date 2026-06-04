package tui

import (
	"strings"
	"testing"

	"maplecode/pkg/provider"
)

// Test that submitting a user turn transitions the model into the streaming state
// and that a TextDelta followed by Done drives the message to "done".
func TestModel_UserSubmitThenStreamDrivesToDone(t *testing.T) {
	m := newTestModel()
	m.UserSubmitted("hello")

	if len(m.messages) != 2 {
		t.Fatalf("expected 2 messages (user + assistant placeholder), got %d", len(m.messages))
	}
	if m.messages[1].state != stateStreaming {
		t.Errorf("assistant message state = %v, want %v", m.messages[1].state, stateStreaming)
	}

	m.HandleChunk(provider.TextDelta{Text: "hi "})
	m.HandleChunk(provider.TextDelta{Text: "there"})
	m.HandleChunk(provider.Done{Usage: provider.Usage{InputTokens: 1, OutputTokens: 2}})

	if m.messages[1].state != stateDone {
		t.Errorf("assistant message state = %v, want %v", m.messages[1].state, stateDone)
	}
	if m.messages[1].content != "hi there" {
		t.Errorf("assistant content = %q, want %q", m.messages[1].content, "hi there")
	}
}

// Test that a StreamError with ErrCanceled transitions to "interrupted" (not "error").
func TestModel_StreamCanceledTransitionsToInterrupted(t *testing.T) {
	m := newTestModel()
	m.UserSubmitted("hello")
	m.HandleChunk(provider.TextDelta{Text: "partial "})
	m.HandleChunk(provider.StreamError{Err: provider.ErrCanceled})

	if m.messages[1].state != stateInterrupted {
		t.Errorf("state = %v, want %v", m.messages[1].state, stateInterrupted)
	}
	if m.messages[1].content != "partial " {
		t.Errorf("interrupted content should be preserved, got %q", m.messages[1].content)
	}
}

// Test that a StreamError with anything else transitions to "error".
func TestModel_StreamErrorTransitionsToError(t *testing.T) {
	m := newTestModel()
	m.UserSubmitted("hello")
	m.HandleChunk(provider.StreamError{Err: provider.ErrAuth})

	if m.messages[1].state != stateError {
		t.Errorf("state = %v, want %v", m.messages[1].state, stateError)
	}
}

// Thinking deltas accumulate but do not appear in the content text.
func TestModel_ThinkingDeltasAccumulateSeparately(t *testing.T) {
	m := newTestModel()
	m.UserSubmitted("hello")
	m.HandleChunk(provider.ThinkingDelta{Text: "let me "})
	m.HandleChunk(provider.ThinkingDelta{Text: "think"})
	m.HandleChunk(provider.TextDelta{Text: "answer"})
	m.HandleChunk(provider.Done{})

	if m.messages[1].content != "answer" {
		t.Errorf("content = %q, want %q", m.messages[1].content, "answer")
	}
	if m.messages[1].thinking != "let me think" {
		t.Errorf("thinking = %q, want %q", m.messages[1].thinking, "let me think")
	}
	if m.messages[1].thinkingTokens != 3 {
		t.Errorf("thinkingTokens = %d, want 3 (12 chars / 4)", m.messages[1].thinkingTokens)
	}
}

// Tab on a done message with thinking toggles expanded state.
func TestModel_TabTogglesThinkingExpansion(t *testing.T) {
	m := newTestModel()
	m.UserSubmitted("hello")
	m.HandleChunk(provider.ThinkingDelta{Text: "x"})
	m.HandleChunk(provider.TextDelta{Text: "y"})
	m.HandleChunk(provider.Done{})

	if m.messages[1].thinkingExpanded {
		t.Fatal("thinking should start collapsed")
	}
	m.toggleThinking(1)
	if !m.messages[1].thinkingExpanded {
		t.Error("after toggle, thinking should be expanded")
	}
	m.toggleThinking(1)
	if m.messages[1].thinkingExpanded {
		t.Error("after second toggle, thinking should be collapsed again")
	}
}

// /clear resets the message list and starts a new session file path.
func TestModel_ClearCommandResetsMessages(t *testing.T) {
	m := newTestModel()
	m.UserSubmitted("hello")
	m.HandleChunk(provider.TextDelta{Text: "world"})
	m.HandleChunk(provider.Done{})

	before := len(m.messages)
	if before < 2 {
		t.Fatalf("setup: need >= 2 messages, got %d", before)
	}
	m.clearConversation()
	if len(m.messages) != 0 {
		t.Errorf("after clear, len(messages) = %d, want 0", len(m.messages))
	}
}

// Status bar format includes the current model name.
func TestModel_StatusBarIncludesModelName(t *testing.T) {
	m := newTestModel()
	m.model = "claude-test"
	out := m.RenderStatusBar()
	if !strings.Contains(out, "claude-test") {
		t.Errorf("status bar missing model name: %q", out)
	}
}
