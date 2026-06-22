package session

import (
	"context"
	"fmt"
	"time"

	"maplecode/pkg/provider"
)

// Compact 使用给定的流式传输器对当前会话进行摘要，向旧 JSONL 文件写入结束标记，
// 并返回一个新会话，其第一个轮次是包含摘要的系统消息。
//
// 新会话的文件路径和 id 由调用者传入（通常基于当前时间 + 新标识）。
func (s *Session) Compact(ctx context.Context, streamer provider.Streamer, newPath, newID string, now time.Time) (*Session, error) {
	if s.file == nil {
		return nil, fmt.Errorf("compact: session is read-only")
	}

	// 快照当前轮次，以便交给流式传输器。
	turns := s.Snapshot()
	wireTurns := make([]provider.Turn, len(turns))
	for i, t := range turns {
		wireTurns[i] = provider.Turn{Role: t.Role, Content: t.Content}
	}

	// 追加一个隐藏的用户消息，请求模型进行摘要。
	summaryPrompt := "Please summarize the conversation so far in 500 words or less. " +
		"Preserve all decisions, code snippets, and user constraints."
	wireTurns = append(wireTurns, provider.Turn{Role: "user", Content: summaryPrompt})

	// 驱动流式传输器并累积响应。
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

	// 向旧文件写入结束标记并关闭它。
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

	// 创建新会话，摘要作为其第一个系统轮次。
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
