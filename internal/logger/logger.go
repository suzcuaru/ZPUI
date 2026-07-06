package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const ringMax = 2000

type Logger struct {
	mu      sync.Mutex
	baseDir string
	maxDays int
	file    *os.File
	date    string
	ring    []entry
	stopCh  chan struct{}
}

type entry struct {
	t        time.Time
	level    string
	category string
	msg      string
}

type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

func New(baseDir string, maxDays int) (*Logger, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("create logs dir: %w", err)
	}
	if maxDays <= 0 {
		maxDays = 7
	}
	l := &Logger{
		baseDir: baseDir,
		maxDays: maxDays,
		stopCh:  make(chan struct{}),
	}
	go l.cleanupLoop()
	return l, nil
}

func (l *Logger) Close() {
	select {
	case <-l.stopCh:
	default:
		close(l.stopCh)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		l.file.Close()
	}
}

func (l *Logger) Info(category, msg string)  { l.write("INFO", category, msg) }
func (l *Logger) Warn(category, msg string)  { l.write("WARN", category, msg) }
func (l *Logger) Error(category, msg string) { l.write("ERROR", category, msg) }

func (l *Logger) write(level, category, msg string) {
	now := time.Now()
	line := fmt.Sprintf("[%s] [%s] [%s] %s\n", now.Format("2006-01-02 15:04:05"), level, category, msg)

	l.mu.Lock()
	defer l.mu.Unlock()

	fmt.Print(line)

	l.ring = append(l.ring, entry{now, level, category, msg})
	if len(l.ring) > ringMax {
		l.ring = l.ring[len(l.ring)-ringMax:]
	}

	today := now.Format("2006-01-02")
	if l.file != nil && l.date != today {
		l.file.Close()
		l.file = nil
	}
	if l.file == nil {
		l.openFile(today)
	}
	if l.file != nil {
		l.file.WriteString(line)
	}
}

func (l *Logger) openFile(today string) {
	path := filepath.Join(l.baseDir, "app.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open log: %v\n", err)
		return
	}
	l.file = f
	l.date = today
}

func (l *Logger) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-l.stopCh:
			return
		case <-ticker.C:
			l.cleanup()
		}
	}
}

func (l *Logger) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().AddDate(0, 0, -l.maxDays)
	if entries, err := os.ReadDir(l.baseDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
				continue
			}
			if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
				os.Remove(filepath.Join(l.baseDir, e.Name()))
			}
		}
	}
}

func (l *Logger) Recent(category string, lines int) []LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	var out []LogEntry
	if category == "" {
		for _, e := range l.ring {
			out = append(out, LogEntry{e.t.Format("2006-01-02 15:04:05"), e.level, e.msg})
		}
	} else {
		for _, e := range l.ring {
			if e.category == category {
				out = append(out, LogEntry{e.t.Format("2006-01-02 15:04:05"), e.level, e.msg})
			}
		}
	}
	if len(out) > lines {
		out = out[len(out)-lines:]
	}
	return out
}
