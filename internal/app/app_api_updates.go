package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/database"
	"zpui/internal/updater"
	"zpui/internal/zapret"
)

var (
	downloadMu    sync.Mutex
	installActive atomic.Bool
	// Флаги состояния загрузок/установок: пишутся из фоновых горутин
	// (под downloadMu), читаются биндингами без него — поэтому свой RWMutex.
	updateState struct {
		mu             sync.RWMutex
		downloadActive bool
		setupReady     bool
		setupVersion   string
		setupPath      string
		zapretDlActive bool
		zapretZipReady bool
		zapretZipVer   string
		zapretZipPath  string
		zapretZipSrc   string
		engineActive   bool
		enginePhase    string
	}
)

func updateFlagsSet(fn func()) {
	updateState.mu.Lock()
	fn()
	updateState.mu.Unlock()
}

func updateFlagsGet(fn func()) {
	updateState.mu.RLock()
	fn()
	updateState.mu.RUnlock()
}

func emitDownload(a *App, component, status string, pct int, downloaded, total int64) {
	runtime.EventsEmit(a.ctx, "update:download", map[string]interface{}{
		"component":  component,
		"status":     status,
		"percent":    pct,
		"downloaded": downloaded,
		"total":      total,
	})
}

// emitEngineProgress/emitEngineDone — прогресс установки движка
// (автоустановка на старте, переустановка из EngineFailModal). Отдельные
// события, а не update:download: поток update:* слушает useUpdateFlow и на
// «downloaded» показывал бы кнопку «Установить» обычного обновления запрета.
func emitEngineProgress(a *App, component, status, step string, pct int, downloaded, total int64) {
	runtime.EventsEmit(a.ctx, "engine:progress", map[string]interface{}{
		"component":  component,
		"status":     status,
		"step":       step,
		"percent":    pct,
		"downloaded": downloaded,
		"total":      total,
	})
}

func emitEngineDone(a *App, component string, fields map[string]interface{}) {
	fields["component"] = component
	runtime.EventsEmit(a.ctx, "engine:done", fields)
}

func emitDownloadDone(a *App, component string, fields map[string]interface{}) {
	fields["component"] = component
	runtime.EventsEmit(a.ctx, "update:download-done", fields)
}

// zpuiPlan — проверенный и готовый к скачиванию план одного источника.
type zpuiPlan struct {
	source  string
	version string
	dest    string
	run     func(progressCb func(int, int64, int64)) (*updater.DownloadResult, error)
}

// zpuiSourcePlans — проверяет цепочку источников режима (config.updates_source)
// и возвращает планы по порядку приоритета. Доступный сервер с ответом
// «обновлений нет» и «не новее текущей» в достижимом источнике — окончательное
// no_update. second — последняя ошибка для сообщения, когда цепочка пуста.
func (a *App) zpuiSourcePlans(mode string) (plans []zpuiPlan, lastErr string, noUpdate bool) {
	for _, src := range updater.Chain(mode, updater.ProductZPUI) {
		switch src {
		case "server":
			m, reachable := a.fetchServerZPUIManifest()
			if !reachable {
				lastErr = "сервер управления недоступен"
				continue
			}
			if m == nil || !updater.IsNewer(a.version, m.Version) {
				return plans, "", true
			}
			name := strings.TrimSpace(m.File)
			if name == "" {
				name = updater.ZPUISetupFileName(m.Version)
			}
			dest := filepath.Join(os.TempDir(), name)
			mm := m
			plans = append(plans, zpuiPlan{
				source:  "server",
				version: m.Version,
				dest:    dest,
				run: func(cb func(int, int64, int64)) (*updater.DownloadResult, error) {
					return a.downloadServerSetup(mm, dest, cb)
				},
			})
		case "yandex":
			info, err := updater.CheckZPUISetupUpdate(a.version)
			if err != nil {
				lastErr = err.Error()
				continue
			}
			if !updater.IsNewer(a.version, info.Version) {
				return plans, "", true
			}
			dest := filepath.Join(os.TempDir(), updater.ZPUISetupFileName(info.Version))
			ii := info
			plans = append(plans, zpuiPlan{
				source:  "yandex",
				version: info.Version,
				dest:    dest,
				run: func(cb func(int, int64, int64)) (*updater.DownloadResult, error) {
					return updater.DownloadZPUISetup(ii, dest, cb)
				},
			})
		case "github", "ghproxy":
			info, err := updater.CheckZPUIGithubRelease(a.version)
			if err != nil {
				lastErr = err.Error()
				continue
			}
			if !updater.IsNewer(a.version, info.Version) {
				return plans, "", true
			}
			dest := filepath.Join(os.TempDir(), updater.ZPUISetupFileName(info.Version))
			ii, ghMode := info, src
			plans = append(plans, zpuiPlan{
				source:  src,
				version: info.Version,
				dest:    dest,
				run: func(cb func(int, int64, int64)) (*updater.DownloadResult, error) {
					return a.downloadGithubSetup(ii, dest, ghMode, cb)
				},
			})
		}
	}
	return plans, lastErr, false
}

