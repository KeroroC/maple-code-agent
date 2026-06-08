package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type EditFileTool struct {
	Root string
}

func (t *EditFileTool) Meta() ToolMeta {
	return ToolMeta{
		Name:        "edit_file",
		Description: "Replace a unique occurrence of old_text with new_text in a file.",
		Params: ParamSchema{
			Type: "object",
			Properties: map[string]Property{
				"path":     {Type: "string", Description: "Relative path to the file"},
				"old_text": {Type: "string", Description: "Text to find (must match exactly once)"},
				"new_text": {Type: "string", Description: "Replacement text"},
			},
			Required: []string{"path", "old_text", "new_text"},
		},
	}
}

type editFileArgs struct {
	Path    string `json:"path"`
	OldText string `json:"old_text"`
	NewText string `json:"new_text"`
}

func (t *EditFileTool) Execute(args json.RawMessage) ToolResult {
	var a editFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ToolResult{OK: false, Error: fmt.Sprintf("invalid args: %v", err)}
	}
	resolved, err := WorkspacePath(t.Root, a.Path)
	if err != nil {
		return ToolResult{OK: false, Error: err.Error()}
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return ToolResult{OK: false, Error: fmt.Sprintf("read failed: %v", err)}
	}
	content := string(data)
	count := strings.Count(content, a.OldText)
	if count == 0 {
		return ToolResult{OK: false, Error: "old_text not found in file"}
	}
	if count > 1 {
		return ToolResult{OK: false, Error: fmt.Sprintf("old_text matches %d times, expected exactly 1", count)}
	}
	updated := strings.Replace(content, a.OldText, a.NewText, 1)
	if err := os.WriteFile(resolved, []byte(updated), 0o644); err != nil {
		return ToolResult{OK: false, Error: fmt.Sprintf("write failed: %v", err)}
	}
	return ToolResult{OK: true, Content: fmt.Sprintf("replaced 1 occurrence in %s", a.Path)}
}
