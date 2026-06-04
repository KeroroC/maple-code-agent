package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNew_WritesMetadataAndAppendTurns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	meta := Metadata{
		ID:       "20260101-120000-hello",
		Created:  time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		Protocol: "anthropic",
		Model:    "claude-test",
	}
	s, err := New(path, meta)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Append(Turn{Role: "user", Content: "hi"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := splitLines(string(data))
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d:\n%s", len(lines), data)
	}
	var gotMeta metaRecord
	if err := json.Unmarshal([]byte(lines[0]), &gotMeta); err != nil {
		t.Fatalf("meta unmarshal: %v", err)
	}
	if gotMeta.Type != "meta" || gotMeta.ID != meta.ID {
		t.Errorf("meta = %+v, want type=meta id=%q", gotMeta, meta.ID)
	}
	if gotMeta.Protocol != "anthropic" {
		t.Errorf("protocol = %q, want anthropic", gotMeta.Protocol)
	}
	var gotTurn turnRecord
	if err := json.Unmarshal([]byte(lines[1]), &gotTurn); err != nil {
		t.Fatalf("turn unmarshal: %v", err)
	}
	if gotTurn.Type != "turn" || gotTurn.Role != "user" || gotTurn.Content != "hi" {
		t.Errorf("turn = %+v", gotTurn)
	}
}

func TestOpen_ReadsBackTurnsInOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	meta := Metadata{ID: "x", Created: time.Now().UTC(), Protocol: "openai", Model: "gpt"}
	s, _ := New(path, meta)
	_ = s.Append(Turn{Role: "user", Content: "a"})
	_ = s.Append(Turn{Role: "assistant", Content: "b"})
	_ = s.Append(Turn{Role: "user", Content: "c"})
	_ = s.Close()

	loaded, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer loaded.Close()

	got := loaded.Snapshot()
	if len(got) != 3 {
		t.Fatalf("got %d turns, want 3", len(got))
	}
	want := []Turn{
		{Role: "user", Content: "a"},
		{Role: "assistant", Content: "b"},
		{Role: "user", Content: "c"},
	}
	for i, w := range want {
		if got[i].Role != w.Role || got[i].Content != w.Content {
			t.Errorf("turn %d = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestOpen_SkipsCorruptLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	body := `{"type":"meta","id":"x","created":"2026-01-01T00:00:00Z","protocol":"anthropic","model":"m"}
{"type":"turn","role":"user","content":"ok","ts":"2026-01-01T00:00:01Z"}
{this is not json}
{"type":"turn","role":"assistant","content":"fine","ts":"2026-01-01T00:00:02Z"}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	turns := s.Snapshot()
	if len(turns) != 2 {
		t.Fatalf("got %d turns, want 2 (corrupt line should be skipped)", len(turns))
	}
	if turns[0].Content != "ok" || turns[1].Content != "fine" {
		t.Errorf("unexpected turns: %+v", turns)
	}
}

func TestAppend_ConcurrentWritesAreSerialized(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	meta := Metadata{ID: "x", Created: time.Now().UTC(), Protocol: "openai", Model: "gpt"}
	s, _ := New(path, meta)
	defer s.Close()

	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			if err := s.Append(Turn{Role: "user", Content: "x"}); err != nil {
				t.Errorf("Append %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	// 1 meta + N turns = N+1 lines
	data, _ := os.ReadFile(path)
	lines := splitLines(string(data))
	if len(lines) != N+1 {
		t.Errorf("line count = %d, want %d", len(lines), N+1)
	}
}

func TestSlugify_StripsAndTruncates(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Hello, world!", "hello-world"},
		{"  spaces  and  tabs\t", "spaces-and-tabs"},
		{"a/b\\c?d:e", "a-b-c-d-e"},
		{"", ""},
	}
	for _, c := range cases {
		if got := slugify(c.in); got != c.want {
			t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIDFromTimestamp_FormatIsYYYYMMDDHHMMSS(t *testing.T) {
	ts := time.Date(2026, 6, 4, 10, 30, 45, 0, time.UTC)
	got := idFromTimestamp(ts)
	if got != "20260604-103045" {
		t.Errorf("idFromTimestamp = %q, want 20260604-103045", got)
	}
}

func splitLines(s string) []string {
	out := []string{}
	for _, l := range strings.Split(s, "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
