// Package notify отправляет Windows 10/11 toast-уведомления через WinRT COM API
// (pure-Go, без запуска PowerShell). При первом вызове создаётся ярлык в Start Menu
// с AUMID (требование WinRT для Win32-приложений) и регистрируются метаданные.
// Все COM-вызовы выполняются на одном закреплённом потоке (LockOSThread).
package notify

import (
	"os"
	"regexp"
	"runtime"
	"sync"

	toast "git.sr.ht/~jackmordaunt/go-toast/v2"
	"zpui/internal/logger"
)

var (
	notifyLogger *logger.Logger
	logMu        sync.RWMutex
)

func SetLogger(l *logger.Logger) {
	logMu.Lock()
	notifyLogger = l
	logMu.Unlock()
}

func logWarn(msg string) {
	logMu.RLock()
	l := notifyLogger
	logMu.RUnlock()
	if l != nil {
		l.Warn("notify", msg)
	}
}

func logError(msg string) {
	logMu.RLock()
	l := notifyLogger
	logMu.RUnlock()
	if l != nil {
		l.Error("notify", msg)
	}
}

const appAUMID = "ZPUI"

var safePattern = regexp.MustCompile(`[^a-zA-Z0-9а-яА-ЯёЁ .,!:;?\-\(\)%+]`)

func sanitize(s string) string {
	return safePattern.ReplaceAllString(s, "")
}

type message struct {
	title string
	body  string
}

var (
	setupOnce sync.Once
	queue     chan message
)

// Show отправляет toast-уведомление (неблокирующий fire-and-forget).
func Show(title, body string) error {
	t := sanitize(title)
	b := sanitize(body)
	if t == "" && b == "" {
		return nil
	}
	setupOnce.Do(func() {
		queue = make(chan message, 32)
		go runWorker(queue)
	})
	select {
	case queue <- message{title: t, body: b}:
	default:
		logWarn("queue full, notification dropped")
	}
	return nil
}

func runWorker(q <-chan message) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	registerApp()

	for msg := range q {
		n := toast.Notification{
			AppID: appAUMID,
			Title: msg.title,
			Body:  msg.body,
			Audio: toast.Default,
		}
		if err := n.Push(); err != nil {
			logError("push error: " + err.Error())
		}
	}
}

func registerApp() {
	data := toast.AppData{
		AppID:               appAUMID,
		IconBackgroundColor: "#FF6200EE",
	}
	if exe, err := os.Executable(); err == nil {
		data.ActivationExe = exe
		if err := ensureStartMenuShortcut(exe, appAUMID); err != nil {
			logWarn("shortcut error: " + err.Error())
		}
	}
	if err := toast.SetAppData(data); err != nil {
		logWarn("SetAppData error: " + err.Error())
	}
}
