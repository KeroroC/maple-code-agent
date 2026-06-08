// pkg/tool/run_command.go
package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

var allowedCommands = map[string]bool{
	"go test":  true,
	"go build": true,
	"go vet":   true,
	"grep":     true,
	"find":     true,
	"ls":       true,
}

const commandTimeout = 30 * time.Second

type RunCommandTool struct {
	Root string
}

func (t *RunCommandTool) Meta() ToolMeta {
	return ToolMeta{
		Name:        "run_command",
		Description: "Execute a safe shell command in the workspace root.",
		Params: ParamSchema{
			Type: "object",
			Properties: map[string]Property{
				"command": {Type: "string", Description: "Command to execute"},
			},
			Required: []string{"command"},
		},
	}
}

type runCommandArgs struct {
	Command string `json:"command"`
}

type commandMetadata struct {
	ExitCode int  `json:"exit_code"`
	Timeout  bool `json:"timeout,omitempty"`
}

func (t *RunCommandTool) Execute(args json.RawMessage) ToolResult {
	var a runCommandArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ToolResult{OK: false, Error: fmt.Sprintf("invalid args: %v", err)}
	}
	cmd := strings.TrimSpace(a.Command)
	if !isAllowedCommand(cmd) {
		return ToolResult{OK: false, Error: fmt.Sprintf("command not allowed: %s", cmd)}
	}
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	parts := strings.Fields(cmd)
	c := exec.CommandContext(ctx, parts[0], parts[1:]...)
	c.Dir = t.Root
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := c.Run()
	exitCode := 0
	timedOut := false
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			timedOut = true
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	var b strings.Builder
	if stdout.Len() > 0 {
		b.WriteString(stdout.String())
	}
	if stderr.Len() > 0 {
		if b.Len() > 0 {
			b.WriteString("\n--- stderr ---\n")
		}
		b.WriteString(stderr.String())
	}
	meta, _ := json.Marshal(commandMetadata{ExitCode: exitCode, Timeout: timedOut})
	result := ToolResult{OK: exitCode == 0 && !timedOut, Content: b.String(), Metadata: meta}
	if timedOut {
		result.Error = "command timed out"
	}
	return LimitResult(result)
}

func isAllowedCommand(cmd string) bool {
	for prefix := range allowedCommands {
		if cmd == prefix || strings.HasPrefix(cmd, prefix+" ") {
			return true
		}
	}
	return false
}
