package app

import (
	"fmt"
	"os"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

)

func (a *App) Quit() {
	a.quitOnce.Do(func() {
		start := time.Now()
		a.log.Info("app", "Quit requested — terminating process")
		a.svcNotifying("exiting")
		if ctx := a.getContext(); ctx != nil {
			// runtime.Quit внутри Wails — синхронный Invoke в главный поток
			// плюс OnBeforeClose под локами; застрявший главный поток делал
			// вызов вечным и процесс оставался зомби. Поэтому fire-and-forget:
			// срок завершения определяет только таймаут ниже, os.Exit(0)
			// обязан выполниться всегда.
			go runtime.Quit(ctx)
			select {
			case <-a.shutdownDone:
				a.log.Info("app", fmt.Sprintf("Quit: graceful shutdown done in %s", time.Since(start).Round(time.Millisecond)))
			case <-time.After(3 * time.Second):
				// Normal when window hidden in tray - Wails is slow to process quit.
				// Force exit is fine, not an error — не Warn, чтобы не мусорить
				// на вкладке «Ошибки» в CRM.
				a.log.Info("app", fmt.Sprintf("Quit: graceful shutdown timed out after %s — force exit", time.Since(start).Round(time.Millisecond)))
				// Wails-флоу не дошёл до Shutdown: чистим критичное сами.
				// Shutdown идемпотентен (shutdownOnce); если он застрял в
				// Wails-потоке — ждём максимум 12с (внутри само восстановление
				// DNS имеет кап 10с) и уходим принудительно.
				go a.Shutdown(ctx)
				select {
				case <-a.shutdownDone:
				case <-time.After(12 * time.Second):
					a.log.Warn("app", "Quit: Shutdown still stuck after 12s — exiting anyway")
				}
			}
		}
		// Гарантированно завершаем процесс (убивает горутину трея и фоновые задачи)
		os.Exit(0)
	})
}

func (a *App) ShowWindow() {
	if ctx := a.getContext(); ctx != nil {
		runtime.EventsEmit(ctx, "window:shown", nil)
	}
	a.windowMu.Lock()
	a.windowVisible = true
	a.windowMu.Unlock()
	if ctx := a.getContext(); ctx != nil {
		runtime.WindowShow(ctx)
	}
}

func (a *App) HideWindow() {
	a.windowMu.Lock()
	a.windowVisible = false
	a.windowMu.Unlock()
	if ctx := a.getContext(); ctx != nil {
		runtime.WindowHide(ctx)
	}
	// Фронт глушит тики/анимации, пока окно скрыто (иначе GPU грузится
	// невидимым дашбордом).
	a.emitSafe("window:hidden", nil)
}

func (a *App) ToggleWindow() {
	a.windowMu.Lock()
	visible := a.windowVisible
	a.windowMu.Unlock()
	if visible {
		a.HideWindow()
	} else {
		a.ShowWindow()
	}
}

func (a *App) Restart() {
	a.log.Info("app", "Restart requested — spawning new process")
	a.svcNotifying("restarting")
	exe, err := os.Executable()
	if err != nil {
		a.log.Error("app", "Restart: os.Executable() failed: "+err.Error())
		return
	}
	var args []string
	if !a.WindowShown() {
		args = append(args, "--hidden")
	}
	if err := spawnWaitAndStart(exe, args...); err != nil {
		a.log.Error("app", "Restart: failed to start new process: "+err.Error())
		return
	}
	a.Quit()
}

func (a *App) WindowShown() bool {
	a.windowMu.Lock()
	defer a.windowMu.Unlock()
	return a.windowVisible
}

// RequestWindowShow — показать окно по запросу фронта. При скрытом старте
// (--hidden, окно ещё не открывали) — no-op: иначе фронт «прорывал» окно из
// трея, а бумажный флаг windowVisible оставался false и гасил загрузку данных.
func (a *App) RequestWindowShow() map[string]interface{} {
	a.windowMu.Lock()
	hiddenStart := a.startHidden && !a.windowVisible
	a.windowMu.Unlock()
	if hiddenStart {
		return okRespWithData(map[string]interface{}{"shown": false})
	}
	a.ShowWindow()
	return okRespWithData(map[string]interface{}{"shown": true})
}
