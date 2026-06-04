package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"maplecode/pkg/provider"
)

func TestCompact_ProducesNewSessionWithSummary(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.jsonl")

	meta := Metadata{ID: "20260101-120000-old", Created: time.Now().UTC(), Protocol: "anthropic", Model: "claude"}
	old, err := New(oldPath, meta)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = old.Append(Turn{Role: "user", Content: "first question"})
	_ = old.Append(Turn{Role: "assistant", Content: "first answer"})

	dummy := provider.NewScriptedStreamer([]provider.Chunk{
		provider.TextDelta{Text: "Summary: "},
		provider.TextDelta{Text: "user asked X"},
		provider.Done{Usage: provider.Usage{}},
	})

	newPath := filepath.Join(dir, "new.jsonl")
	newID := "20260101-120100-summary"

	compacted, err := old.Compact(context.Background(), dummy, newPath, newID, time.Now().UTC())
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	defer compacted.Close()

	// New session's first turn should be system, containing the summary
	turns := compacted.Snapshot()
	if len(turns) != 1 {
		t.Fatalf("new session turns = %d, want 1", len(turns))
	}
	if turns[0].Role != "system" {
		t.Errorf("first role = %q, want system", turns[0].Role)
	}
	if turns[0].Content != "Summary: user asked X" {
		t.Errorf("summary = %q, want %q", turns[0].Content, "Summary: user asked X")
	}
	if compacted.ID() != newID {
		t.Errorf("new ID = %q, want %q", compacted.ID(), newID)
	}

	// Old session should be closed (file closed), and the dummy should have been driven
	// through with the original turns + a hidden summary prompt.
	_ = old.Close()
}

func TestCompact_WritesEndMarkerToOldFile(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.jsonl")
	meta := Metadata{ID: "x", Created: time.Now().UTC(), Protocol: "openai", Model: "gpt"}
	old, _ := New(oldPath, meta)
	_ = old.Append(Turn{Role: "user", Content: "hi"})

	dummy := provider.NewScriptedStreamer([]provider.Chunk{
		provider.TextDelta{Text: "summary"},
		provider.Done{},
	})

	if _, err := old.Compact(context.Background(), dummy, filepath.Join(dir, "new.jsonl"), "new-id", time.Now().UTC()); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	data, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatalf("read old: %v", err)
	}
	if !strings.Contains(string(data), `"type":"end"`) {
		t.Errorf("old file missing end marker:\n%s", data)
	}
}
