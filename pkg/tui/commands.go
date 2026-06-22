package tui

import (
	"fmt"
	"strconv"
	"strings"
)

// Command 是解析后的斜杠命令。Kind 名称与 Model 方法 1:1 映射。
type Command struct {
	Kind string
	Args string
}

// ParseCommand 接受用户输入的原始文本（可能以斜杠开头也可能不以斜杠开头）
// 并返回 (kind, args, ok)。ok 仅在输入根本不是命令时为 false。
// 未知命令名称返回 ("unknown", input, true)，以便调用者可以渲染友好的"未知命令"错误。
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

// HelpText 返回用户输入 /help 时渲染的多行帮助文本。
func HelpText() string {
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

// ExecuteCommand 将解析后的命令应用到模型。对于格式错误的参数或未知命令返回错误；
// 调用者应在对话区域中内联显示错误。
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
		// 这些需要访问文件系统/程序控制；推迟到 bubbletea Update 路径，使模型与程序解耦。
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
