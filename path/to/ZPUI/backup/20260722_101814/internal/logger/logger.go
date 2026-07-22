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

// ringMax — верхний предел in-memory кольцевого буфера (записи всех категорий).
const ringMax = 5000

// ringWindow — глубина среза ошибок (логи за этот период до ошибки).
const ringWindow = 1 * time.Hour

// snapshotDebounce — минимальный интервал между срезами ошибок (анти-спам).
const snapshotDebounce = 30 * time.Second

// errRetention — сколько дней хранить файлы срезов ошибок.
const errRetention = 30

type Logger struct {
	mu              sync.Mutex
	baseDir         string
	maxDays         int
	files           map[string]*os.File
	fileDates       map[string]string
	ring            []ringEntry
	lastSnap        time.Time
	debugCategories map[string]bool
	stopCh          chan struct{}

	// OnError callback - called when an ERROR-level message is logged.
	// Used by App to show desktop notifications (if NotifyErrors is enabled).
	onError         func(category, msg string)
}

type ringEntry struct {
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

// New создаёт логгер. baseDir — каталог логов, maxDays — срок хранения архивов.
func New(baseDir string, maxDays int) (*Logger, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("create logs dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "archive"), 0755); err != nil {
		return nil, fmt.Errorf("create archive dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "errors"), 0755); err != nil {
		return nil, fmt.Errorf("create errors dir: %w", err)
	}

	if maxDays <= 0 {
		maxDays = 7
	}

	l := &Logger{
		baseDir:         baseDir,
		maxDays:         maxDays,
		files:           make(map[string]*os.File),
		fileDates:       make(map[string]string),
		debugCategories: make(map[string]bool),
		stopCh:          make(chan struct{}),
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

// SetOnError registers a callback invoked whenever an ERROR is logged.
// Useful for showing desktop notifications on critical errors.
func (l *Logger) SetOnError(fn func(category, msg string)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onError = fn
}

func (l *Logger) Error(category, msg string) {
	l.write("ERROR", category, msg)
}

func (l *Logger) Warn(category, msg string) {
	l.write("WARN", category, msg)
}

func (l *Logger) Debug(category, msg string) {
	l.mu.Lock()
	debug := l.debugCategories[category]
	l.mu.Unlock()
	if !debug {
		return
	}
	l.write("DEBUG", category, msg)
}

func (l *Logger) Network(msg string) {
	l.write("DEBUG", "network", msg)
}

func (l *Logger) ZapretLog(msg string) {
	l.write("INFO", "zapret", msg)
}

func (l *Logger) WriteZapretOutput(line string) {
	l.Debug("zapret", strings.TrimRight(line, "\r\n"))
}

func (l *Logger) SetDebug(category string, enabled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.debugCategories[category] = enabled
}

func (l *Logger) IsDebug(category string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.debugCategories[category]
}

func (l *Logger) GetDebugCategories() map[string]bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make(map[string]bool)
	for k, v := range l.debugCategories {
		result[k] = v
	}
	return result
}

// mapBucket сворачивает категорию в физический файл-бакет.
func mapBucket(category string) string {
	switch category {
	case "network", "proxy":
		return "network"
	case "zapret", "service", "strategy", "updater", "install":
		return "zapret"
	default:
		return category
	}
}

func (l *Logger) write(level, category, msg string) {
	now := time.Now()
	timestamp := now.Format("2006-01-02 15:04:05")
	entry := fmt.Sprintf("[%s] [%s] [%s] %s\n", timestamp, level, category, msg)

	l.mu.Lock()
	defer l.mu.Unlock()

	fmt.Print(entry)

	// Кольцевой буфер
	l.ring = append(l.ring, ringEntry{t: now, level: level, category: category, msg: msg})
	if len(l.ring) > ringMax {
		l.ring = l.ring[len(l.ring)-ringMax:]
	}

	bucket := mapBucket(category)
	today := now.Format("2006-01-02")

	// Ротация: если файл открыт для другой даты — архивируем
	if l.files[bucket] != nil && l.fileDates[bucket] != today {
		l.files[bucket].Close()
		delete(l.files, bucket)
	}
	if l.files[bucket] == nil {
		l.openFile(bucket, today)
	}
	if f, ok := l.files[bucket]; ok {
		f.WriteString(entry)
	}

	// Срез ошибки
	if level == "ERROR" {
		l.flushErrorSnapshot(now, category, msg)
		// Trigger OnError callback (for desktop notifications)
		if l.onError != nil {
			go l.onError(category, msg)
		}
	}
}

// openFile открывает (или создаёт) файл бакета. Если существует устаревший
// файл — он переносится в archive/ перед созданием нового.
func (l *Logger) openFile(bucket, today string) {
	path := filepath.Join(l.baseDir, bucket+".log")

	if info, err := os.Stat(path); err == nil {
		fileDate := info.ModTime().Format("2006-01-02")
		if fileDate != today {
			archiveDir := filepath.Join(l.baseDir, "archive")
			os.MkdirAll(archiveDir, 0755)
			archivePath := filepath.Join(archiveDir, fmt.Sprintf("%s-%s.log", bucket, fileDate))
			os.Rename(path, archivePath)
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open log file %s: %v\n", path, err)
		return
	}
	l.files[bucket] = f
	l.fileDates[bucket] = today
}

// flushErrorSnapshot создаёт файл в errors/ с логами за последний час.
func (l *Logger) flushErrorSnapshot(now time.Time, category, msg string) {
	if now.Sub(l.lastSnap) < snapshotDebounce {
		return
	}
	l.lastSnap = now

	cutoff := now.Add(-ringWindow)
	var entries []ringEntry
	for _, e := range l.ring {
		if e.t.After(cutoff) {
			entries = append(entries, e)
		}
	}

	errorsDir := filepath.Join(l.baseDir, "errors")
	os.MkdirAll(errorsDir, 0755)

	fname := fmt.Sprintf("error-%s.log", now.Format("2006-01-02_150405"))
	path := filepath.Join(errorsDir, fname)

	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "=== СРЕЗ ОШИБКИ ===\n")
	fmt.Fprintf(f, "Время:  %s\n", now.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(f, "Кат-я:  %s\n", category)
	fmt.Fprintf(f, "Ошибка: %s\n", msg)
	fmt.Fprintf(f, "Записей за последний час: %d\n", len(entries))
	fmt.Fprintf(f, "\n--- Хронология логов (за час до ошибки) ---\n\n")

	for _, e := range entries {
		ts := e.t.Format("2006-01-02 15:04:05")
		marker := ""
		if e.level == "ERROR" {
			marker = " <<<"
		}
		fmt.Fprintf(f, "[%s] [%s] [%s] %s%s\n", ts, e.level, e.category, e.msg, marker)
	}
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

	cutoffDate := time.Now().AddDate(0, 0, -l.maxDays)

	// Архивы старше maxDays
	archiveDir := filepath.Join(l.baseDir, "archive")
	if entries, err := os.ReadDir(archiveDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoffDate) {
				os.Remove(filepath.Join(archiveDir, e.Name()))
			}
		}
	}

	// Срезы ошибок старше errRetention дней
	errCutoff := time.Now().AddDate(0, 0, -errRetention)
	errorsDir := filepath.Join(l.baseDir, "errors")
	if entries, err := os.ReadDir(errorsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(errCutoff) {
				os.Remove(filepath.Join(errorsDir, e.Name()))
			}
		}
	}
}

func (l *Logger) ReadRecent(category string, lines int) []LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	bucket := mapBucket(category)
	if f := l.files[bucket]; f != nil {
		f.Sync()
	}

	path := filepath.Join(l.baseDir, bucket+".log")
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

func (l *Logger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.ring = nil
}

// ClearBucket clears a specific log bucket (file + in-memory entries).
// bucket = "zapret" / "network" / "app" / "availability" / "tray" / "xboxdns" / ...
// Ring entries for this category are removed, file is truncated to 0 bytes.
func (l *Logger) ClearBucket(bucket string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 1. Remove entries from ring buffer
	filtered := l.ring[:0]
	for _, e := range l.ring {
		if mapBucket(e.category) != bucket {
			filtered = append(filtered, e)
		}
	}
	l.ring = filtered

	// 2. Close file if open, then truncate
	if f, ok := l.files[bucket]; ok {
		f.Close()
		delete(l.files, bucket)
		delete(l.fileDates, bucket)
	}
	path := filepath.Join(l.baseDir, bucket+".log")
	if err := os.Truncate(path, 0); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ClearAll clears all log buckets (in-memory + all .log files in baseDir).
// Archives and error snapshots are NOT touched.
func (l *Logger) ClearAll() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 1. Clear ring buffer entirely
	l.ring = nil

	// 2. Close all open files
	for _, f := range l.files {
		f.Close()
	}
	l.files = make(map[string]*os.File)
	l.fileDates = make(map[string]string)

	// 3. Truncate all .log files in baseDir
	entries, err := os.ReadDir(l.baseDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		path := filepath.Join(l.baseDir, e.Name())
		if err := os.Truncate(path, 0); err != nil && !os.IsNotExist(err) {
			continue
		}
	}
	return nil
}

// ListLogFiles возвращает все доступные лог-файлы: текущие, архивы и срезы ошибок.
func (l *Logger) ListLogFiles() []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	var names []string

	// Текущие логи (корень baseDir)
	if entries, err := os.ReadDir(l.baseDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".log") {
				names = append(names, e.Name())
			}
		}
	}

	// Архивы
	archiveDir := filepath.Join(l.baseDir, "archive")
	if entries, err := os.ReadDir(archiveDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".log") {
				names = append(names, "archive/"+e.Name())
			}
		}
	}

	// Срезы ошибок
	errorsDir := filepath.Join(l.baseDir, "errors")
	if entries, err := os.ReadDir(errorsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".log") {
				names = append(names, "errors/"+e.Name())
			}
		}
	}

	sort.Strings(names)
	return names
}

// ReadLogFile читает лог-файл по имени. Поддерживает подпапки archive/ и errors/.
func (l *Logger) ReadLogFile(name string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(name))
	if clean == "" || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("invalid log file path")
	}

	path := filepath.Join(l.baseDir, clean)
	resolved, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	baseAbs, _ := filepath.Abs(l.baseDir)
	if !strings.HasPrefix(resolved, baseAbs) {
		return "", fmt.Errorf("path outside logs dir")
	}

	f, err := os.Open(resolved)
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
