package main

import (
	"context"
	_ "embed"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/pkginst"
)

// App — биндинги обновлятора для фронтенда.
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

//go:embed changelog.txt
var changelogText string

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

// Info — данные для плашки, текстов и панель «Что нового» (из CHANGELOG).
func (a *App) Info() map[string]string {
	changelog := changelogText
	if strings.TrimSpace(changelog) == "" || strings.Contains(changelog, "наполняется из CHANGELOG.md") {
		changelog = "Список изменений будет добавлен в релизной сборке."
	}
	return map[string]string{
		"dir": a.dir, "old": a.oldVer, "new": a.ver, "changelog": changelog,
	}
}

// Minimize сворачивает окно (кастомный тайтлбар).
func (a *App) Minimize() {
	runtime.WindowMinimise(a.ctx)
}

// Close закрывает окно; посреди обновления — запрещено.
func (a *App) Close() {
	a.mu.Lock()
	running := a.running
	a.mu.Unlock()
	if running {
		return
	}
	runtime.Quit(a.ctx)
}

// Start запускает обновление; прогресс — событиями pkg:step / pkg:done.
func (a *App) Start() {
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
				// в песочнице фазы искусственно замедлены — для скриншотов
				time.Sleep(900 * time.Millisecond)
			}
			runtime.EventsEmit(a.ctx, "pkg:step", map[string]interface{}{
				"phase": phase, "value": value, "detail": detail,
			})
		}
		err := pkginst.UpdateExisting(a.payload, a.dir, a.ver, emit)
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

// Finish завершает работу обновлятора; run — запустить приложение.
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

// Retry повторяет запуск логики после ошибки (кнопка «Повторить»).
func (a *App) Retry() {}

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
