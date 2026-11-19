package app

import (
	"os"
	"time"

	"zpui/internal/database"
)

// Установка/обновление движка запрета (zapret, Flowseal): приложение скачивает
// дистрибутив по updates_source (авто: сервер → Яндекс → GitHub) и ставит его.
// Прогресс — события engine:progress/engine:done с component "zapret".

func (a *App) GetZapretEngine() map[string]interface{} {
	version := a.zapret.GetVersion()
	vr := a.zapret.VerifyFiles()
	updateState.mu.RLock()
	busy := updateState.engineActive
	phase := updateState.enginePhase
	updateState.mu.RUnlock()
	return okRespWithData(map[string]interface{}{
		"installed":  vr.AllPresent || (version != "" && version != "unknown"),
		"engine":     "zapret",
		"version":    version,
		"installing": busy,
		"phase":      phase,
	})
}

// InstallZapretEngine — установка/обновление запрета: скачивание + распаковка
// + служба. Прогресс — события engine:progress / engine:done с
// component "zapret".
func (a *App) InstallZapretEngine() map[string]interface{} {
	if !downloadMu.TryLock() {
		return errResp("Загрузка уже идёт")
	}

	a.safeGo(func() {
		defer downloadMu.Unlock()
		updateState.mu.Lock()
		updateState.engineActive = true
		updateState.mu.Unlock()

		err := a.installEngineFlow()

		updateState.mu.Lock()
		updateState.engineActive = false
		updateState.mu.Unlock()
		if err != nil {
			a.log.Error("zapret", "engine install failed: "+err.Error())
			emitEngineDone(a, "zapret", map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		emitEngineDone(a, "zapret", map[string]interface{}{"ok": true, "engine": "zapret"})
	})

	return okRespWithData(map[string]interface{}{"status": "downloading", "engine": "zapret"})
}

// pauseEngineWatchdogDuring — ставит watchdog службы ZPUI на паузу, пока идёт
// замена каталога движка: служба иначе поднимет winws (SCM-recovery или
// своё слежение) под руку установщику, и тот упадёт на залоченных файлах.
func (a *App) pauseEngineWatchdogDuring(fn func() error) error {
	a.svcUpdating(svcUpdateTTL)
	pauseDone := make(chan struct{})
	defer close(pauseDone)
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-pauseDone:
				return
			case <-t.C:
				a.svcUpdating(svcUpdateTTL)
			}
		}
	}()
	return fn()
}

// installEngineFlow — скачивание запрета (events downloading) и установка
// (events installing) с паузой watchdog службы на время замены каталога.
func (a *App) installEngineFlow() error {
	emitEngineProgress(a, "zapret", "starting", "", 0, 0, 0)
	info, err := a.zapret.CheckForUpdatesVia(a.cfg.GetUpdatesSource())
	if err != nil {
		return err
	}
	a.log.Info("zapret", "zapret "+info.LatestVersion+" (source "+info.Source+")")

	dest, err := a.zapret.DownloadUpdateInfo(info, func(downloaded, total int64) {
		pct := 0
		if total > 0 {
			pct = int(downloaded * 100 / total)
		}
		emitEngineProgress(a, "zapret", "downloading", "", pct, downloaded, total)
	})
	if err != nil {
		return err
	}
	defer os.Remove(dest)

	err = a.pauseEngineWatchdogDuring(func() error {
		return a.zapret.InstallFromZip(dest, func(step string, pct int) {
			emitEngineProgress(a, "zapret", "installing", step, pct, 0, 0)
		})
	})
	if err != nil {
		return err
	}

	a.finishEngineInstall(info.LatestVersion)
	return nil
}

// finishEngineInstall — общие шаги после установки движка: пользовательские
// списки, версия, служба (если была установлена).
func (a *App) finishEngineInstall(version string) {
	_ = a.zapret.EnsureUserLists()
	a.zapret.RefreshVersion()

	a.log.Info("zapret", "движок запрета установлен: "+version)
	database.InsertActionLog(&database.ActionLog{
		Category: "zapret",
		Action:   "engine_installed",
		Details:  "Движок запрета установлен: " + version,
	})

	if a.HasSystemZapretService() {
		strategy := a.cfg.GetCurrentStrategy()
		if strategy == "" {
			strategy = a.zapret.DefaultStrategyName()
		}
		if res, err := a.zapret.InstallServiceLogged(strategy); err == nil && res != nil && len(res.Errors) > 0 {
			a.log.Warn("zapret", "служба: "+res.Errors[0])
		}
	}
}
