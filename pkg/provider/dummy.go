package provider

import (
	"context"
	"time"
)

// ScriptedStreamer 是测试中使用的确定性 Streamer。它按固定顺序回放 chunk，
// 每个 chunk 之间有可配置的延迟，并尊重 ctx 取消。
type ScriptedStreamer struct {
	chunks []Chunk
	delay  time.Duration
}

// NewScriptedStreamer 构建按顺序发出给定 chunk 的流式传输器。
// delay 是 chunk 之间的等待时间；使用 0 则连续发出。
func NewScriptedStreamer(chunks []Chunk) *ScriptedStreamer {
	return &ScriptedStreamer{chunks: chunks, delay: 5 * time.Millisecond}
}

// Stream 逐个发送脚本化的 chunk。如果在流式传输过程中 ctx 被取消，
// 剩余的 chunk 将被丢弃，并在关闭前发出 StreamError{ErrCanceled}。
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
