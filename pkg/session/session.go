// Package session maintains an in-memory turn list and persists it to a JSONL file.
// Each session has a metadata header (first line) followed by one record per turn.
// Writes are serialized with a mutex; concurrent appends never interleave on disk.
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

// Metadata is the JSONL header written when a session is created.
type Metadata struct {
	ID       string    `json:"id"`
	Created  time.Time `json:"created"`
	Protocol string    `json:"protocol"`
	Model    string    `json:"model"`
}

// Turn is one user or assistant message. The Timestamp/Interrupted fields are
// optional and populated when the value is non-zero / true.
type Turn struct {
	Role        string
	Content     string
	Timestamp   time.Time
	Interrupted bool
}

// Snapshot returns a copy of the in-memory turn list. Callers may mutate the result
// without affecting the session.
func (s *Session) Snapshot() []Turn {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Turn, len(s.turns))
	copy(out, s.turns)
	return out
}

// ID returns the session identifier (timestamp + slug).
func (s *Session) ID() string {
	return s.meta.ID
}

// Path returns the JSONL file path on disk.
func (s *Session) Path() string {
	return s.path
}

// metaRecord is the on-disk shape of the first JSONL line.
type metaRecord struct {
	Type     string    `json:"type"`
	ID       string    `json:"id"`
	Created  time.Time `json:"created"`
	Protocol string    `json:"protocol"`
	Model    string    `json:"model"`
}

// turnRecord is the on-disk shape of every subsequent line.
type turnRecord struct {
	Type       string    `json:"type"`
	Role       string    `json:"role"`
	Content    string    `json:"content"`
	TS         time.Time `json:"ts,omitempty"`
	Interrupted bool     `json:"interrupted,omitempty"`
}

// Session holds the current in-memory turns and an open JSONL file for appends.
// Use New to create a fresh session, Open to resume an existing one.
type Session struct {
	path string
	meta Metadata
	mu   sync.Mutex
	turns []Turn
	file  *os.File
	w     *bufio.Writer
}

// New creates a new session at path, writes the metadata header, and returns the
// session ready for Append. The parent directory must already exist.
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

// Open reads an existing session file. Lines that fail to parse are silently
// skipped (a warning is logged via the caller's logx package, but session keeps
// a minimal contract: no errors bubble up for corrupt lines).
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
		}
		// else: skip unparseable line
	}
	if err := scanner.Err(); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("scan: %w", err)
	}
	return s, nil
}

// Append records a turn in memory and writes a JSONL line to disk.
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

// Close flushes any buffered writes and closes the underlying file. It is safe
// to call on a session that was opened read-only.
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

// idFromTimestamp formats a time as YYYYMMDD-HHMMSS in UTC.
func idFromTimestamp(t time.Time) string {
	return t.UTC().Format("20060102-150405")
}

// slugify converts a free-form string to a filesystem-safe slug of up to 20 chars.
// Non-alphanumeric runes become "-", runs of "-" are collapsed, and leading/trailing
// "-" are trimmed.
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
