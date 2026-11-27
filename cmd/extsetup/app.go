package main

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/extension"
	"zpui/internal/pkginst"
)

// App — биндинги установщика расширения для фронтенда.
// Ответы — в формате приложения: {"status":"ok"} / {"error": "..."}.
type App struct {
	ctx     context.Context
	mu      sync.Mutex
	ver     string // версия установщика (= версия расширения из build.bat)
	extVer  string // версия встроенного CRX
	dir     string // каталог пакета: %LOCALAPPDATA%\ZPUI\extension или -target
	sandbox bool   // -target: без записей в реестр
}

func newApp(ver, extVer, dir string, sandbox bool) *App {
	return &App{ver: ver, extVer: extVer, dir: dir, sandbox: sandbox}
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

func okMsg() map[string]interface{} {
	return map[string]interface{}{"status": "ok"}
}

func errMsg(msg string) map[string]interface{} {
	return map[string]interface{}{"error": msg}
}

func failsMsg(fails map[string]string, prefix string) string {
	msgs := make([]string, 0, len(fails))
	for id, msg := range fails {
		msgs = append(msgs, id+": "+msg)
	}
	return prefix + strings.Join(msgs, "; ")
}

// Info — версии, каталог пакета и состояние расширения в браузерах.
func (a *App) Info() map[string]interface{} {
	return map[string]interface{}{
		"version":    a.ver,
		"extVersion": a.extVer,
		"browsers":   extension.List(""),
		"sandbox":    a.sandbox,
		"dir":        a.dir,
	}
}

// Install — регистрирует пакет (уже сохранён на диске) в выбранных браузерах.
// Браузер подхватывает расширение при следующем старте.
func (a *App) Install(ids []string) map[string]interface{} {
	if len(ids) == 0 {
		return errMsg("выберите хотя бы один браузер")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sandbox {
		return okMsg()
	}
	if fails := extension.InstallPath(filepath.Join(a.dir, extension.File), a.extVer, ids); len(fails) > 0 {
		return errMsg(failsMsg(fails, "не удалось зарегистрировать: "))
	}
	return okMsg()
}

// Uninstall — снимает регистрацию расширения в выбранных браузерах
// (браузер уберёт его при старте).
func (a *App) Uninstall(ids []string) map[string]interface{} {
	if len(ids) == 0 {
		return errMsg("выберите хотя бы один браузер")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sandbox {
		return okMsg()
	}
	if fails := extension.Uninstall(ids); len(fails) > 0 {
		return errMsg(failsMsg(fails, "не удалось снять регистрацию: "))
	}
	return okMsg()
}

// OpenPage — страница расширений браузера на карточке ZPUI: там пользователь
// включает внешнее расширение (один раз после установки/обновления).
func (a *App) OpenPage(id string) map[string]interface{} {
	if err := extension.OpenExtensionsPage(id); err != nil {
		return errMsg(err.Error())
	}
	return okMsg()
}

// Minimize сворачивает окно (кастомный тайтлбар).
func (a *App) Minimize() {
	runtime.WindowMinimise(a.ctx)
}

// Close закрывает окно.
func (a *App) Close() {
	runtime.Quit(a.ctx)
}
