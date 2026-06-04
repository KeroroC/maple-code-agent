// Package logx provides a leveled logger that writes to a rotated file in ~/.maplecode/logs/.
// When Init is called with debug=true, debug-level messages also go to stderr.
package logx

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	// MaxLogSizeMB is the per-file rotation threshold.
	MaxLogSizeMB = 50
	// MaxLogBackups is the number of old log files to keep.
	MaxLogBackups = 7
	// MaxLogAgeDays is how long an old log file is retained before deletion.
	MaxLogAgeDays = 7
)

var (
	mu       sync.Mutex
	fileLog  *log.Logger
	dbgLog   *log.Logger // nil when debug is disabled
	logPath  string
	rotator  *lumberjack.Logger
)

// Init configures the global logger. logDir is the directory where the rotated
// log file lives. When debug is true, debug-level messages are also written to stderr.
func Init(debug bool, logDir string) error {
	mu.Lock()
	defer mu.Unlock()

	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	logPath = filepath.Join(logDir, fmt.Sprintf("maple-%s.log", time.Now().Format("2006-01-02")))
	rotator = &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    MaxLogSizeMB,
		MaxBackups: MaxLogBackups,
		MaxAge:     MaxLogAgeDays,
		Compress:   true,
	}

	fileLog = log.New(rotator)
	fileLog.SetLevel(log.InfoLevel)
	fileLog.SetReportTimestamp(true)
	fileLog.SetTimeFormat(time.RFC3339)

	if debug {
		dbgLog = log.New(stderrProxy{})
		dbgLog.SetLevel(log.DebugLevel)
		dbgLog.SetReportTimestamp(false)
	} else {
		dbgLog = nil
	}
	return nil
}

// LogPath returns the file path of the currently active log file. Empty if Init has not run.
func LogPath() string {
	mu.Lock()
	defer mu.Unlock()
	return logPath
}

// Close releases the underlying file handle held by the rotator.
func Close() error {
	mu.Lock()
	defer mu.Unlock()
	if rotator == nil {
		return nil
	}
	return rotator.Close()
}

// Debug logs at debug level. When Init was called with debug=true the message also goes to stderr.
func Debug(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if dbgLog != nil {
		dbgLog.Debugf(format, args...)
	}
}

// Info logs at info level to the rotated log file.
func Info(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if fileLog != nil {
		fileLog.Infof(format, args...)
	}
}

// Warn logs at warn level to the rotated log file.
func Warn(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if fileLog != nil {
		fileLog.Warnf(format, args...)
	}
}

// Error logs at error level to the rotated log file. Format string is rendered with fmt.
func Error(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if fileLog != nil {
		fileLog.Errorf(format, args...)
	}
}

// stderrProxy routes writes to the current value of os.Stderr. We need this indirection
// because tests replace os.Stderr with a pipe and we want our logger to follow that swap.
type stderrProxy struct{}

func (stderrProxy) Write(p []byte) (int, error) {
	return os.Stderr.Write(p)
}
