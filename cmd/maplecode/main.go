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

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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

	sessionsDir := filepath.Join(filepath.Dir(configPath), "sessions")
	chat := tui.New(sess, streamer, cfg.Model, cfg.Thinking.Enabled, cfg.Thinking.BudgetTokens)
	app := newApp(chat, streamer, sess, cfg, sessionsDir)
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
	streamer    provider.Streamer
	sess        *session.Session
	cfg         *config.Config
	ctx         context.Context
	cancel      context.CancelFunc
	program     chunkSender
	streaming   bool
	sessionsDir string
	viewport    viewport.Model
	textarea    textarea.Model
	width       int
	height      int
}

func newApp(m *tui.Model, s provider.Streamer, sess *session.Session, cfg *config.Config, sessionsDir string) *app {
	ctx, cancel := context.WithCancel(context.Background())

	ta := textarea.New()
	ta.Placeholder = "Send a message..."
	ta.ShowLineNumbers = false
	ta.SetWidth(80)
	ta.SetHeight(1)
	ta.Focus()

	vp := viewport.New(80, 20)
	vp.SetContent(m.RenderMessages())
	vp.GotoBottom()
	// Disable viewport key bindings to avoid conflicts with textarea.
	vp.KeyMap = viewport.KeyMap{}

	return &app{
		Model:       m,
		streamer:    s,
		sess:        sess,
		cfg:         cfg,
		ctx:         ctx,
		cancel:      cancel,
		sessionsDir: sessionsDir,
		textarea:    ta,
		viewport:    vp,
		width:       80,
		height:      24,
	}
}

// setProgram hands the running Bubble Tea program to the app so the streaming
// goroutine can push chunks back into Update via Send. Called once, right
// after tea.NewProgram returns.
func (a *app) setProgram(p *tea.Program) { a.program = p }

// Init returns the textarea focus command so the cursor blinks from the start.
func (a *app) Init() tea.Cmd { return a.textarea.Focus() }

// refreshView re-renders all messages into the viewport and scrolls to the bottom.
func (a *app) refreshView() {
	a.viewport.SetContent(a.Model.RenderMessages())
	a.viewport.GotoBottom()
}

// Update is the central event router. KeyMsg drives the input line and the
// chunkMsg values from the streaming goroutine update the model.
func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		taCmd  tea.Cmd
		vpCmd  tea.Cmd
		cmds   []tea.Cmd
	)

	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = v.Width
		a.height = v.Height
		// Layout: viewport (most space) + status bar (1 line) + textarea (1 line)
		textareaHeight := 1
		statusBarHeight := 1
		a.textarea.SetWidth(v.Width)
		a.textarea.SetHeight(textareaHeight)
		a.viewport.Width = v.Width
		a.viewport.Height = v.Height - textareaHeight - statusBarHeight
		a.refreshView()
		return a, nil

	case tea.KeyMsg:
		return a.onKey(v)

	case chunkMsg:
		a.Model.HandleChunk(v.c)
		a.refreshView()
		if _, done := v.c.(provider.Done); done {
			a.streaming = false
			return a, nil
		}
		if se, ok := v.c.(provider.StreamError); ok {
			if errors.Is(se.Err, provider.ErrCanceled) {
				a.streaming = false
			}
		}
		return a, nil
	}

	// Forward unhandled messages to textarea and viewport.
	a.textarea, taCmd = a.textarea.Update(msg)
	a.viewport, vpCmd = a.viewport.Update(msg)
	cmds = append(cmds, taCmd, vpCmd)
	return a, tea.Batch(cmds...)
}

// View renders the three-section layout: viewport, status bar, textarea.
func (a *app) View() string {
	statusBar := lipgloss.NewStyle().
		Foreground(lipgloss.Color("0")).
		Background(lipgloss.Color("6")).
		Width(a.width).
		Render(a.Model.RenderStatusBar())

	return fmt.Sprintf("%s\n%s\n%s", a.viewport.View(), statusBar, a.textarea.View())
}

