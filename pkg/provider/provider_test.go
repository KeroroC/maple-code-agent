package provider

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestStream_DeliversTextDeltasThenDone(t *testing.T) {
	dummy := NewScriptedStreamer([]Chunk{
		TextDelta{Text: "hello "},
		TextDelta{Text: "world"},
		Done{Usage: Usage{InputTokens: 5, OutputTokens: 2}},
	})

	ch, err := dummy.Stream(context.Background(), "system", nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var got []string
	for c := range ch {
		if td, ok := c.(TextDelta); ok {
			got = append(got, td.Text)
		}
	}
	want := "hello world"
	if strings.Join(got, "") != want {
		t.Errorf("chunks = %q, want %q", strings.Join(got, ""), want)
	}
}

func TestStream_CtxCanceled_EmitsStreamErrorCanceled(t *testing.T) {
	// Multiple chunks so the cancel can fire mid-stream.
	dummy := NewScriptedStreamer([]Chunk{
		TextDelta{Text: "a"},
		TextDelta{Text: "b"},
		TextDelta{Text: "c"},
		TextDelta{Text: "d"},
		TextDelta{Text: "e"},
	})
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := dummy.Stream(ctx, "system", nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	// Pull first chunk
	<-ch
	cancel()

	// Drain remaining; expect at least one StreamError with ErrCanceled
	var sawCanceled bool
	deadline := time.After(2 * time.Second)
	for {
		select {
		case c, ok := <-ch:
			if !ok {
				if !sawCanceled {
					t.Fatal("channel closed without seeing ErrCanceled")
				}
				return
			}
			if se, ok := c.(StreamError); ok {
				if errors.Is(se.Err, ErrCanceled) {
					sawCanceled = true
				}
			}
		case <-deadline:
			t.Fatal("timed out waiting for channel close")
		}
	}
}

func TestSentinels_AreDistinct(t *testing.T) {
	sentinels := []error{ErrCanceled, ErrContextLength, ErrAuth, ErrRateLimit}
	for i, a := range sentinels {
		for j, b := range sentinels {
			if i == j {
				continue
			}
			if errors.Is(a, b) {
				t.Errorf("sentinels should be distinct, but %v matched %v", a, b)
			}
		}
	}
}

func TestTurn_StoresRoleAndContent(t *testing.T) {
	tr := Turn{Role: "user", Content: "hi"}
	if tr.Role != "user" || tr.Content != "hi" {
		t.Errorf("Turn not stored verbatim: %+v", tr)
	}
}
