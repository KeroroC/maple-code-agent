// Package provider defines the Streamer interface and the chunk types that flow through it.
// Each implementation (anthropic, openai, openai-compatible) is responsible for translating
// provider-specific events into the normalized Chunk values defined here.
package provider

import (
	"context"
	"errors"
)

// Turn is a single message in the conversation. Role is "user" or "assistant".
// System instructions are passed separately to Stream and do not appear in the turns slice.
type Turn struct {
	Role    string
	Content string
}

// Chunk is the sealed interface implemented by every event that may be emitted on the
// Stream channel. Callers must use a type switch to extract the payload.
type Chunk interface {
	chunk()
}

// TextDelta is a fragment of the assistant's final answer.
type TextDelta struct {
	Text string
}

// ThinkingDelta is a fragment of the assistant's chain-of-thought.
// It is only emitted when extended thinking is enabled.
type ThinkingDelta struct {
	Text string
}

// Usage is the token accounting reported by the provider at the end of a stream.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Done signals that the provider has finished a stream successfully. It carries the
// final Usage so the caller can update status bars and accounting.
type Done struct {
	Usage Usage
}

// StreamError wraps any non-success condition: network failure, auth failure, cancellation,
// context overflow, etc. The embedded error is one of the package-level sentinels when
// the failure mode is recognized, otherwise it is the raw provider error.
type StreamError struct {
	Err error
}

func (TextDelta) chunk()    {}
func (ThinkingDelta) chunk() {}
func (Done) chunk()         {}
func (StreamError) chunk()  {}

// Sentinel errors. Use errors.Is to classify stream failures.
var (
	ErrCanceled       = errors.New("stream canceled")
	ErrContextLength  = errors.New("context length exceeded")
	ErrAuth           = errors.New("authentication failed")
	ErrRateLimit      = errors.New("rate limited")
)

// Streamer is the contract every provider backend must implement. It opens a streaming
// completion request and returns a channel of Chunk values. The channel is closed by
// the implementation when the stream ends (success, error, or context cancellation).
//
// Implementations must respect ctx cancellation: when ctx.Done() fires, they emit a
// StreamError wrapping ErrCanceled and then close the channel.
type Streamer interface {
	Stream(ctx context.Context, system string, turns []Turn) (<-chan Chunk, error)
}

// ThinkingConfig is passed to provider constructors so each streamer knows whether to
// enable extended thinking and how many thinking tokens to budget.
type ThinkingConfig struct {
	Enabled      bool
	BudgetTokens int
}
