package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"maplecode/pkg/provider"
)

// Compact summarizes the current session using the given streamer, writes an
// end-marker to the old JSONL file, and returns a fresh Session whose first turn
// is a system message containing the summary.
//
// The new session's file path and id are passed in by the caller (typically
// derived from the current time + a new slug).
func (s *Session) Compact(ctx context.Context, streamer provider.Streamer, newPath, newID string, now time.Time) (*Session, error) {
	if s.file == nil {
		return nil, fmt.Errorf("compact: session is read-only")
	}

	// Snapshot the current turns so we can hand them to the streamer.
	turns := s.Snapshot()
	wireTurns := make([]provider.Turn, len(turns))
	for i, t := range turns {
		wireTurns[i] = provider.Turn{Role: t.Role, Content: t.Content}
	}

	// Append a hidden user message asking the model to summarize.
	summaryPrompt := "Please summarize the conversation so far in 500 words or less. " +
		"Preserve all decisions, code snippets, and user constraints."
	wireTurns = append(wireTurns, provider.Turn{Role: "user", Content: summaryPrompt})

	// Drive the streamer and accumulate the response.
	ch, err := streamer.Stream(ctx, "", wireTurns)
	if err != nil {
		return nil, fmt.Errorf("compact: stream: %w", err)
	}
	var summary string
	for c := range ch {
		switch v := c.(type) {
		case provider.TextDelta:
			summary += v.Text
		case provider.StreamError:
			return nil, fmt.Errorf("compact: stream error: %w", v.Err)
		case provider.Done:
			// expected
		}
	}

	// Write the end marker to the old file and close it.
	if _, err := s.file.WriteString(`{"type":"end","reason":"compact"}` + "\n"); err != nil {
		return nil, fmt.Errorf("compact: write end marker: %w", err)
	}
	if s.w != nil {
		_ = s.w.Flush()
	}
	if err := s.file.Close(); err != nil {
		return nil, fmt.Errorf("compact: close old: %w", err)
	}
	s.file = nil
	s.w = nil

	// Create the new session with the summary as its first system turn.
	newSess, err := New(newPath, Metadata{
		ID:       newID,
		Created:  now,
		Protocol: s.meta.Protocol,
		Model:    s.meta.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("compact: create new: %w", err)
	}
	if err := newSess.Append(Turn{Role: "system", Content: summary, Timestamp: now}); err != nil {
		_ = newSess.Close()
		return nil, fmt.Errorf("compact: append summary: %w", err)
	}
	return newSess, nil
}

// ensure encoding/json import is used (helper for future expansion)
var _ = json.Marshal
var _ = os.O_APPEND
