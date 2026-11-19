package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type StartupLogger struct {
	mu   sync.Mutex
	f    *os.File
	path string
}

func NewStartupLogger(logDir string) (*StartupLogger, error) {
	os.MkdirAll(logDir, 0755)
	path := filepath.Join(logDir, "startup.log")
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &StartupLogger{f: f, path: path}, nil
}

func (l *StartupLogger) Path() string { return l.path }

func (l *StartupLogger) Write(step, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	line := fmt.Sprintf("[%s] [%s] %s\n", time.Now().Format("15:04:05.000"), step, msg)
	l.f.WriteString(line)
	l.f.Sync()
}

func (l *StartupLogger) Writef(step, format string, args ...interface{}) {
	l.Write(step, fmt.Sprintf(format, args...))
}

func (l *StartupLogger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f != nil {
		l.f.Close()
		l.f = nil
	}
}