func (a *app) onKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "ctrl+c":
		if a.streaming {
			a.cancel()
			return a, nil
		}
		a.cancel()
		return a, tea.Quit

	case "tab":
		// Toggle thinking expansion on the last message.
		if a.Model.LastMessageHasThinking() {
			views := a.Model.MessageViews()
			a.Model.ToggleThinkingExported(len(views) - 1)
			a.refreshView()
		}
		return a, nil

	case "enter":
		text := strings.TrimSpace(a.textarea.Value())
		a.textarea.Reset()
		if text == "" {
			return a, nil
		}
		if kind, args, ok := tui.ParseCommand(text); ok {
			if kind == "exit" {
				a.cancel()
				return a, tea.Quit
			}
			if a.streaming {
				a.Model.AppendSystemError("busy: a stream is already in progress")
				a.refreshView()
				return a, nil
			}
			if kind == "resume" {
				a.handleResume(args)
				a.refreshView()
				return a, nil
			}
			if kind == "compact" {
				a.handleCompact()
				a.refreshView()
				return a, nil
			}
			if kind == "help" {
				a.Model.AppendSystemMessage(tui.HelpText())
				a.refreshView()
				return a, nil
			}
			if err := a.Model.ExecuteCommand(tui.Command{Kind: kind, Args: args}); err != nil {
				a.Model.AppendSystemError(err.Error())
			}
			a.refreshView()
			return a, nil
		}
		if a.streaming {
			a.Model.AppendSystemError("busy: a stream is already in progress")
			a.refreshView()
			return a, nil
		}
		a.Model.UserSubmitted(text)
		a.streaming = true
		a.refreshView()
		return a, a.startStream()
	}

	// Forward all other keys to the textarea.
	var cmd tea.Cmd
	a.textarea, cmd = a.textarea.Update(k)
	return a, cmd
}

// chunkMsg carries one provider.Chunk through the Bubble Tea Update loop.
type chunkMsg struct{ c provider.Chunk }

// startStream kicks off a goroutine that drives the streamer and converts each
// chunk into a tea.Msg. It returns a tea.Cmd that runs the goroutine; the
// goroutine uses program.Send so the chunks arrive back in Update without
// deadlocking the cmd's return path.
func (a *app) startStream() tea.Cmd {
	// Create a fresh context so Ctrl+C from a previous stream doesn't poison this one.
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel

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

// handleResume loads a previous session by ID prefix and swaps it in.
func (a *app) handleResume(id string) tea.Model {
	if id == "" {
		a.Model.AppendSystemError("usage: /resume <id|timestamp>")
		return a
	}
	match, err := findSessionFile(a.sessionsDir, id)
	if err != nil {
		a.Model.AppendSystemError(fmt.Sprintf("resume: %v", err))
		return a
	}
	newSess, err := session.Open(match)
	if err != nil {
		a.Model.AppendSystemError(fmt.Sprintf("resume: %v", err))
		return a
	}
	_ = a.sess.Close()
	a.sess = newSess
	a.Model.SetSession(newSess)
	logx.Info("resumed session id=%s", newSess.ID())
	return a
}

// handleCompact summarizes the current session and replaces it with a new one.
func (a *app) handleCompact() tea.Model {
	now := time.Now().UTC()
	newID := now.Format("20060102-150405") + "-compact"
	newPath := filepath.Join(a.sessionsDir, newID+".jsonl")

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel

	newSess, err := a.sess.Compact(ctx, a.streamer, newPath, newID, now)
	if err != nil {
		a.Model.AppendSystemError(fmt.Sprintf("compact failed: %v", err))
		return a
	}
	_ = a.sess.Close()
	a.sess = newSess
	a.Model.SetSession(newSess)
	logx.Info("compacted session, new id=%s", newSess.ID())
	return a
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
