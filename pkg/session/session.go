// Package session 维护内存中的轮次列表并将其持久化到 JSONL 文件。
// 每个会话有一个元数据头（第一行），后面每个轮次一条记录。
// 写入通过互斥锁序列化；并发追加永远不会在磁盘上交错。
package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Metadata 是创建会话时写入的 JSONL 头。
type Metadata struct {
	ID       string    `json:"id"`
	Created  time.Time `json:"created"`
	Protocol string    `json:"protocol"`
	Model    string    `json:"model"`
}

// Turn 是一条用户或助手消息。Timestamp/Interrupted 字段是可选的，
// 当值非零/为 true 时填充。
type Turn struct {
	Role        string
	Content     string
	Timestamp   time.Time
	Interrupted bool
}

// ToolCall 是持久化的工具调用。
type ToolCall struct {
	CallID   string
	ToolName string
	Args     json.RawMessage
	TS       time.Time
}

// ToolResult 是持久化的工具执行结果。
type ToolResult struct {
	CallID   string
	ToolName string
	Result   json.RawMessage
	Summary  string
	TS       time.Time
}

// Snapshot 返回内存中轮次列表的副本。调用者可以修改结果而不影响会话。
func (s *Session) Snapshot() []Turn {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Turn, len(s.turns))
	copy(out, s.turns)
	return out
}

// ToolCalls 返回内存中工具调用列表的副本。
func (s *Session) ToolCalls() []ToolCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ToolCall, len(s.toolCalls))
	copy(out, s.toolCalls)
	return out
}

// ToolResults 返回内存中工具结果列表的副本。
func (s *Session) ToolResults() []ToolResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ToolResult, len(s.toolResults))
	copy(out, s.toolResults)
	return out
}

// ID 返回会话标识符（时间戳 + 标识）。
func (s *Session) ID() string {
	return s.meta.ID
}

// Path 返回磁盘上的 JSONL 文件路径。
func (s *Session) Path() string {
	return s.path
}

// metaRecord 是第一行 JSONL 的磁盘格式。
type metaRecord struct {
	Type     string    `json:"type"`
	ID       string    `json:"id"`
	Created  time.Time `json:"created"`
	Protocol string    `json:"protocol"`
	Model    string    `json:"model"`
}

// turnRecord 是后续每行的磁盘格式。
type turnRecord struct {
	Type        string    `json:"type"`
	Role        string    `json:"role"`
	Content     string    `json:"content"`
	TS          time.Time `json:"ts,omitempty"`
	Interrupted bool      `json:"interrupted,omitempty"`
}

// toolCallRecord 是工具调用条目的磁盘格式。
type toolCallRecord struct {
	Type     string          `json:"type"` // "tool_call"
	CallID   string          `json:"call_id"`
	ToolName string          `json:"tool_name"`
	Args     json.RawMessage `json:"args"`
	TS       time.Time       `json:"ts"`
}

// toolResultRecord 是工具结果条目的磁盘格式。
type toolResultRecord struct {
	Type     string          `json:"type"` // "tool_result"
	CallID   string          `json:"call_id"`
	ToolName string          `json:"tool_name"`
	Result   json.RawMessage `json:"result"`
	Summary  string          `json:"summary"`
	TS       time.Time       `json:"ts"`
}

// Session 持有当前内存中的轮次和用于追加的已打开 JSONL 文件。
// 使用 New 创建新会话，使用 Open 恢复已有会话。
type Session struct {
	path        string
	meta        Metadata
	mu          sync.Mutex
	turns       []Turn
	toolCalls   []ToolCall
	toolResults []ToolResult
	file        *os.File
	w           *bufio.Writer
}

// New 在指定路径创建新会话，写入元数据头，并返回准备好进行 Append 的会话。
// 父目录必须已存在。
func New(path string, meta Metadata) (*Session, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	s := &Session{
		path:  path,
		meta:  meta,
		file:  f,
		w:     bufio.NewWriter(f),
		turns: []Turn{},
	}
	if err := s.writeMeta(); err != nil {
		_ = f.Close()
		return nil, err
	}
	return s, nil
}

