// pkg/tool/glob_files.go
package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

const maxGlobResults = 200

type GlobFilesTool struct {
	Root string
}

func (t *GlobFilesTool) Meta() ToolMeta {
	return ToolMeta{
		Name:        "glob_files",
		Description: "Find files matching a glob pattern within the workspace.",
		Params: ParamSchema{
			Type: "object",
			Properties: map[string]Property{
				"pattern": {Type: "string", Description: "Glob pattern (e.g. **/*.go)"},
			},
			Required: []string{"pattern"},
		},
	}
}

type globFilesArgs struct {
	Pattern string `json:"pattern"`
}

func (t *GlobFilesTool) Execute(args json.RawMessage) ToolResult {
	var a globFilesArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ToolResult{OK: false, Error: fmt.Sprintf("invalid args: %v", err)}
	}
	pattern := a.Pattern
	if filepath.IsAbs(pattern) {
		return ToolResult{OK: false, Error: "absolute pattern not allowed"}
	}
	var matches []string
	err := filepath.Walk(t.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(t.Root, path)
		rel = filepath.ToSlash(rel)
		matched, _ := doublestar.Match(pattern, rel)
		if matched {
			matches = append(matches, rel)
		}
		if len(matches) >= maxGlobResults {
			return fmt.Errorf("limit reached")
		}
		return nil
	})
	if err != nil && !strings.Contains(err.Error(), "limit reached") {
		return ToolResult{OK: false, Error: fmt.Sprintf("walk failed: %v", err)}
	}
	truncated := len(matches) >= maxGlobResults
	content := strings.Join(matches, "\n")
	meta, _ := json.Marshal(map[string]any{"count": len(matches), "truncated": truncated})
	return LimitResult(ToolResult{OK: true, Content: content, Truncated: truncated, Metadata: meta})
}
