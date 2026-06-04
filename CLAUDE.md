# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

MapleCode is a terminal AI coding assistant (similar to Claude Code), written in Go. Phase 1 implements the "conversation kernel" — a TUI-based multi-turn chat with LLM backends, streaming responses, session persistence, and markdown rendering. Tool use and file operations are out of scope for this phase.

## Commands

```bash
# Build
go build ./...

# Run
go run ./cmd/maplecode
go run ./cmd/maplecode --config <path> --debug --resume <id>

# Test all
go test -race ./...

# Test a single package
go test ./pkg/provider/
go test -run TestAnthropicSSE ./pkg/provider/

# Vet
go vet ./...
```

CLI flags: `--config` (default `~/.maplecode/config.yaml`), `--debug`, `--resume <id>`

## Architecture

Four-layer design:

1. **Provider** (`pkg/provider/`) — `Streamer` interface with `Chunk` sealed interface (discriminated union: `TextDelta`, `ThinkingDelta`, `Done`, `StreamError`). Implementations: `AnthropicStreamer`, `OpenAIStreamer` (also handles OpenAI-compatible via custom base URL). Factory in `factory.go` dispatches on protocol. Sentinel errors: `ErrCanceled`, `ErrContextLength`, `ErrAuth`, `ErrRateLimit`.

2. **Session** (`pkg/session/`) — JSONL-based persistence. `Session` struct with mutex-protected appends. `Compact()` summarizes conversation and creates new session. Sessions at `~/.maplecode/sessions/<timestamp>-<slug>.jsonl`.

3. **TUI** (`pkg/tui/`) — Bubble Tea model. Per-message state machine: `pending → streaming → done | interrupted | error`. Slash commands: `/clear`, `/compact`, `/thinking`, `/model`, `/help`, `/quit`, `/sessions`. Markdown rendering via glamour (only on done messages).

4. **Application** (`cmd/maplecode/`) — `app` struct embeds `*tui.Model`, wires config/logging/providers/sessions. `startStream()` launches goroutine that drives streamer and sends chunks via `program.Send()`. `chunkSender` interface over `*tea.Program` enables testability.

Data flow: User input → `onKey()` → `UserSubmitted()` → `startStream()` → goroutine reads `streamer.Stream()` channel → sends `chunkMsg` → `HandleChunk()` updates state → `View()` re-renders.

## Testing Patterns

- Standard `testing` package, no external test frameworks
- Table-driven tests for validation/parsing
- `httptest.NewServer` for provider SSE simulation
- `t.TempDir()` for filesystem isolation
- Manual fakes: `fakeSender`, `scriptedStreamer`, `erroringStreamer`
- Tests are in `_test.go` files alongside source

## Development Workflow

When given a new idea or feature request:

1. Ask clarifying questions to refine requirements and explore edge cases
2. Create three documents in `docs/`:

**spec.md** — What problem, what capabilities, what's out of scope, design skeleton. No function names, parameter names, defaults, or error text (these are implementation details that go stale).

**tasks.md** — 5–15 tasks, each completable in one session. Each task lists: affected files, dependent tasks, reference locations (exact function/line numbers OK). Last two tasks must be "wire into main flow" + "end-to-end verification".

**checklist.md** — Every item must be checkable and observable. No "implementation complete" or "good quality". Use concrete values from spec as acceptance criteria. Examples: "`grep -r X` returns ≥3 results", "input Y shows output Z". At least one end-to-end acceptance item.
