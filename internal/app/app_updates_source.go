package app

import (
	"strings"

	"zpui/internal/config"
	"zpui/internal/database"
)

// Источник обновлений (панель «Обновления», один на приложение и запреты).

// GetUpdatesSource — выбранный источник обновлений.
func (a *App) GetUpdatesSource() map[string]interface{} {
	return okRespWithData(map[string]interface{}{"source": a.cfg.GetUpdatesSource()})
}

// SetUpdatesSource — источник: auto|server|yandex|github|ghproxy.
func (a *App) SetUpdatesSource(source string) map[string]interface{} {
	s := strings.ToLower(strings.TrimSpace(source))
	if !config.ValidUpdatesSource(s) {
		return errResp("неизвестный источник: " + source)
	}
	if err := a.cfg.SetUpdatesSource(s); err != nil {
		return errResp(err.Error())
	}
	a.log.Info("updater", "updates source set: "+s)
	database.InsertActionLog(&database.ActionLog{
		Category: "updater",
		Action:   "source_change",
		Details:  "Источник обновлений: " + s,
	})
	return okRespWithData(map[string]interface{}{"source": s})
}
