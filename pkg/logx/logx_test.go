package logx

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInfo_WritesToLogFile(t *testing.T) {
	dir := t.TempDir()
	if err := Init(false, dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	Info("hello %s", "world")

	got := readLogFile(t, dir)
	if !strings.Contains(got, "hello world") {
		t.Errorf("expected log file to contain 'hello world', got:\n%s", got)
	}
}

func TestDebug_DoesNotWriteToLogFileByDefault(t *testing.T) {
	dir := t.TempDir()
	if err := Init(false, dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Write one Info so the file actually exists on disk.
	Info("anchor message")
	Debug("secret debug %s", "noise")

	got := readLogFile(t, dir)
	if !strings.Contains(got, "anchor message") {
		t.Errorf("expected 'anchor message' in log file, got:\n%s", got)
	}
	if strings.Contains(got, "secret debug") {
		t.Errorf("debug message should not appear in log file by default, got:\n%s", got)
	}
}

func TestDebug_WritesToStderrWhenDebugEnabled(t *testing.T) {
	dir := t.TempDir()
	if err := Init(true, dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	oldStderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = oldStderr }()

	Debug("visible debug %s", "now")
	w.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	if !strings.Contains(buf.String(), "visible debug now") {
		t.Errorf("expected stderr to contain 'visible debug now', got: %q", buf.String())
	}
}

func TestInit_CreatesLogDirectoryIfMissing(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "logs", "nested")
	if err := Init(false, dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("expected %s to exist, got: %v", dir, err)
	}
}

// readLogFile finds the rotated log file under dir and returns its contents.
// The lumberjack writer is closed during Init; we read whatever the most recent
// writer left on disk.
func readLogFile(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read log dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasPrefix(e.Name(), "maple-") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		return string(data)
	}
	// Debug: list what is actually there
	t.Logf("dir contents:")
	for _, e := range entries {
		t.Logf("  - %s (dir=%v)", e.Name(), e.IsDir())
	}
	t.Fatalf("no maple-*.log file found in %s", dir)
	return ""
}
