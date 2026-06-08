package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustJSON(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

func TestWorkspacePath(t *testing.T) {
	root := t.TempDir()

	t.Run("relative ok", func(t *testing.T) {
		p, err := WorkspacePath(root, "foo/bar.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.HasSuffix(p, "foo/bar.txt") && !strings.HasSuffix(p, "foo\\bar.txt") {
			t.Fatalf("unexpected path: %s", p)
		}
	})

	t.Run("absolute rejected", func(t *testing.T) {
		_, err := WorkspacePath(root, "/etc/passwd")
		if err == nil {
			t.Fatal("expected error for absolute path")
		}
	})

	t.Run("dot-dot rejected", func(t *testing.T) {
		_, err := WorkspacePath(root, "../secret")
		if err == nil {
			t.Fatal("expected error for .. path")
		}
	})

	t.Run("symlink to outside rejected", func(t *testing.T) {
		outside := filepath.Join(t.TempDir(), "outside.txt")
		os.WriteFile(outside, []byte("secret"), 0o644)
		link := filepath.Join(root, "link.txt")
		os.Symlink(outside, link)
		_, err := WorkspacePath(root, "link.txt")
		if err == nil {
			t.Fatal("expected error for symlink escape")
		}
	})
}

func TestReadFile(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "hello.txt"), []byte("world"), 0o644)
	tool := &ReadFileTool{Root: root}

	t.Run("read ok", func(t *testing.T) {
		r := tool.Execute(mustJSON(map[string]string{"path": "hello.txt"}))
		if !r.OK || r.Content != "world" {
			t.Fatalf("got %+v", r)
		}
	})

	t.Run("absolute rejected", func(t *testing.T) {
		r := tool.Execute(mustJSON(map[string]string{"path": "/etc/passwd"}))
		if r.OK {
			t.Fatal("expected failure")
		}
	})

	t.Run("directory rejected", func(t *testing.T) {
		r := tool.Execute(mustJSON(map[string]string{"path": "."}))
		if r.OK {
			t.Fatal("expected failure for directory")
		}
		if !strings.Contains(r.Error, "directory") {
			t.Fatalf("error %q should mention directory", r.Error)
		}
	})
}

func TestWriteFile(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "sub"), 0o755)
	tool := &WriteFileTool{Root: root}

	t.Run("write ok", func(t *testing.T) {
		r := tool.Execute(mustJSON(map[string]string{"path": "sub/a.txt", "content": "hello"}))
		if !r.OK {
			t.Fatalf("expected success: %v", r.Error)
		}
		data, _ := os.ReadFile(filepath.Join(root, "sub", "a.txt"))
		if string(data) != "hello" {
			t.Fatalf("got %q", data)
		}
	})

	t.Run("parent missing", func(t *testing.T) {
		r := tool.Execute(mustJSON(map[string]string{"path": "nope/file.txt", "content": "x"}))
		if r.OK {
			t.Fatal("expected failure")
		}
		if !strings.Contains(r.Error, "parent") {
			t.Fatalf("error %q should mention parent", r.Error)
		}
	})
}

func TestEditFile(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "code.go"), []byte("func main() {\n\tfmt.Println()\n}"), 0o644)
	tool := &EditFileTool{Root: root}

	t.Run("unique replace ok", func(t *testing.T) {
		r := tool.Execute(mustJSON(map[string]string{
			"path":     "code.go",
			"old_text": "fmt.Println()",
			"new_text": `fmt.Println("hello")`,
		}))
		if !r.OK {
			t.Fatalf("expected success: %v", r.Error)
		}
		data, _ := os.ReadFile(filepath.Join(root, "code.go"))
		if !strings.Contains(string(data), `"hello"`) {
			t.Fatalf("replacement not applied: %s", data)
		}
	})

	t.Run("not found", func(t *testing.T) {
		os.WriteFile(filepath.Join(root, "x.go"), []byte("aaa"), 0o644)
		r := tool.Execute(mustJSON(map[string]string{
			"path":     "x.go",
			"old_text": "bbb",
			"new_text": "ccc",
		}))
		if r.OK {
			t.Fatal("expected failure")
		}
		if !strings.Contains(r.Error, "not found") {
			t.Fatalf("error %q should mention not found", r.Error)
		}
	})

	t.Run("multiple matches", func(t *testing.T) {
		os.WriteFile(filepath.Join(root, "y.go"), []byte("aa bb aa"), 0o644)
		r := tool.Execute(mustJSON(map[string]string{
			"path":     "y.go",
			"old_text": "aa",
			"new_text": "cc",
		}))
		if r.OK {
			t.Fatal("expected failure")
		}
		if !strings.Contains(r.Error, "matches 2 times") {
			t.Fatalf("error %q should mention match count", r.Error)
		}
	})
}
