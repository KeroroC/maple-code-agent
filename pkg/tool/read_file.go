package tool

import (
	"encoding/json"
	"fmt"
	"os"
)

type ReadFileTool struct {
	Root string
}

func (t *ReadFileTool) Meta() ToolMeta {
	return ToolMeta{
		Name:        "read_file",
		Description: "Read the contents of a file within the workspace.",
		Params: ParamSchema{
			Type: "object",
			Properties: map[string]Property{
				"path": {Type: "string", Description: "Relative path to the file"},
			},
			Required: []string{"path"},
		},
	}
}

type readFileArgs struct {
	Path string `json:"path"`
}

func (t *ReadFileTool) Execute(args json.RawMessage) ToolResult {
	var a readFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ToolResult{OK: false, Error: fmt.Sprintf("invalid args: %v", err)}
	}
	resolved, err := WorkspacePath(t.Root, a.Path)
	if err != nil {
		return ToolResult{OK: false, Error: err.Error()}
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return ToolResult{OK: false, Error: fmt.Sprintf("stat failed: %v", err)}
	}
	if info.IsDir() {
		return ToolResult{OK: false, Error: "cannot read a directory"}
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return ToolResult{OK: false, Error: fmt.Sprintf("read failed: %v", err)}
	}
	return LimitResult(ToolResult{OK: true, Content: string(data)})
}
