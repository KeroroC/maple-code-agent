// pkg/tool/command_tool_test.go
package tool

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRunCommand(t *testing.T) {
	root := t.TempDir()
	tool := &RunCommandTool{Root: root}

	t.Run("allowed ls", func(t *testing.T) {
		r := tool.Execute(mustJSON(map[string]string{"command": "ls"}))
		if !r.OK {
			t.Fatalf("expected success: %v", r.Error)
		}
		var meta commandMetadata
		json.Unmarshal(r.Metadata, &meta)
		if meta.ExitCode != 0 {
			t.Fatalf("exit code %d", meta.ExitCode)
		}
	})

	t.Run("disallowed command", func(t *testing.T) {
		r := tool.Execute(mustJSON(map[string]string{"command": "rm -rf /"}))
		if r.OK {
			t.Fatal("expected failure")
		}
		if !strings.Contains(r.Error, "not allowed") {
			t.Fatalf("error %q should mention not allowed", r.Error)
		}
	})
}
