// pkg/tool/search_tools_test.go
package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlobFiles(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "pkg"), 0o755)
	os.WriteFile(filepath.Join(root, "pkg", "a.go"), []byte("package a"), 0o644)
	os.WriteFile(filepath.Join(root, "pkg", "b.go"), []byte("package b"), 0o644)
	os.WriteFile(filepath.Join(root, "readme.md"), []byte("# hi"), 0o644)
	tool := &GlobFilesTool{Root: root}

	t.Run("find go files", func(t *testing.T) {
		r := tool.Execute(mustJSON(map[string]string{"pattern": "**/*.go"}))
		if !r.OK {
			t.Fatalf("expected success: %v", r.Error)
		}
		if !strings.Contains(r.Content, "pkg/a.go") {
			t.Fatalf("missing pkg/a.go in: %s", r.Content)
		}
		if strings.Contains(r.Content, "readme.md") {
			t.Fatalf("should not contain readme.md")
		}
	})

	t.Run("absolute rejected", func(t *testing.T) {
		r := tool.Execute(mustJSON(map[string]string{"pattern": "/**/*.go"}))
		if r.OK {
			t.Fatal("expected failure")
		}
	})
}

func TestGrepCode(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "src"), 0o755)
	os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("func main() {\n\tfmt.Println(\"hello\")\n}"), 0o644)
	os.WriteFile(filepath.Join(root, "src", "util.go"), []byte("func helper() {}"), 0o644)
	tool := &GrepCodeTool{Root: root}

	t.Run("find pattern", func(t *testing.T) {
		r := tool.Execute(mustJSON(map[string]string{"pattern": "Println"}))
		if !r.OK {
			t.Fatalf("expected success: %v", r.Error)
		}
		if !strings.Contains(r.Content, "src/main.go:2:") {
			t.Fatalf("unexpected format: %s", r.Content)
		}
	})

	t.Run("with include glob", func(t *testing.T) {
		r := tool.Execute(mustJSON(map[string]string{"pattern": "func", "include": "*.go"}))
		if !r.OK {
			t.Fatalf("expected success: %v", r.Error)
		}
	})

	t.Run("not found", func(t *testing.T) {
		r := tool.Execute(mustJSON(map[string]string{"pattern": "NOTFOUND"}))
		if !r.OK {
			t.Fatalf("expected success with empty results: %v", r.Error)
		}
		if r.Content != "" {
			t.Fatalf("expected empty content, got: %s", r.Content)
		}
	})
}
