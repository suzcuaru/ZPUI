package app

// Кастомные темы оформления: слоты галереи «Свои темы» + активный слот.
// Контракт зеркалирует мок UI-стенда (mockups/app-ui/src/stand-mock.js):
// активация слота ставит config.theme="custom", переключение на обычную
// тему (SetConfig) снимает активный слот.

import (
	"zpui/internal/themes"
)

func (a *App) GetCustomThemes() map[string]interface{} {
	return map[string]interface{}{
		"themes":   a.themes.List(),
		"activeId": a.themes.ActiveID(),
	}
}

// SaveCustomThemes создаёт или обновляет слот {id?, name, theme, activate?}.
func (a *App) SaveCustomThemes(body map[string]interface{}) map[string]interface{} {
	id, _ := body["id"].(string)
	name, _ := body["name"].(string)
	theme, _ := body["theme"].(map[string]interface{})

	id = a.themes.Upsert(id, name, theme)
	if id == "" {
		return errResp("invalid_input")
	}
	if activate, _ := body["activate"].(bool); activate {
		if err := a.themes.Activate(id); err != nil {
			return errResp(err.Error())
		}
		a.cfg.SetTheme("custom")
	}
	return map[string]interface{}{
		"status":   "ok",
		"id":       id,
		"activeId": a.themes.ActiveID(),
		"themes":   a.themes.List(),
	}
}

// ActivateCustomTheme делает слот активным и переводит config.theme в "custom".
func (a *App) ActivateCustomTheme(body map[string]interface{}) map[string]interface{} {
	id, _ := body["id"].(string)
	if err := a.themes.Activate(id); err != nil {
		if err == themes.ErrNotFound {
			return errResp("theme_not_found")
		}
		return errResp(err.Error())
	}
	a.cfg.SetTheme("custom")
	return map[string]interface{}{"status": "ok", "activeId": a.themes.ActiveID()}
}

// DeleteCustomTheme удаляет слот; активный удалить нельзя.
func (a *App) DeleteCustomTheme(body map[string]interface{}) map[string]interface{} {
	id, _ := body["id"].(string)
	if err := a.themes.Delete(id); err != nil {
		switch err {
		case themes.ErrActive:
			return errResp("theme_active")
		case themes.ErrNotFound:
			return errResp("theme_not_found")
		}
		return errResp(err.Error())
	}
	return map[string]interface{}{
		"status":   "ok",
		"themes":   a.themes.List(),
		"activeId": a.themes.ActiveID(),
	}
}

// GetCustomTheme — легаси-точка чтения: активная тема как «единственная»
// (App.jsx применяет её на старте до загрузки галереи).
func (a *App) GetCustomTheme() map[string]interface{} {
	var theme map[string]interface{}
	if act := a.themes.Active(); act != nil {
		theme = act.Theme
	}
	return map[string]interface{}{"theme": theme, "activeId": a.themes.ActiveID()}
}

// SaveCustomTheme — легаси-сохранение в активный слот (создаёт и активирует
// слот, если активного нет).
func (a *App) SaveCustomTheme(body map[string]interface{}) map[string]interface{} {
	theme, _ := body["theme"].(map[string]interface{})
	var id string
	if act := a.themes.Active(); act != nil {
		id = act.ID
	}
	if id == "" {
		id = a.themes.Upsert("", "", theme)
		if err := a.themes.Activate(id); err != nil {
			return errResp(err.Error())
		}
		a.cfg.SetTheme("custom")
	} else {
		a.themes.Upsert(id, "", theme)
	}
	return okResp()
}
