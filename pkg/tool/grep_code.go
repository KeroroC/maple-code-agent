// pkg/tool/grep_code.go
package tool

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxGrepResults = 100

type GrepCodeTool struct {
	Root string
}

func (t *GrepCodeTool) Meta() ToolMeta {
	return ToolMeta{
		Name:        "grep_code",
		Description: "Search for a string pattern in workspace files.",
		Params: ParamSchema{
			Type: "object",
			Properties: map[string]Property{
				"pattern": {Type: "string", Description: "String to search for"},
				"include": {Type: "string", Description: "Optional glob to filter files (e.g. *.go)"},
			},
			Required: []string{"pattern"},
		},
	}
}

type grepCodeArgs struct {
	Pattern string `json:"pattern"`
	Include string `json:"include,omitempty"`
}

func (t *GrepCodeTool) Execute(args json.RawMessage) ToolResult {
	var a grepCodeArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ToolResult{OK: false, Error: fmt.Sprintf("invalid args: %v", err)}
	}
	if a.Pattern == "" {
		return ToolResult{OK: false, Error: "pattern must not be empty"}
	}
	var results []string
	truncated := false
	err := filepath.Walk(t.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || len(results) >= maxGrepResults {
			if len(results) >= maxGrepResults {
				return fmt.Errorf("limit reached")
			}
			return nil
		}
		rel, _ := filepath.Rel(t.Root, path)
		rel = filepath.ToSlash(rel)
		if a.Include != "" {
			matched, _ := filepath.Match(a.Include, filepath.Base(path))
			if !matched {
				return nil
			}
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if strings.Contains(line, a.Pattern) {
				results = append(results, fmt.Sprintf("%s:%d:%s", rel, lineNum, line))
				if len(results) >= maxGrepResults {
					return fmt.Errorf("limit reached")
				}
			}
		}
		return nil
	})
	if err != nil && !strings.Contains(err.Error(), "limit reached") {
		return ToolResult{OK: false, Error: fmt.Sprintf("walk failed: %v", err)}
	}
	truncated = len(results) >= maxGrepResults
	content := strings.Join(results, "\n")
	meta, _ := json.Marshal(map[string]any{"count": len(results), "truncated": truncated})
	return LimitResult(ToolResult{OK: true, Content: content, Truncated: truncated, Metadata: meta})
}
