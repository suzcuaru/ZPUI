package main

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/pkginst"
)

// App — биндинги установщика для фронтенда.
type App struct {
	ctx     context.Context
	mu      sync.Mutex
	running bool

	payload *pkginst.Payload
	dir     string
	oldVer  string
	ver     string
	sandbox bool
}

func newApp(payload *pkginst.Payload, dir, oldVer, ver string, sandbox bool) *App {
	return &App{payload: payload, dir: dir, oldVer: oldVer, ver: ver, sandbox: sandbox}
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

// Info — версия, найденная установка, текст лицензии, список изменений.
func (a *App) Info() map[string]interface{} {
	license := licenseText
	if license == "" || license == "PLACEHOLDER\n" {
		license = "Лицензионное соглашение будет добавлено в релизной сборке.\n(файл LICENSE подставляется build.bat)"
	}
	changelog := changelogText
	if strings.TrimSpace(changelog) == "" || strings.Contains(changelog, "наполняется из CHANGELOG.md") {
		changelog = "Список изменений будет добавлен в релизной сборке."
	}
	return map[string]interface{}{
		"new":       a.ver,
		"old":       a.oldVer,
		"hasOld":    a.oldVer != "",
		"dir":       a.dir,
		"license":   license,
		"changelog": changelog,
	}
}

// Minimize сворачивает окно (кастомный тайтлбар).
func (a *App) Minimize() {
	runtime.WindowMinimise(a.ctx)
}

// Close закрывает окно; во время установки — запрещено.
func (a *App) Close() {
	a.mu.Lock()
	running := a.running
	a.mu.Unlock()
	if running {
		return
	}
	runtime.Quit(a.ctx)
}

// Start запускает установку. mode: fresh | update | clean;
// desktop/startMenu — выбранные в мастере ярлыки.
func (a *App) Start(mode string, desktop bool, startMenu bool) {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return
	}
	a.running = true
	a.mu.Unlock()

	go func() {
		emit := func(phase string, value float64, detail string) {
			if a.sandbox {
				time.Sleep(700 * time.Millisecond)
			}
			runtime.EventsEmit(a.ctx, "pkg:step", map[string]interface{}{
				"phase": phase, "value": value, "detail": detail,
			})
		}
		var err error
		switch mode {
		case "update":
			err = pkginst.UpdateExisting(a.payload, a.dir, a.ver, emit)
		case "clean":
			err = pkginst.InstallFresh(a.payload, a.dir, a.ver, true, desktop, startMenu, emit)
		default:
			err = pkginst.InstallFresh(a.payload, a.dir, a.ver, false, desktop, startMenu, emit)
		}
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()
		if err == nil {
			runtime.EventsEmit(a.ctx, "pkg:done", map[string]interface{}{"ok": true})
			return
		}
		runtime.EventsEmit(a.ctx, "pkg:done", map[string]interface{}{"ok": false, "err": err.Error()})
	}()
}

// Finish завершает мастер; run — запустить приложение.
func (a *App) Finish(run bool) {
	a.mu.Lock()
	wasRunning := a.running
	a.mu.Unlock()
	if wasRunning {
		return
	}
	if run && !a.sandbox {
		pkginst.LaunchApp(a.dir)
	}
	runtime.Quit(a.ctx)
}

// openPayload открывает встроенный пакет или файл (-payload, для отладки).
func openPayload(override string) (pkginst.Payload, error) {
	if override != "" {
		f, err := os.Open(override)
		if err != nil {
			return pkginst.Payload{}, err
		}
		st, err := f.Stat()
		if err != nil {
			f.Close()
			return pkginst.Payload{}, err
		}
		return pkginst.Payload{Reader: f, Size: st.Size()}, nil
	}
	f, err := payloadFS.Open("payload.zip")
	if err != nil {
		return pkginst.Payload{}, err
	}
	ff, ok := f.(io.ReaderAt)
	if !ok {
		return pkginst.Payload{}, io.ErrUnexpectedEOF
	}
	st, err := f.Stat()
	if err != nil {
		return pkginst.Payload{}, err
	}
	return pkginst.Payload{Reader: ff, Size: st.Size()}, nil
}