// Open 读取现有的会话文件。解析失败的行会被静默跳过
// （通过调用者的 logx 包记录警告，但会话保持最小契约：
// 损坏的行不会冒泡错误）。
func Open(path string) (*Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	s := &Session{
		path:  path,
		file:  f,
		turns: []Turn{},
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	first := true
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		if first {
			var m metaRecord
			if err := json.Unmarshal(line, &m); err == nil && m.Type == "meta" {
				s.meta = Metadata{ID: m.ID, Created: m.Created, Protocol: m.Protocol, Model: m.Model}
				first = false
				continue
			}
			first = false
			// Fall through and try to parse as turn.
		}
		var tr turnRecord
		if err := json.Unmarshal(line, &tr); err == nil && tr.Type == "turn" {
			s.turns = append(s.turns, Turn{
				Role:        tr.Role,
				Content:     tr.Content,
				Timestamp:   tr.TS,
				Interrupted: tr.Interrupted,
			})
			continue
		}

		var tcr toolCallRecord
		if err := json.Unmarshal(line, &tcr); err == nil && tcr.Type == "tool_call" {
			s.toolCalls = append(s.toolCalls, ToolCall{
				CallID:   tcr.CallID,
				ToolName: tcr.ToolName,
				Args:     tcr.Args,
				TS:       tcr.TS,
			})
			continue
		}

		var trr toolResultRecord
		if err := json.Unmarshal(line, &trr); err == nil && trr.Type == "tool_result" {
			s.toolResults = append(s.toolResults, ToolResult{
				CallID:   trr.CallID,
				ToolName: trr.ToolName,
				Result:   trr.Result,
				Summary:  trr.Summary,
				TS:       trr.TS,
			})
			continue
		}
		// else: skip unparseable line
	}
	if err := scanner.Err(); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("scan: %w", err)
	}
	return s, nil
}

// Append 在内存中记录一个轮次并写入一行 JSONL 到磁盘。
func (s *Session) Append(turn Turn) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turns = append(s.turns, turn)
	if s.w == nil {
		return nil // read-only session (Open without subsequent write)
	}
	rec := turnRecord{
		Type:        "turn",
		Role:        turn.Role,
		Content:     turn.Content,
		TS:          turn.Timestamp,
		Interrupted: turn.Interrupted,
	}
	if rec.TS.IsZero() {
		rec.TS = time.Now().UTC()
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if _, err := s.w.Write(data); err != nil {
		return err
	}
	if _, err := s.w.WriteString("\n"); err != nil {
		return err
	}
	return s.w.Flush()
}

// AppendToolCall 在内存中记录工具调用并写入一行 JSONL 到磁盘。
func (s *Session) AppendToolCall(tc ToolCall) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolCalls = append(s.toolCalls, tc)
	if s.w == nil {
		return nil
	}
	rec := toolCallRecord{
		Type:     "tool_call",
		CallID:   tc.CallID,
		ToolName: tc.ToolName,
		Args:     tc.Args,
		TS:       tc.TS,
	}
	if rec.TS.IsZero() {
		rec.TS = time.Now().UTC()
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if _, err := s.w.Write(data); err != nil {
		return err
	}
	if _, err := s.w.WriteString("\n"); err != nil {
		return err
	}
	return s.w.Flush()
}

// AppendToolResult 在内存中记录工具结果并写入一行 JSONL 到磁盘。
func (s *Session) AppendToolResult(tr ToolResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolResults = append(s.toolResults, tr)
	if s.w == nil {
		return nil
	}
	rec := toolResultRecord{
		Type:     "tool_result",
		CallID:   tr.CallID,
		ToolName: tr.ToolName,
		Result:   tr.Result,
		Summary:  tr.Summary,
		TS:       tr.TS,
	}
	if rec.TS.IsZero() {
		rec.TS = time.Now().UTC()
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if _, err := s.w.Write(data); err != nil {
		return err
	}
	if _, err := s.w.WriteString("\n"); err != nil {
		return err
	}
	return s.w.Flush()
}

// Close 刷新所有缓冲写入并关闭底层文件。对以只读方式打开的会话调用是安全的。
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.w != nil {
		if err := s.w.Flush(); err != nil {
			return err
		}
		s.w = nil
	}
	if s.file != nil {
		err := s.file.Close()
		s.file = nil
		return err
	}
	return nil
}

func (s *Session) writeMeta() error {
	rec := metaRecord{
		Type:     "meta",
		ID:       s.meta.ID,
		Created:  s.meta.Created,
		Protocol: s.meta.Protocol,
		Model:    s.meta.Model,
	}
	if rec.Created.IsZero() {
		rec.Created = time.Now().UTC()
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if _, err := s.w.Write(data); err != nil {
		return err
	}
	if _, err := s.w.WriteString("\n"); err != nil {
		return err
	}
	return s.w.Flush()
}

// idFromTimestamp 将时间格式化为 UTC 的 YYYYMMDD-HHMMSS。
func idFromTimestamp(t time.Time) string {
	return t.UTC().Format("20060102-150405")
}

// slugify 将自由格式字符串转换为最多 20 个字符的文件系统安全标识。
// 非字母数字字符变为 "-"，连续的 "-" 被合并，前后的 "-" 被去除。
func slugify(s string) string {
	const maxLen = 20
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			if b.Len() > 0 {
				// Avoid emitting consecutive separators.
				if !strings.HasSuffix(b.String(), "-") {
					b.WriteByte('-')
				}
			}
		}
		if b.Len() >= maxLen {
			break
		}
	}
	out := strings.TrimRight(b.String(), "-")
	return out
}
