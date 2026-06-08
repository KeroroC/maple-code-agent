package tool

import (
	"encoding/json"
	"strings"
	"testing"
)

type fakeTool struct {
	meta ToolMeta
}

func (f *fakeTool) Meta() ToolMeta { return f.meta }
func (f *fakeTool) Execute(args json.RawMessage) ToolResult {
	return ToolResult{OK: true, Content: "fake"}
}

func TestRegistryLookup(t *testing.T) {
	r := NewRegistry()
	r.Register(&fakeTool{meta: ToolMeta{Name: "a", Description: "tool a"}})
	r.Register(&fakeTool{meta: ToolMeta{Name: "b", Description: "tool b"}})

	t.Run("known tool", func(t *testing.T) {
		tool, err := r.Lookup("a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tool.Meta().Name != "a" {
			t.Fatalf("got %s, want a", tool.Meta().Name)
		}
	})

	t.Run("unknown tool", func(t *testing.T) {
		_, err := r.Lookup("nope")
		if err == nil {
			t.Fatal("expected error for unknown tool")
		}
		if !strings.Contains(err.Error(), "unknown tool") {
			t.Fatalf("error %q does not mention unknown tool", err)
		}
	})
}

func TestRegistryAllMeta(t *testing.T) {
	r := NewRegistry()
	r.Register(&fakeTool{meta: ToolMeta{Name: "x", Description: "x tool"}})
	r.Register(&fakeTool{meta: ToolMeta{Name: "y", Description: "y tool"}})
	meta := r.AllMeta()
	if len(meta) != 2 {
		t.Fatalf("got %d meta, want 2", len(meta))
	}
}

func TestLimitResult(t *testing.T) {
	t.Run("short content unchanged", func(t *testing.T) {
		r := LimitResult(ToolResult{OK: true, Content: "hello"})
		if r.Content != "hello" || r.Truncated {
			t.Fatalf("unexpected truncation")
		}
	})

	t.Run("long content truncated", func(t *testing.T) {
		long := strings.Repeat("x", MaxResultSize+100)
		r := LimitResult(ToolResult{OK: true, Content: long})
		if len(r.Content) != MaxResultSize {
			t.Fatalf("got len %d, want %d", len(r.Content), MaxResultSize)
		}
		if !r.Truncated {
			t.Fatal("expected truncated=true")
		}
	})
}