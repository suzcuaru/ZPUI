package app

import (
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// WindowClose закрывает окно с семантикой нативного крестика: close-to-tray
// прячет окно в трей, иначе приложение завершается.
func (a *App) WindowClose() map[string]interface{} {
	if a.ctx != nil {
		a.BeforeClose(a.ctx)
	}
	return okResp()
}

// WindowMinimise сворачивает окно (кнопка тайтлбара).
func (a *App) WindowMinimise() map[string]interface{} {
	if a.ctx != nil {
		runtime.WindowMinimise(a.ctx)
	}
	return okResp()
}

// WindowToggleMaximise разворачивает/восстанавливает окно (кнопка тайтлбара).
func (a *App) WindowToggleMaximise() map[string]interface{} {
	if a.ctx != nil {
		runtime.WindowToggleMaximise(a.ctx)
	}
	return okResp()
}

// WindowIsMaximised возвращает фактическое состояние окна, чтобы UI не расходился с ОС.
func (a *App) WindowIsMaximised() map[string]interface{} {
	maximised := true
	if a.ctx != nil {
		maximised = runtime.WindowIsMaximised(a.ctx)
	}
	return okRespWithData(map[string]interface{}{"maximised": maximised})
}

// WindowIsShown — видно ли окно (старт с --hidden: фронт сразу глушит тики).
func (a *App) WindowIsShown() map[string]interface{} {
	return okRespWithData(map[string]interface{}{"shown": a.WindowShown()})
}
