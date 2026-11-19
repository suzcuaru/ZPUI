package zsvc

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	logMaxSize = 512 * 1024
)

// SvcLog — журнал службы: append-файл с ротацией service.log → .1 → .2
// при превышении 512KB (диагностика «что было вчера» не стирается).
type SvcLog struct {
	mu   sync.Mutex
	path string
}

// OpenLog открывает журнал: <InstallDir>\logs\service.log (путь установки из
// реестра), при недоступности — <exeDir>\service.log рядом с exe службы.
func OpenLog() *SvcLog {
	path := ""
	if dir := InstallDir(); dir != "" {
		path = filepath.Join(dir, "logs", "service.log")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			path = ""
		}
	}
	if path == "" {
		if exe, err := os.Executable(); err == nil {
			path = filepath.Join(filepath.Dir(exe), "service.log")
		} else {
			path = "service.log"
		}
	}
	return &SvcLog{path: path}
}

// Path — фактический путь файла журнала.
func (l *SvcLog) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

func (l *SvcLog) writef(format string, args ...interface{}) {
	if l == nil {
		return
	}
	line := fmt.Sprintf("%s %s\n", time.Now().Format("2006-01-02 15:04:05.000"), fmt.Sprintf(format, args...))
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rotateLocked()
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line)
}

// rotateLocked сдвигает переполнившийся журнал: .1 → .2, текущий → .1.
func (l *SvcLog) rotateLocked() {
	st, err := os.Stat(l.path)
	if err != nil || st.Size() <= logMaxSize {
		return
	}
	_ = os.Remove(l.path + ".2")
	_ = os.Rename(l.path+".1", l.path+".2")
	_ = os.Rename(l.path, l.path+".1")
}

// Info пишет информационную строку в журнал.
func (l *SvcLog) Info(msg string) { l.writef("INFO %s", msg) }

// Warn пишет предупреждение в журнал.
func (l *SvcLog) Warn(msg string) { l.writef("WARN %s", msg) }

// Error пишет строку об ошибке в журнал.
func (l *SvcLog) Error(msg string) { l.writef("ERROR %s", msg) }

// Debug пишет отладочную строку в журнал.
func (l *SvcLog) Debug(msg string) { l.writef("DEBUG %s", msg) }
