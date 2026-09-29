//go:build fyne_gui

package ui

import (
	"bytes"
	"strings"
	"sync"
)

// LogBuf is a thread-safe append-only log for background tasks.
type LogBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *LogBuf) Append(line string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	line = strings.TrimRight(line, "\r\n")
	if line != "" {
		l.buf.WriteString(line)
	}
	l.buf.WriteByte('\n')
}

func (l *LogBuf) String() string {
	if l == nil {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

func (l *LogBuf) Clear() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf.Reset()
}
