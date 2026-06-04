package main

import (
	"context"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"maplecode/pkg/config"
	"maplecode/pkg/provider"
	"maplecode/pkg/session"
	"maplecode/pkg/tui"
)

// fakeSender records every message the stream pump hands to it. It stands
// in for *tea.Program in tests, so we can assert on chunk delivery without
// spinning up a real Bubble Tea program.
type fakeSender struct {
	mu       sync.Mutex
	messages []tea.Msg
}

func (f *fakeSender) Send(m tea.Msg) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, m)
}

func (f *fakeSender) Snapshot() []tea.Msg {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]tea.Msg, len(f.messages))
	copy(out, f.messages)
	return out
}

// scriptedStreamer yields a fixed chunk sequence and honours ctx.Done().
type scriptedStreamer struct {
	chunks []provider.Chunk
	calls  int
}

func (s *scriptedStreamer) Stream(ctx context.Context, _ string, _ []provider.Turn) (<-chan provider.Chunk, error) {
	s.calls++
	out := make(chan provider.Chunk, len(s.chunks))
	go func() {
		defer close(out)
		for _, c := range s.chunks {
			select {
			case <-ctx.Done():
				return
			case out <- c:
			}
		}
	}()
	return out, nil
}

type erroringStreamer struct{ err error }

func (e *erroringStreamer) Stream(_ context.Context, _ string, _ []provider.Turn) (<-chan provider.Chunk, error) {
	return nil, e.err
}

func newTestApp(t *testing.T, st provider.Streamer) (*app, *fakeSender) {
	t.Helper()
	dir := t.TempDir()
	sess, err := session.New(dir+"/s.jsonl", session.Metadata{
		ID: "x", Created: time.Now().UTC(), Protocol: "anthropic", Model: "m",
	})
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	chat := tui.New(sess, st, "m", false, 0)
	cfg := &config.Config{Protocol: "anthropic", Model: "m", SystemPrompt: "test"}
	a := newApp(chat, st, sess, cfg)
	sender := &fakeSender{}
	a.program = sender
	return a, sender
}

// drainPump runs the streaming cmd in a goroutine and waits briefly for
// the goroutine to drain the streamer's channel.
func drainPump(a *app) {
	cmd := a.startStream()
	go func() { _ = cmd() }()
	// Poll until the scripted streamer's goroutine has had a chance to
	// send all chunks. 50ms is plenty for tests with no I/O.
	time.Sleep(50 * time.Millisecond)
}

func TestStartStream_DeliversAllChunksAndDone(t *testing.T) {
	st := &scriptedStreamer{chunks: []provider.Chunk{
		provider.TextDelta{Text: "hello "},
		provider.TextDelta{Text: "world"},
		provider.Done{},
	}}
	a, sender := newTestApp(t, st)
	drainPump(a)

	msgs := sender.Snapshot()
	if len(msgs) < 4 {
		t.Fatalf("got %d messages, want >=4: %+v", len(msgs), msgs)
	}
	last, ok := msgs[len(msgs)-1].(chunkMsg)
	if !ok {
		t.Fatalf("last message is %T, want chunkMsg", msgs[len(msgs)-1])
	}
	if _, done := last.c.(provider.Done); !done {
		t.Errorf("last chunk = %T, want provider.Done", last.c)
	}
	if st.calls != 1 {
		t.Errorf("Stream calls = %d, want 1", st.calls)
	}
}

func TestStartStream_SynchronousErrorIsForwardedAsStreamError(t *testing.T) {
	st := &erroringStreamer{err: provider.ErrAuth}
	a, sender := newTestApp(t, st)
	drainPump(a)

	msgs := sender.Snapshot()
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1 (the forwarded StreamError): %+v", len(msgs), msgs)
	}
	cm, ok := msgs[0].(chunkMsg)
	if !ok {
		t.Fatalf("message 0 is %T, want chunkMsg", msgs[0])
	}
	se, ok := cm.c.(provider.StreamError)
	if !ok {
		t.Fatalf("chunk is %T, want provider.StreamError", cm.c)
	}
	if se.Err != provider.ErrAuth {
		t.Errorf("forwarded err = %v, want ErrAuth", se.Err)
	}
}

func TestWireTurnsFromSession_DropsSystemTurns(t *testing.T) {
	dir := t.TempDir()
	sess, _ := session.New(dir+"/s.jsonl", session.Metadata{
		ID: "x", Created: time.Now().UTC(), Protocol: "anthropic", Model: "m",
	})
	t.Cleanup(func() { _ = sess.Close() })
	_ = sess.Append(session.Turn{Role: "system", Content: "ignored"})
	_ = sess.Append(session.Turn{Role: "user", Content: "hi"})
	_ = sess.Append(session.Turn{Role: "assistant", Content: "hello"})

	turns := wireTurnsFromSession(sess, nil)
	if len(turns) != 2 {
		t.Fatalf("got %d turns, want 2 (system dropped): %+v", len(turns), turns)
	}
	if turns[0].Role != "user" || turns[0].Content != "hi" {
		t.Errorf("turn 0 = %+v, want user/hi", turns[0])
	}
	if turns[1].Role != "assistant" || turns[1].Content != "hello" {
		t.Errorf("turn 1 = %+v, want assistant/hello", turns[1])
	}
}

func TestOnKey_EnterStartsStreamAndClearsInput(t *testing.T) {
	a, _ := newTestApp(t, &scriptedStreamer{})
	a.input.WriteString("hello world")
	_, _ = a.onKey(tea.KeyMsg{Type: tea.KeyEnter})
	if a.input.Len() != 0 {
		t.Errorf("input should be cleared after Enter, got %q", a.input.String())
	}
	if !a.streaming {
		t.Error("after Enter, streaming should be true")
	}
	snap := a.Model.Snapshot()
	if len(snap) == 0 || snap[0].Content != "hello world" {
		t.Errorf("user message = %q, want %q", snap[0].Content, "hello world")
	}
}

func TestOnKey_EnterWhileStreamingIsRejected(t *testing.T) {
	a, _ := newTestApp(t, &scriptedStreamer{})
	a.streaming = true
	a.input.WriteString("another question")
	_, _ = a.onKey(tea.KeyMsg{Type: tea.KeyEnter})
	if a.input.Len() != 0 {
		t.Errorf("input should be cleared even on rejected submit, got %q", a.input.String())
	}
	snap := a.Model.Snapshot()
	if len(snap) == 0 {
		t.Fatal("expected a system error message to be appended")
	}
	last := snap[len(snap)-1]
	if last.Content != "[error] busy: a stream is already in progress" {
		t.Errorf("last message = %q, want busy system error", last.Content)
	}
}

func TestOnKey_CtrlCWhileStreamingCancelsButDoesNotQuit(t *testing.T) {
	a, _ := newTestApp(t, &scriptedStreamer{})
	a.streaming = true
	_, cmd := a.onKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil {
		t.Errorf("Ctrl+C while streaming should not return tea.Quit, got %v", cmd)
	}
	if a.ctx.Err() == nil {
		t.Error("Ctrl+C while streaming should cancel the context")
	}
}

func TestOnKey_ExitCommandQuits(t *testing.T) {
	a, _ := newTestApp(t, &scriptedStreamer{})
	a.input.WriteString("/exit")
	_, cmd := a.onKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected non-nil cmd from /exit")
	}
}
