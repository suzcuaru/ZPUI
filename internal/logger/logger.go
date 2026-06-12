package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Logger struct {
	mu       sync.Mutex
	baseDir  string
	maxFiles int
	files    map[string]*os.File
	stopCh   chan struct{}
}

func New(baseDir string, maxFiles int) (*Logger, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("create logs dir: %w", err)
	}

	l := &Logger{
		baseDir:  baseDir,
		maxFiles: maxFiles,
		files:    make(map[string]*os.File),
		stopCh:   make(chan struct{}),
	}

	go l.cleanupLoop()

	return l, nil
}

func (l *Logger) Close() {
	close(l.stopCh)
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, f := range l.files {
		f.Close()
	}
}

func (l *Logger) Info(category, msg string) {
	l.write("INFO", category, msg)
}

func (l *Logger) Error(category, msg string) {
	l.write("ERROR", category, msg)
}

func (l *Logger) Warn(category, msg string) {
	l.write("WARN", category, msg)
}

func (l *Logger) Debug(category, msg string) {
	l.write("DEBUG", category, msg)
}

func (l *Logger) Network(msg string) {
	l.write("INFO", "network", msg)
}

func (l *Logger) ZapretLog(msg string) {
	l.write("INFO", "zapret", msg)
}

func (l *Logger) WriteZapretOutput(line string) {
	l.ZapretLog(strings.TrimRight(line, "\r\n"))
}

func (l *Logger) write(level, category, msg string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	entry := fmt.Sprintf("[%s] [%s] [%s] %s\n", timestamp, level, category, msg)

	l.mu.Lock()
	defer l.mu.Unlock()

	fmt.Print(entry)

	cat := category
	if cat == "network" || cat == "proxy" {
		cat = "network"
	} else if cat == "zapret" || cat == "service" || cat == "strategy" || cat == "updater" {
		cat = "zapret"
	}

	if l.files[cat] == nil {
		l.openFile(cat)
	}

	if f, ok := l.files[cat]; ok {
		f.WriteString(entry)
	}
}

func (l *Logger) openFile(category string) {
	dateStr := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("%s_%s.log", dateStr, category)
	path := filepath.Join(l.baseDir, filename)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open log file %s: %v\n", path, err)
		return
	}

	l.files[category] = f
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

	entries, err := os.ReadDir(l.baseDir)
	if err != nil {
		return
	}

	type logFile struct {
		name    string
		modTime time.Time
	}

	var files []logFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, logFile{name: e.Name(), modTime: info.ModTime()})
	}

	cutoff := time.Now().AddDate(0, 0, -l.maxFiles)
	for _, f := range files {
		if f.modTime.Before(cutoff) {
			os.Remove(filepath.Join(l.baseDir, f.name))
		}
	}

	cutOff := time.Now().AddDate(0, 0, -1)
	for cat, f := range l.files {
		fi, err := f.Stat()
		if err != nil {
			continue
		}
		dateStr := fi.ModTime().Format("2006-01-02")
		today := time.Now().Format("2006-01-02")
		if dateStr != today {
			f.Close()
			delete(l.files, cat)
		}
		_ = cutOff
	}
}

type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

func (l *Logger) ReadRecent(category string, lines int) []LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.files[category] != nil {
		l.files[category].Sync()
	}

	dateStr := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("%s_%s.log", dateStr, category)
	path := filepath.Join(l.baseDir, filename)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	allLines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	start := 0
	if len(allLines) > lines {
		start = len(allLines) - lines
	}

	var entries []LogEntry
	for _, line := range allLines[start:] {
		e := parseLine(line)
		if e != nil {
			entries = append(entries, *e)
		}
	}
	return entries
}

func parseLine(line string) *LogEntry {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	if len(line) > 21 && line[0] == '[' {
		end1 := strings.Index(line, "]")
		if end1 > 0 {
			timeStr := line[1:end1]
			rest := line[end1+1:]
			if len(rest) > 2 && rest[0] == ' ' && rest[1] == '[' {
				end2 := strings.Index(rest[2:], "]")
				if end2 > 0 {
					level := rest[2 : 2+end2]
					rest2 := rest[2+end2+1:]
					if len(rest2) > 2 && rest2[0] == ' ' && rest2[1] == '[' {
						end3 := strings.Index(rest2[2:], "]")
						if end3 > 0 {
							msg := strings.TrimPrefix(rest2[2+end3+1:], " ")
							return &LogEntry{Time: timeStr, Level: level, Message: msg}
						}
					}
					msg := strings.TrimLeft(rest2, " []")
					return &LogEntry{Time: timeStr, Level: level, Message: msg}
				}
			}
		}
	}
	return &LogEntry{Time: "", Level: "INFO", Message: line}
}

func (l *Logger) ListLogFiles() []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	entries, err := os.ReadDir(l.baseDir)
	if err != nil {
		return nil
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".log") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

func (l *Logger) ReadLogFile(name string) (string, error) {
	path := filepath.Join(l.baseDir, filepath.Base(name))
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
