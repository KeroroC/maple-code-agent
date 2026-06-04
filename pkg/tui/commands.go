package tui

import (
	"fmt"
	"strconv"
	"strings"
)

// Command is a parsed slash command. Kind names map 1:1 to Model methods.
type Command struct {
	Kind string
	Args string
}

// ParseCommand takes the raw input the user typed (which may or may not start
// with a slash) and returns (kind, args, ok). ok is false only when the input
// is not a command at all. Unknown command names return ("unknown", input, true)
// so the caller can render a friendly "unknown command" error.
func ParseCommand(input string) (string, string, bool) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return "", "", false
	}
	head, args, _ := strings.Cut(input, " ")
	head = strings.ToLower(head)
	switch head {
	case "/clear", "/resume", "/compact", "/thinking", "/model", "/help", "/exit":
		return strings.TrimPrefix(head, "/"), strings.TrimSpace(args), true
	}
	return "unknown", input, true
}

// helpText returns the multi-line help text rendered when the user types /help.
func helpText() string {
	return strings.Join([]string{
		"Available commands:",
		"  /clear                    reset the current session and start a new one",
		"  /resume <id|timestamp>    load a previous session from ~/.maplecode/sessions/",
		"  /compact                  summarize the current session into a new one",
		"  /thinking on|off|<N>      toggle extended thinking or set its budget",
		"  /model <name>             switch the active model",
		"  /help                     show this help",
		"  /exit                     save and quit",
	}, "\n")
}

// ExecuteCommand applies a parsed command to the model. Returns an error for
// malformed arguments or unknown commands; the caller should display the error
// inline in the conversation area.
func (m *Model) ExecuteCommand(cmd Command) error {
	switch cmd.Kind {
	case "clear":
		m.clearConversation()
		return nil
	case "thinking":
		return m.applyThinking(cmd.Args)
	case "model":
		if cmd.Args == "" {
			return fmt.Errorf("usage: /model <name>")
		}
		m.model = cmd.Args
		return nil
	case "resume", "compact", "help", "exit":
		// These need access to filesystem / program control; defer them to the
		// bubbletea Update path so the model stays decoupled from the program.
		return fmt.Errorf("/%s is handled by the program, not the model", cmd.Kind)
	case "unknown":
		return fmt.Errorf("unknown command: %s. Type /help.", cmd.Args)
	}
	return fmt.Errorf("unhandled command kind: %q", cmd.Kind)
}

func (m *Model) applyThinking(args string) error {
	switch strings.ToLower(args) {
	case "on":
		m.thinkingEnabled = true
		if m.thinkingBudget == 0 {
			m.thinkingBudget = 4096
		}
		return nil
	case "off":
		m.thinkingEnabled = false
		return nil
	default:
		n, err := strconv.Atoi(args)
		if err != nil || n <= 0 {
			return fmt.Errorf("usage: /thinking on|off|<positive-int>")
		}
		m.thinkingEnabled = true
		m.thinkingBudget = n
		return nil
	}
}
