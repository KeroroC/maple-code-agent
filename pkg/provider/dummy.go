package provider

import (
	"context"
	"time"
)

// ScriptedStreamer is a deterministic Streamer used in tests. It plays back a fixed
// sequence of chunks with a configurable per-chunk delay, and honors ctx cancellation.
type ScriptedStreamer struct {
	chunks []Chunk
	delay  time.Duration
}

// NewScriptedStreamer builds a streamer that emits the given chunks in order. delay is
// the wait between chunks; use 0 to emit back-to-back.
func NewScriptedStreamer(chunks []Chunk) *ScriptedStreamer {
	return &ScriptedStreamer{chunks: chunks, delay: 5 * time.Millisecond}
}

// Stream sends the scripted chunks one by one. If ctx is canceled mid-stream, the
// remaining chunks are dropped and a StreamError{ErrCanceled} is emitted before close.
func (s *ScriptedStreamer) Stream(ctx context.Context, _ string, _ []Turn) (<-chan Chunk, error) {
	out := make(chan Chunk, len(s.chunks))
	go func() {
		defer close(out)
		for _, c := range s.chunks {
			select {
			case <-ctx.Done():
				out <- StreamError{Err: ErrCanceled}
				return
			case <-time.After(s.delay):
				out <- c
			}
		}
	}()
	return out, nil
}