// downloadServerSetup — установщик с сервера управления, SHA из манифеста
// (пустой SHA — warning, ставится без проверки).
func (a *App) downloadServerSetup(m *serverZPUIManifest, dest string, progressCb func(int, int64, int64)) (*updater.DownloadResult, error) {
	if err := updater.DownloadHTTPWithProgress(m.URL, dest, progressCb); err != nil {
		return nil, err
	}
	sha, err := updater.ComputeSHA256(dest)
	if err != nil {
		os.Remove(dest)
		return nil, fmt.Errorf("не удалось вычислить контрольную сумму обновления: %v", err)
	}
	if m.SHA256 != "" && !strings.EqualFold(sha, m.SHA256) {
		os.Remove(dest)
		return nil, fmt.Errorf("контрольная сумма файла обновления не совпала")
	}
	if m.SHA256 == "" {
		a.log.Warn("updater", "Манифест сервера без SHA-256 — файл не проверен: "+sha)
	}
	res := &updater.DownloadResult{Source: "server", URL: m.URL, SHA256: sha, DestPath: dest}
	if st, statErr := os.Stat(dest); statErr == nil {
		res.Size = st.Size()
	}
	return res, nil
}

// downloadGithubSetup — установщик из релиза ZPUI на GitHub (mode=ghproxy —
// через зеркала gh-proxy.com/ghfast.top с прямым ретраем). SHA сверяется по
// ассету checksums.sha256: файл есть и запись не совпала — отказ; checksums
// недоступны — warning, установка продолжается (как у Яндекса).
func (a *App) downloadGithubSetup(info *updater.ZPUISetupInfo, dest, mode string, progressCb func(int, int64, int64)) (*updater.DownloadResult, error) {
	var lastErr error
	downloaded := false
	for _, u := range updater.GitHubURLs(mode, info.URL) {
		if err := updater.DownloadHTTPWithProgress(u, dest, progressCb); err != nil {
			lastErr = err
			a.log.Warn("updater", "GitHub download failed ("+u[:min(len(u), 80)]+"): "+err.Error())
			continue
		}
		downloaded = true
		break
	}
	if !downloaded {
		if lastErr == nil {
			lastErr = fmt.Errorf("нет доступных URL релиза")
		}
		return nil, lastErr
	}

	sha, err := updater.ComputeSHA256(dest)
	if err != nil {
		os.Remove(dest)
		return nil, fmt.Errorf("не удалось вычислить контрольную сумму обновления: %v", err)
	}
	res := &updater.DownloadResult{Source: mode, URL: info.URL, SHA256: sha, DestPath: dest}
	if st, statErr := os.Stat(dest); statErr == nil {
		res.Size = st.Size()
	}

	if info.ChecksumsURL == "" {
		a.log.Warn("updater", "в релизе нет checksums.sha256 — установщик принят без проверки")
		return res, nil
	}
	var entry *updater.ChecksumEntry
	for _, cu := range updater.GitHubURLs(mode, info.ChecksumsURL) {
		sums, err := updater.FetchChecksumsHTTP(cu)
		if err != nil {
			lastErr = err
			continue
		}
		if e, ok := sums[strings.ToLower(filepath.Base(dest))]; ok {
			entry = &e
			break
		}
	}
	if entry == nil {
		a.log.Warn("updater", "checksums.sha256 релиза недоступен — установщик принят без проверки: "+fmt.Sprint(lastErr))
		return res, nil
	}
	if !strings.EqualFold(sha, entry.SHA256) {
		os.Remove(dest)
		return nil, fmt.Errorf("контрольная сумма установщика не совпала — файл удалён")
	}
	return res, nil
}

