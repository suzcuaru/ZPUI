package main

import (
	"context"
	"os"
	"os/exec"
	"sync"
	"time"
	"syscall"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/pkginst"
)

// App — биндинги деинсталлятора.
type App struct {
	ctx     context.Context
	mu      sync.Mutex
	running bool
	dir     string
	ver     string
}

func newApp(dir, ver string) *App {
	return &App{dir: dir, ver: ver}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	go func() {
		for i := 0; i < 10; i++ {
			time.Sleep(200 * time.Millisecond)
			if pkginst.RoundOwnWindow() {
				return
			}
		}
	}()
}

// Info — что удаляем.
func (a *App) Info() map[string]string {
	return map[string]string{"dir": a.dir, "ver": a.ver}
}

// Minimize сворачивает окно.
func (a *App) Minimize() {
	runtime.WindowMinimise(a.ctx)
}

// Close закрывает окно; во время удаления — запрещено.
func (a *App) Close() {
	a.mu.Lock()
	running := a.running
	a.mu.Unlock()
	if running {
		return
	}
	runtime.Quit(a.ctx)
}

// Start выполняет удаление; прогресс — событиями pkg:step / pkg:done.
func (a *App) Start(keepData bool) {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return
	}
	a.running = true
	a.mu.Unlock()

	go func() {
		emit := func(phase string, value float64, detail string) {
			runtime.EventsEmit(a.ctx, "pkg:step", map[string]interface{}{
				"phase": phase, "value": value, "detail": detail,
			})
		}
		emit("Остановка ZPUI…", 0.15, "")
		pkginst.RemoveInstallation(a.dir, !keepData)
		emit("Очистка реестра и ярлыков…", 0.85, "")
		deleteSelfLater()
		emit("Готово", 1.0, "")
		runtime.EventsEmit(a.ctx, "pkg:done", map[string]interface{}{"ok": true})
	}()
}

// Finish закрывает деинсталлятор.
func (a *App) Finish() {
	runtime.Quit(a.ctx)
}

// deleteSelfLater удаляет собственный exe (занят работающим процессом) —
// отсоединённый помощник доделает это после закрытия окна.
func deleteSelfLater() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command("cmd", "/C", "ping -n 2 127.0.0.1 > nul & del /f /q \""+exe+"\"")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	_ = cmd.Start()
}
