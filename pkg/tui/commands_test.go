package tui

import (
	"strings"
	"testing"
	"time"

	"maplecode/pkg/session"
)

func TestParseCommand_RecognizesAllBuiltinCommands(t *testing.T) {
	cases := []struct {
		in   string
		kind string
		args string
		ok   bool
	}{
		{"/clear", "clear", "", true},
		{"/resume 20260604-103045-hello", "resume", "20260604-103045-hello", true},
		{"/compact", "compact", "", true},
		{"/thinking on", "thinking", "on", true},
		{"/thinking off", "thinking", "off", true},
		{"/thinking 2048", "thinking", "2048", true},
		{"/model claude-opus-4-6", "model", "claude-opus-4-6", true},
		{"/help", "help", "", true},
		{"/exit", "exit", "", true},
		{"/foo", "unknown", "/foo", true},
		{"hello", "", "", false}, // not a command at all
	}
	for _, c := range cases {
		kind, args, ok := ParseCommand(c.in)
		if ok != c.ok || kind != c.kind || args != c.args {
			t.Errorf("ParseCommand(%q) = (%q, %q, %v), want (%q, %q, %v)",
				c.in, kind, args, ok, c.kind, c.args, c.ok)
		}
	}
}

func TestHelpText_ContainsAllCommandNames(t *testing.T) {
	help := HelpText()
	for _, name := range []string{"clear", "resume", "compact", "thinking", "model", "help", "exit"} {
		if !strings.Contains(help, name) {
			t.Errorf("help text missing %q, got:\n%s", name, help)
		}
	}
}

func TestModel_ClearCommandClearsMessagesAndStartsNewSession(t *testing.T) {
	dir := t.TempDir()
	sess, err := session.New(dir+"/s.jsonl", session.Metadata{ID: "x", Created: time.Now().UTC(), Protocol: "openai", Model: "gpt"})
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	defer sess.Close()
	m := New(sess, nil, "gpt", false, 0)
	m.UserSubmitted("hi")
	m.HandleChunk(nil) // no-op for nil, but we need at least one done
	// Use real text/done chunks
	m.messages[len(m.messages)-1].state = stateDone

	if len(m.messages) == 0 {
		t.Fatal("setup: messages should not be empty")
	}
	if err := m.ExecuteCommand(parseCommandOrFatal(t, "/clear")); err != nil {
		t.Fatalf("executeCommand: %v", err)
	}
	if len(m.messages) != 0 {
		t.Errorf("after /clear, len(messages) = %d, want 0", len(m.messages))
	}
}

func TestModel_ThinkingCommand_TogglesEnabledAndBudget(t *testing.T) {
	dir := t.TempDir()
	sess, _ := session.New(dir+"/s.jsonl", session.Metadata{ID: "x", Created: time.Now().UTC(), Protocol: "anthropic", Model: "m"})
	defer sess.Close()
	m := New(sess, nil, "m", false, 0)

	if err := m.ExecuteCommand(parseCommandOrFatal(t, "/thinking on")); err != nil {
		t.Fatalf("thinking on: %v", err)
	}
	if !m.thinkingEnabled {
		t.Error("/thinking on did not set thinkingEnabled")
	}
	if err := m.ExecuteCommand(parseCommandOrFatal(t, "/thinking 8192")); err != nil {
		t.Fatalf("thinking 8192: %v", err)
	}
	if m.thinkingBudget != 8192 {
		t.Errorf("thinkingBudget = %d, want 8192", m.thinkingBudget)
	}
	if err := m.ExecuteCommand(parseCommandOrFatal(t, "/thinking off")); err != nil {
		t.Fatalf("thinking off: %v", err)
	}
	if m.thinkingEnabled {
		t.Error("/thinking off did not clear thinkingEnabled")
	}
}

func TestModel_ModelCommand_ChangesModelName(t *testing.T) {
	dir := t.TempDir()
	sess, _ := session.New(dir+"/s.jsonl", session.Metadata{ID: "x", Created: time.Now().UTC(), Protocol: "openai", Model: "gpt-4"})
	defer sess.Close()
	m := New(sess, nil, "gpt-4", false, 0)

	if err := m.ExecuteCommand(parseCommandOrFatal(t, "/model gpt-4o")); err != nil {
		t.Fatalf("model: %v", err)
	}
	if m.model != "gpt-4o" {
		t.Errorf("model = %q, want gpt-4o", m.model)
	}
}

func TestModel_UnknownCommand_RecordsError(t *testing.T) {
	dir := t.TempDir()
	sess, _ := session.New(dir+"/s.jsonl", session.Metadata{ID: "x", Created: time.Now().UTC(), Protocol: "openai", Model: "m"})
	defer sess.Close()
	m := New(sess, nil, "m", false, 0)

	err := m.ExecuteCommand(parseCommandOrFatal(t, "/foo"))
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("error should mention 'unknown command', got: %v", err)
	}
}

// parseCommandOrFatal is a tiny helper that turns a parse failure into a t.Fatal
// so the test can read like "act on this command" without per-case boilerplate.
func parseCommandOrFatal(t *testing.T, in string) Command {
	t.Helper()
	kind, args, ok := ParseCommand(in)
	if !ok {
		t.Fatalf("ParseCommand(%q) failed", in)
	}
	return Command{Kind: kind, Args: args}
}