// DownloadZPUISetup скачивает установщик ZPUI в %TEMP% с проверкой SHA-256.
// Источник — выбранный в панели «Обновления» (config.updates_source): server
// (ZPUI Live), yandex, github, ghproxy; auto — server → yandex → github.
// Прогресс — события update:download / update:download-done.
func (a *App) DownloadZPUISetup() map[string]interface{} {
	if !downloadMu.TryLock() {
		return errResp("Загрузка уже идёт")
	}
	updateFlagsSet(func() {
		updateState.downloadActive = true
		updateState.setupReady = false
	})

	plans, lastErr, noUpdate := a.zpuiSourcePlans(a.cfg.GetUpdatesSource())
	if noUpdate {
		downloadMu.Unlock()
		updateFlagsSet(func() { updateState.downloadActive = false })
		return okRespWithData(map[string]interface{}{"status": "no_update"})
	}
	if len(plans) == 0 {
		downloadMu.Unlock()
		updateFlagsSet(func() { updateState.downloadActive = false })
		if lastErr == "" {
			lastErr = "источники обновлений недоступны"
		}
		return errResp(lastErr)
	}
	plan := plans[0]
	version, dest := plan.version, plan.dest

	updateFlagsSet(func() {
		updateState.setupVersion = version
		updateState.setupPath = dest
	})

	database.InsertActionLog(&database.ActionLog{
		Category: "updater",
		Action:   "download_start",
		Details:  "Скачивание установщика ZPUI " + version + " (" + plan.source + ")",
	})
	a.log.Info("updater", "Setup download started: "+version+" ("+plan.source+")")

	a.safeGo(func() {
		defer downloadMu.Unlock()
		updateFlagsSet(func() { updateState.downloadActive = true })

		emitDownload(a, "ZPUI", "starting", 0, 0, 0)

		lastLogPct := 0
		progressCb := func(pct int, downloaded, total int64) {
			emitDownload(a, "ZPUI", "downloading", pct, downloaded, total)
			if pct-lastLogPct >= 25 || pct == 100 {
				lastLogPct = pct
				database.InsertActionLog(&database.ActionLog{
					Category: "updater",
					Action:   "download_progress",
					Details:  fmt.Sprintf("Установщик ZPUI: %d%%", pct),
				})
			}
		}

		result, err := plan.run(progressCb)

		if err != nil {
			updateFlagsSet(func() { updateState.downloadActive = false })
			database.InsertActionLog(&database.ActionLog{
				Category: "updater",
				Action:   "download_error",
				Details:  err.Error(),
			})
			a.log.Error("updater", "Setup download failed: "+err.Error())
			emitDownloadDone(a, "ZPUI", map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}

		updateFlagsSet(func() {
			updateState.downloadActive = false
			updateState.setupReady = true
		})

		database.InsertActionLog(&database.ActionLog{
			Category: "updater",
			Action:   "download_complete",
			Details:  "Установщик скачан: " + result.DestPath,
		})
		a.log.Info("updater", "Setup download complete: "+result.DestPath)

		emitDownloadDone(a, "ZPUI", map[string]interface{}{
			"ok":      true,
			"version": version,
			"source":  result.Source,
			"path":    result.DestPath,
		})
	})

	return okRespWithData(map[string]interface{}{
		"status":  "downloading",
		"version": version,
	})
}

// DownloadZapretUpdate скачивает zip запрета с выигравшего источника
// (сервер управления/GitHub) с автоматическим фолбэком, в %TEMP%.
// Прогресс — события update:download / update:download-done с component "zapret".
func (a *App) DownloadZapretUpdate() map[string]interface{} {
	if !downloadMu.TryLock() {
		return errResp("Загрузка уже идёт")
	}
	updateFlagsSet(func() {
		updateState.zapretDlActive = true
		updateState.zapretZipReady = false
	})

	info, err := a.zapret.CheckForUpdates()
	if err != nil {
		downloadMu.Unlock()
		updateFlagsSet(func() { updateState.zapretDlActive = false })
		return errResp(err.Error())
	}
	if !info.UpdateNeeded {
		downloadMu.Unlock()
		updateFlagsSet(func() { updateState.zapretDlActive = false })
		return okRespWithData(map[string]interface{}{"status": "no_update", "version": info.LatestVersion})
	}

	return a.startZapretDownload(info)
}

// startZapretDownload качает zip по готовому UpdateInfo в фоне (downloadMu уже
// удерживается вызывающим). Прогресс — события update:download / update:download-done.
func (a *App) startZapretDownload(info *zapret.UpdateInfo) map[string]interface{} {
	database.InsertActionLog(&database.ActionLog{
		Category: "updater",
		Action:   "download_start",
		Details:  "Скачивание запрета " + info.LatestVersion + " (" + info.Source + ")",
	})
	a.log.Info("updater", fmt.Sprintf("Zapret download started: %s (%s)", info.LatestVersion, info.Source))

	a.safeGo(func() {
		defer downloadMu.Unlock()
		updateFlagsSet(func() { updateState.zapretDlActive = true })

		emitDownload(a, "zapret", "starting", 0, 0, 0)

		lastLogPct := 0
		path, err := a.zapret.DownloadUpdateInfo(info, func(downloaded, total int64) {
			pct := 0
			if total > 0 {
				pct = int(downloaded * 100 / total)
			}
			emitDownload(a, "zapret", "downloading", pct, downloaded, total)
			if pct-lastLogPct >= 25 || pct == 100 {
				lastLogPct = pct
				database.InsertActionLog(&database.ActionLog{
					Category: "updater",
					Action:   "download_progress",
					Details:  fmt.Sprintf("Запрет: %d%%", pct),
				})
			}
		})

		if err != nil {
			updateFlagsSet(func() { updateState.zapretDlActive = false })
			database.InsertActionLog(&database.ActionLog{
				Category: "updater",
				Action:   "download_error",
				Details:  err.Error(),
			})
			a.log.Error("updater", "Zapret download failed: "+err.Error())
			emitDownloadDone(a, "zapret", map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}

		updateFlagsSet(func() {
			updateState.zapretDlActive = false
			updateState.zapretZipReady = true
			updateState.zapretZipPath = path
			updateState.zapretZipVer = info.LatestVersion
			updateState.zapretZipSrc = info.Source
		})

		database.InsertActionLog(&database.ActionLog{
			Category: "updater",
			Action:   "download_complete",
			Details:  "Запрет скачан: " + path,
		})
		a.log.Info("updater", "Zapret download complete: "+path)

		emitDownloadDone(a, "zapret", map[string]interface{}{
			"ok":      true,
			"version": info.LatestVersion,
			"source":  info.Source,
			"path":    path,
		})
	})

	return okRespWithData(map[string]interface{}{
		"status":  "downloading",
		"version": info.LatestVersion,
		"source":  info.Source,
	})
}

func (a *App) zpuiLatestCached() string {
	updFlags.mu.Lock()
	defer updFlags.mu.Unlock()
	return updFlags.zpuiLatest
}

func (a *App) zapretCurrentCached() string {
	updFlags.mu.Lock()
	defer updFlags.mu.Unlock()
	return updFlags.zapretCurrent
}

func (a *App) zapretLatestCached() string {
	updFlags.mu.Lock()
	defer updFlags.mu.Unlock()
	return updFlags.zapretLatest
}

// ApplyZPUISetup запускает скачанный установщик после закрытия приложения:
// detached-помощник ждёт выхода процесса (Wait-Process) и стартует установщик.
// Установщик наследует права администратора ZPUI — UAC не появляется.
func (a *App) ApplyZPUISetup() map[string]interface{} {
	var setupPath, setupVersion string
	updateFlagsGet(func() {
		setupPath = updateState.setupPath
		setupVersion = updateState.setupVersion
	})
	if setupPath == "" {
		return errResp("установщик ещё не скачан")
	}
	if _, err := os.Stat(setupPath); err != nil {
		return errResp("файл установщика не найден: " + setupPath)
	}

	database.InsertActionLog(&database.ActionLog{
		Category: "updater",
		Action:   "apply_start",
		Details:  "Запуск установщика ZPUI " + setupVersion,
	})
	a.log.Info("updater", "Launching installer after quit: "+setupPath)
	// Watchdog службы — на паузу, пока установщик заменяет файлы; пауза
	// продлевается тикером, пока помощник не отчитается о запуске установщика
	// (на медленной машине установка дольше фиксированного TTL).
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

	if err := spawnWaitAndStart(setupPath); err != nil {
		return errResp("не удалось запустить помощник установки: " + err.Error())
	}

	updateFlagsSet(func() { updateState.setupReady = false })
	a.safeGo(func() {
		time.Sleep(500 * time.Millisecond)
		a.Quit()
	})

	return okRespWithData(map[string]interface{}{
		"status":  "installer_launching",
		"version": setupVersion,
	})
}

// ApplyZapretPackage применяет скачанный zip запрета: бэкап состояния в БД,
// распаковка с остановкой/восстановлением службы. Прогресс — события
// update:install / update:install-done.
func (a *App) ApplyZapretPackage() map[string]interface{} {
	var zipPath, zipVer string
	updateFlagsGet(func() {
		zipPath = updateState.zapretZipPath
		zipVer = updateState.zapretZipVer
	})
	if zipPath == "" {
		return errResp("архив запрета ещё не скачан")
	}
	if !installActive.CompareAndSwap(false, true) {
		return errResp("Установка уже идёт")
	}
	// Деструктивная операция: отменяет идущие попытки запуска/восстановления.
	a.opGen.Add(1)

	database.SetZapretJustUpdated(true)
	a.saveBackupToDB()
	database.InsertActionLog(&database.ActionLog{
		Category: "updater",
		Action:   "apply_start",
		Details:  "Установка запрета " + zipVer,
	})
	a.log.Info("updater", "Zapret install started from "+zipPath)

	a.safeGo(func() {
		defer installActive.Store(false)

		emit := func(step string, pct int) {
			runtime.EventsEmit(a.ctx, "update:install", map[string]interface{}{
				"component": "zapret",
				"step":      step,
				"percent":   pct,
			})
		}

		emit("Подготовка", 5)
		err := a.pauseEngineWatchdogDuring(func() error {
			return a.zapret.InstallFromZip(zipPath, emit)
		})

		if err != nil {
			database.InsertActionLog(&database.ActionLog{
				Category: "updater",
				Action:   "apply_error",
				Details:  err.Error(),
			})
			a.log.Error("updater", "Zapret install failed: "+err.Error())
			runtime.EventsEmit(a.ctx, "update:install-done", map[string]interface{}{
				"component": "zapret",
				"ok":        false,
				"error":     err.Error(),
			})
			return
		}

		database.InsertActionLog(&database.ActionLog{
			Category: "updater",
			Action:   "apply_complete",
			Details:  "Запрет обновлён до " + zipVer,
		})
		a.log.Info("updater", "Zapret install complete: "+zipVer)

		updateFlagsSet(func() {
			updateState.zapretZipReady = false
			updateState.zapretZipPath = ""
		})
		a.setZapretUpdateAvailable(zipVer, zipVer, "")

		runtime.EventsEmit(a.ctx, "update:install-done", map[string]interface{}{
			"component": "zapret",
			"ok":        true,
			"version":   zipVer,
		})
	})

	return okRespWithData(map[string]interface{}{"status": "installing"})
}
