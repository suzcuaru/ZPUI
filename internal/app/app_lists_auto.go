package app

import (
	"time"

	"zpui/internal/notify"
	"zpui/internal/power"
)

// Авто-обновление ipset-списка: при запуске приложения и далее раз в 24 часа.
// Ручные кнопки скачивания из панели фильтров убраны — список свежеет сам.
// В экономном режиме плановое скачивание откладывается (полчаса туда-обратно),
// чтобы не будить сеть; стартовое выполняется всегда.

const listsAutoInterval = 24 * time.Hour

func (a *App) startListsAutoUpdate() {
	select {
	case <-a.stopCh:
		return
	case <-time.After(15 * time.Second):
	}
	a.autoUpdateLists("startup")
	for {
		select {
		case <-a.stopCh:
			return
		case <-time.After(listsAutoInterval):
		}
		for power.Active() {
			select {
			case <-a.stopCh:
				return
			case <-time.After(30 * time.Minute):
			}
		}
		a.autoUpdateLists("scheduled")
	}
}

func (a *App) autoUpdateLists(trigger string) {
	listFile := a.ipsetFilePath()
	// Список не зависит от режима ipset (режим живёт в конфиге и подменяет
	// файл в аргументах winws), поэтому обновление безопасно в любом режиме.
	result := downloadAndSave(ipsetServiceURL, listFile)
	// Стартовое обновление выполняется тихо (пользователь только что открыл
	// приложение — свежесть видна в панели); плановое и все сбои — с тостом.
	withToast := trigger != "startup"
	lang := a.cfg.GetLanguage()
	if result["error"] == nil {
		a.log.Info("strategy", "Ipset list auto-updated ("+trigger+")")
		a.emitStrategiesChanged()
		if withToast && a.cfg.GetNotificationsEnabled() {
			_ = notify.Show("ZPUI · IPSet", tr(lang, "ipset_updated"))
		}
	} else {
		if err, ok := result["error"].(string); ok {
			a.log.Warn("strategy", "Ipset auto-update failed ("+trigger+"): "+err)
		}
		if withToast && a.cfg.GetNotificationsEnabled() {
			_ = notify.Show("ZPUI · IPSet", tr(lang, "ipset_update_failed"))
		}
	}
}
