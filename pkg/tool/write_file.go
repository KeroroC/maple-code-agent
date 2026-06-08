package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type WriteFileTool struct {
	Root string
}

func (t *WriteFileTool) Meta() ToolMeta {
	return ToolMeta{
		Name:        "write_file",
		Description: "Write content to a file within the workspace. Overwrites existing files.",
		Params: ParamSchema{
			Type: "object",
			Properties: map[string]Property{
				"path":    {Type: "string", Description: "Relative path to the file"},
				"content": {Type: "string", Description: "Content to write"},
			},
			Required: []string{"path", "content"},
		},
	}
}

type writeFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (t *WriteFileTool) Execute(args json.RawMessage) ToolResult {
	var a writeFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ToolResult{OK: false, Error: fmt.Sprintf("invalid args: %v", err)}
	}
	resolved, err := WorkspacePath(t.Root, a.Path)
	if err != nil {
		return ToolResult{OK: false, Error: err.Error()}
	}
	parent := filepath.Dir(resolved)
	if _, err := os.Stat(parent); os.IsNotExist(err) {
		return ToolResult{OK: false, Error: fmt.Sprintf("parent directory does not exist: %s", parent)}
	}
	if err := os.WriteFile(resolved, []byte(a.Content), 0o644); err != nil {
		return ToolResult{OK: false, Error: fmt.Sprintf("write failed: %v", err)}
	}
	return ToolResult{OK: true, Content: fmt.Sprintf("wrote %d bytes to %s", len(a.Content), a.Path)}
}
