// MapleCode is a terminal AI coding assistant.
//
// main.go wires together config loading, logging, the provider streamer, the
// session, and a Bubble Tea program. The TUI state machine lives in pkg/tui;
// this file is a thin orchestrator that also implements the Bubble Tea glue
// (key handling, the chunk-to-msg pump, and graceful shutdown).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"maplecode/pkg/config"
	"maplecode/pkg/logx"
	"maplecode/pkg/provider"
	"maplecode/pkg/session"
	"maplecode/pkg/tui"
)

func main() {
	configPath := flag.String("config", "", "path to config.yaml (default: ~/.maplecode/config.yaml)")
	debug := flag.Bool("debug", false, "write debug logs to stderr in addition to the log file")
	resumeID := flag.String("resume", "", "resume a previous session by id or timestamp prefix")
	flag.Parse()

	if err := run(*configPath, *debug, *resumeID); err != nil {
		fmt.Fprintln(os.Stderr, "maplecode:", err)
		os.Exit(1)
	}
}

func run(configPath string, debug bool, resumeID string) error {
	if configPath == "" {
		var err error
		configPath, err = config.DefaultPath()
		if err != nil {
			return err
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		if errors.Is(err, config.ErrConfigNotFound) {
			fmt.Fprintf(os.Stderr, "MapleCode: created config at %s\nPlease fill in your api_key and re-run.\n", configPath)
			return nil
		}
		return err
	}

	logDir := filepath.Join(filepath.Dir(configPath), "logs")
	if err := logx.Init(debug, logDir); err != nil {
		return fmt.Errorf("init log: %w", err)
	}
	defer logx.Close()
	logx.Info("maplecode starting protocol=%s model=%s", cfg.Protocol, cfg.Model)
	logx.Debug("config loaded base_url=%s thinking_enabled=%t budget=%d", cfg.BaseURL, cfg.Thinking.Enabled, cfg.Thinking.BudgetTokens)

	streamer, err := provider.NewStreamer(provider.StreamerConfig{
		Protocol: cfg.Protocol,
		Model:    cfg.Model,
		BaseURL:  cfg.BaseURL,
		APIKey:   cfg.APIKey,
		Thinking: provider.ThinkingConfig{Enabled: cfg.Thinking.Enabled, BudgetTokens: cfg.Thinking.BudgetTokens},
	})
	if err != nil {
		return err
	}

	sess, err := openOrCreateSession(cfg, configPath, resumeID)
	if err != nil {
		return err
	}
	defer sess.Close()
	logx.Info("session opened id=%s", sess.ID())

	chat := tui.New(sess, streamer, cfg.Model, cfg.Thinking.Enabled, cfg.Thinking.BudgetTokens)
	app := newApp(chat, streamer, sess, cfg)
	prog := tea.NewProgram(app, tea.WithAltScreen())
	app.setProgram(prog)
	_, err = prog.Run()
	return err
}

func openOrCreateSession(cfg *config.Config, configPath, resumeID string) (*session.Session, error) {
	sessionsDir := filepath.Join(filepath.Dir(configPath), "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		return nil, err
	}

	if resumeID != "" {
		match, err := findSessionFile(sessionsDir, resumeID)
		if err != nil {
			return nil, err
		}
		return session.Open(match)
	}

	now := time.Now().UTC()
	meta := session.Metadata{
		ID:       now.Format("20060102-150405") + "-new",
		Created:  now,
		Protocol: cfg.Protocol,
		Model:    cfg.Model,
	}
	path := filepath.Join(sessionsDir, meta.ID+".jsonl")
	return session.New(path, meta)
}

func findSessionFile(dir, resumeID string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var matches []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		if strings.HasPrefix(e.Name(), resumeID) {
			matches = append(matches, filepath.Join(dir, e.Name()))
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no session matching %q", resumeID)
	}
	return matches[0], nil
}

// chunkSender is the minimal interface the stream pump needs to push chunks
// into the Bubble Tea runtime. *tea.Program satisfies it; tests can pass a
// fake to assert on chunks without running a real TTY-bound program.
type chunkSender interface {
	Send(tea.Msg)
}

// app is the Bubble Tea program. It embeds *tui.Model for the state machine
// and adds the IO glue: starting a streamer goroutine, converting provider.Chunk
// values to tea.Msg values, and rendering the viewport.
type app struct {
	*tui.Model
	streamer provider.Streamer
	sess     *session.Session
	cfg      *config.Config
	ctx      context.Context
	cancel   context.CancelFunc
	input    strings.Builder
	program  chunkSender
	streaming bool
}

func newApp(m *tui.Model, s provider.Streamer, sess *session.Session, cfg *config.Config) *app {
	ctx, cancel := context.WithCancel(context.Background())
	return &app{Model: m, streamer: s, sess: sess, cfg: cfg, ctx: ctx, cancel: cancel}
}

// setProgram hands the running Bubble Tea program to the app so the streaming
// goroutine can push chunks back into Update via Send. Called once, right
// after tea.NewProgram returns.
func (a *app) setProgram(p *tea.Program) { a.program = p }

// Init returns a no-op command.
func (a *app) Init() tea.Cmd { return nil }

// Update is the central event router. KeyMsg drives the input line and the
// chunkMsg values from the streaming goroutine update the model.
func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.KeyMsg:
		return a.onKey(v)
	case chunkMsg:
		a.Model.HandleChunk(v.c)
		if _, done := v.c.(provider.Done); done {
			a.streaming = false
			return a, nil
		}
		if se, ok := v.c.(provider.StreamError); ok {
			if errors.Is(se.Err, provider.ErrCanceled) {
				a.streaming = false
			}
			// other stream errors fall through; bubble tea keeps running
		}
		return a, nil
	}
	return a, nil
}

