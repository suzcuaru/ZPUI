package app

import (
	"zpui/internal/updater"
)

// ============================================================
// ZAPRET CONTROL
// ============================================================

// ZapretStart запускает движок асинхронно: если за 5 секунд он не ожил —
// автоматически запускается механизм восстановления (окно с этапами).
func (a *App) ZapretStart() map[string]interface{} {
	if a.zapret.IsAutoTestRunning() {
		return errResp("strategy test in progress")
	}
	if a.isRecoveryRunning() {
		return map[string]interface{}{"status": "recovering", "reason": "recovery already in progress"}
	}

	a.safeGo(func() {
		if err := a.zapretStartAndVerify(); err != nil {
			a.log.Warn("zapret", "Manual start failed, recovering: "+err.Error())
			a.startZapretRecovery("manual_start_failed: " + err.Error())
			return
		}
		a.zapretExpected.Store(true)
	})
	return map[string]interface{}{"status": "starting"}
}

func (a *App) ZapretStop() map[string]interface{} {
	if a.zapret.IsAutoTestRunning() {
		return errResp("strategy test in progress")
	}
	// Деструктивная операция: отменяет идущие попытки запуска/восстановления.
	a.opGen.Add(1)
	// Явная остановка пользователем — авто-восстановление не поднимает движок.
	a.zapretExpected.Store(false)
	a.zapret.Stop()
	return okResp()
}

// ZapretRestart перезапускает движок с текущей стратегией (async: статус "restarting").
func (a *App) ZapretRestart() map[string]interface{} {
	if a.zapret.IsAutoTestRunning() {
		return errResp("strategy test in progress")
	}
	a.safeGo(func() {
		a.opGen.Add(1)
		if err := a.zapret.Restart(); err != nil {
			a.log.Warn("zapret", "restart failed: "+err.Error())
			a.startZapretRecovery("restart_failed: " + err.Error())
		}
	})
	return okRespWithData(map[string]interface{}{"status": "restarting"})
}

// DefaultStrategy — стратегия по умолчанию (первый general* ALT).
func (a *App) DefaultStrategy() map[string]interface{} {
	return map[string]interface{}{"strategy": a.zapret.DefaultStrategyName()}
}

// ============================================================
// UPDATES
// ============================================================

// zapretEngineCheck — проверка обновлений запрета (Flowseal-цепочка
// CheckForUpdates: server → yandex → github по updates_source).
// Единая точка для биндинга и фонового цикла — иначе разные каналы
// перезаписывают флаги версий друг друга.
func (a *App) zapretEngineCheck() (current, latest, source string, needed bool, err error) {
	info, e := a.zapret.CheckForUpdates()
	if e != nil {
		return "", "", "", false, e
	}
	return info.CurrentVersion, info.LatestVersion, info.Source, info.UpdateNeeded || updater.IsNewer(info.CurrentVersion, info.LatestVersion), nil
}

func (a *App) CheckForUpdates() interface{} {
	current, latest, source, needed, err := a.zapretEngineCheck()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{
		"current_version": current,
		"latest_version":  latest,
		"update_needed":   needed,
		"source":          source,
	}
}
