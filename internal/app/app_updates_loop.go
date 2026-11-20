package app

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/power"
	"zpui/internal/updater"
)

func (a *App) checkUpdatesOnStartup() {
	time.Sleep(10 * time.Second)
	a.runUpdateCheck()
}

// Интервал фоновых проверок обновлений во время работы. Час — компромисс:
// новая версия подхватывается в среднем за полчаса, а нагрузка на сервер
// мизерная (один лёгкий запрос в час с устройства, лимит ядра — 60/мин).
const (
	updateCheckInterval = time.Hour
	updateCheckJitter   = 10 * time.Minute
)

func (a *App) startUpdateCheckLoop() {
	for {
		delay := power.Stretch(updateCheckInterval + time.Duration(rand.Int63n(int64(updateCheckJitter))) - updateCheckJitter/2)
		select {
		case <-a.stopCh:
			return
		case <-time.After(delay):
		}
		if !a.cfg.AutoUpdateCheck {
			continue
		}
		a.runUpdateCheck()
	}
}

func (a *App) runUpdateCheck() {
	// --- ZPUI --- (приоритет источника: сервер ZPUI Live → Яндекс.Диск →
	// GitHub. Доступный сервер с ответом «обновлений нет» — окончательный
	// ответ; остальные каналы ищем только когда сервер именно недоступен.)
	zpuiLatest := ""
	var srv *serverZPUIManifest
	srvReachable := false
	if m, reachable := a.fetchServerZPUIManifest(); reachable {
		srv, srvReachable = m, true
		if m != nil {
			zpuiLatest = m.Version
			a.log.Info("updater", "server manifest version: "+zpuiLatest)
		}
	} else if yinfo, err := updater.CheckZPUISetupUpdate(a.version); err == nil && yinfo.Version != "" {
		zpuiLatest = yinfo.Version
	} else if ginfo, err := updater.CheckZPUIGithubRelease(a.version); err == nil && ginfo.Version != "" {
		zpuiLatest = ginfo.Version
		a.log.Info("updater", "github release version: "+zpuiLatest)
	}
	if zpuiLatest != "" || srvReachable {
		needed := zpuiLatest != "" && updater.IsNewer(a.version, zpuiLatest)
		if needed {
			// Кэш манифеста — только при обновлении именно с сервера; при
			// фолбэке на Яндекс/GitHub и при «не новее» запись протухшая — чистим.
			if srv != nil {
				a.setZPUIServerManifest(srv.Version, srv.URL, srv.SHA256)
			} else {
				a.setZPUIServerManifest("", "", "")
			}
			a.setZPUIUpdateAvailable(zpuiLatest)
			// Windows-тост об обновлении убран (решение владельца 14.11):
			// обновление видно в панели «Обновления» и в бейдже тайтлбара.
			runtime.EventsEmit(a.getContext(), "update:available", map[string]interface{}{
				"component": "ZPUI",
				"current":   a.version,
				"latest":    zpuiLatest,
			})
			a.log.Info("updater", fmt.Sprintf("ZPUI update available: %s -> %s", a.version, zpuiLatest))
			if srv != nil && srv.Force {
				a.safeGo(func() {
					a.forceInstallServerUpdate(srv.URL, zpuiLatest, srv.SHA256, srv.File)
				})
			}
		} else {
			a.setZPUIUpdateAvailable("")
			a.setZPUIServerManifest("", "", "")
		}
	} else {
		// Вся цепочка недоступна (сервер в том числе) — кэш манифеста сброс.
		a.setZPUIServerManifest("", "", "")
	}

	// --- Zapret ---
	if !a.cfg.GetZapretSkipped() {
		current, latest, source, needed, err := a.zapretEngineCheck()
		if err != nil {
			a.log.Warn("updater", "Zapret update check failed: "+err.Error())
		} else {
			a.setZapretUpdateAvailable(current, latest, source)
			if needed {
				// Тост убран вместе с ZPUI-тостом: достаточно панели обновлений.
				runtime.EventsEmit(a.getContext(), "update:available", map[string]interface{}{
					"component": "zapret",
					"current":   current,
					"latest":    latest,
				})
				a.log.Info("updater", fmt.Sprintf("Zapret update available: %s -> %s (%s)", current, latest, source))
			}
		}

		vr := a.zapret.VerifyFiles()
		if !vr.AllPresent {
			missing := []string{}
			for _, f := range vr.Files {
				if !f.Exists {
					missing = append(missing, f.Path)
				}
			}
			a.log.Warn("zapret", fmt.Sprintf("Missing files: %v — auto-recovering", missing))
			runtime.EventsEmit(a.getContext(), "zapret:files-missing", map[string]interface{}{
				"missing": missing,
			})

			a.log.Info("zapret", "Auto-downloading missing zapret files...")
			runtime.EventsEmit(a.getContext(), "zapret:recovering", map[string]interface{}{
				"missing_count": len(missing),
			})
			if err := a.zapret.DownloadAndInstall(nil); err != nil {
				a.log.Error("zapret", "Auto-recovery failed: "+err.Error())
				runtime.EventsEmit(a.getContext(), "zapret:recovery_failed", map[string]interface{}{
					"error": err.Error(),
				})
			} else {
				a.log.Info("zapret", "Auto-recovery completed successfully")
				runtime.EventsEmit(a.getContext(), "zapret:recovery_complete", nil)
			}
		}
	}

	// Канал обновления запрета единый: CheckForUpdates (сервер → Яндекс →
	// GitHub Flowseal), скачивание/установка — DownloadAndInstall менеджера.

	a.syncVersionsToDB()
}
