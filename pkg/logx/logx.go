// Package logx 提供分级日志记录器，写入 ~/.maplecode/logs/ 下的轮转文件。
// 当 Init 以 debug=true 调用时，debug 级别消息也会输出到 stderr。
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
	// MaxLogSizeMB 是每个日志文件的轮转阈值（MB）。
	MaxLogSizeMB = 50
	// MaxLogBackups 是保留的旧日志文件数量。
	MaxLogBackups = 7
	// MaxLogAgeDays 是旧日志文件在删除前保留的天数。
	MaxLogAgeDays = 7
)

var (
	mu      sync.Mutex
	fileLog *log.Logger
	dbgLog  *log.Logger // nil when debug is disabled
	logPath string
	rotator *lumberjack.Logger
)

// Init 配置全局日志记录器。logDir 是轮转日志文件所在的目录。
// 当 debug 为 true 时，debug 级别消息也会写入 stderr。
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

// LogPath 返回当前活动日志文件的路径。如果 Init 未运行则返回空字符串。
func LogPath() string {
	mu.Lock()
	defer mu.Unlock()
	return logPath
}

// Close 释放轮转器持有的底层文件句柄。
func Close() error {
	mu.Lock()
	defer mu.Unlock()
	if rotator == nil {
		return nil
	}
	return rotator.Close()
}

// Debug 记录 debug 级别日志。当 Init 以 debug=true 调用时，消息也会输出到 stderr。
func Debug(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if dbgLog != nil {
		dbgLog.Debugf(format, args...)
	}
}

// Info 记录 info 级别日志到轮转日志文件。
func Info(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if fileLog != nil {
		fileLog.Infof(format, args...)
	}
}

// Warn 记录 warn 级别日志到轮转日志文件。
func Warn(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if fileLog != nil {
		fileLog.Warnf(format, args...)
	}
}

// Error 记录 error 级别日志到轮转日志文件。格式字符串使用 fmt 渲染。
func Error(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if fileLog != nil {
		fileLog.Errorf(format, args...)
	}
}

// stderrProxy 将写入路由到 os.Stderr 的当前值。需要这种间接方式，
// 因为测试会用管道替换 os.Stderr，我们希望日志记录器能跟随这个替换。
type stderrProxy struct{}

func (stderrProxy) Write(p []byte) (int, error) {
	return os.Stderr.Write(p)
}