// View is a simple plaintext render: message list + status bar + input prompt.
func (a *app) View() string {
	var b strings.Builder
	for _, m := range a.Model.Snapshot() {
		role := m.Role
		if role == "" {
			role = "assistant"
		}
		fmt.Fprintf(&b, "[%s] %s\n", role, m.Content)
	}
	b.WriteString("\n")
	b.WriteString(a.Model.RenderStatusBar())
	b.WriteString("\n> ")
	b.WriteString(a.input.String())
	return b.String()
}

func (a *app) onKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "ctrl+c":
		if a.streaming {
			a.cancel()
			// keep running; the next chunk from the goroutine will arrive as
			// StreamError(ErrCanceled) and flip streaming off.
			return a, nil
		}
		a.cancel()
		return a, tea.Quit
	case "enter":
		text := strings.TrimSpace(a.input.String())
		a.input.Reset()
		if text == "" {
			return a, nil
		}
		if kind, args, ok := tui.ParseCommand(text); ok {
			if kind == "exit" {
				a.cancel()
				return a, tea.Quit
			}
			if err := a.Model.ExecuteCommand(tui.Command{Kind: kind, Args: args}); err != nil {
				a.Model.AppendSystemError(err.Error())
			}
			return a, nil
		}
		if a.streaming {
			a.Model.AppendSystemError("busy: a stream is already in progress")
			return a, nil
		}
		a.Model.UserSubmitted(text)
		a.streaming = true
		return a, a.startStream()
	}
	a.input.WriteString(k.String())
	return a, nil
}

// chunkMsg carries one provider.Chunk through the Bubble Tea Update loop.
type chunkMsg struct{ c provider.Chunk }

// startStream kicks off a goroutine that drives the streamer and converts each
// chunk into a tea.Msg. It returns a tea.Cmd that runs the goroutine; the
// goroutine uses program.Send so the chunks arrive back in Update without
// deadlocking the cmd's return path.
func (a *app) startStream() tea.Cmd {
	ctx := a.ctx
	streamer := a.streamer
	systemPrompt := a.cfg.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = config.DefaultSystemPrompt
	}
	turns := wireTurnsFromSession(a.sess, a.Model)
	prog := a.program

	return func() tea.Msg {
		ch, err := streamer.Stream(ctx, systemPrompt, turns)
		if err != nil {
			if prog != nil {
				prog.Send(chunkMsg{provider.StreamError{Err: err}})
			}
			return nil
		}
		for c := range ch {
			if prog != nil {
				prog.Send(chunkMsg{c: c})
			}
		}
		if prog != nil {
			prog.Send(chunkMsg{c: provider.Done{}})
		}
		return nil
	}
}

// wireTurnsFromSession rebuilds the wire-format turn list from the session
// snapshot, dropping any partially-written assistant turn that the user just
// submitted (that one is already represented by the streaming placeholder).
func wireTurnsFromSession(s *session.Session, _ *tui.Model) []provider.Turn {
	snap := s.Snapshot()
	// The last turn (if it's the user message we just submitted) is included
	// so the model sees it. We keep the snapshot intact and only filter out
	// a trailing empty assistant turn, which shouldn't normally exist anyway.
	out := make([]provider.Turn, 0, len(snap))
	for _, t := range snap {
		if t.Role == "system" {
			continue
		}
		out = append(out, provider.Turn{Role: t.Role, Content: t.Content})
	}
	return out
}
