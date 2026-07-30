package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/database"
	"zpui/internal/executil"
	"zpui/internal/updater"
)

var (
	downloadMu      sync.Mutex
	downloadActive  bool
	downloadedReady bool
)

// DownloadZPUIUpdate скачивает архив обновления ZPUI в .zpui-update.zip с
// потоковой передачей прогресса через события update:download и
// финальным событием update:download-done. Запуск фоновый (fire-and-forget).
func (a *App) DownloadZPUIUpdate() map[string]interface{} {
	if !downloadMu.TryLock() {
		return errResp("Загрузка уже идёт")
	}
	downloadActive = true
	downloadedReady = false

	a.safeGo(func() {
		defer downloadMu.Unlock()
		downloadActive = true

		zipPath := filepath.Join(a.exeDir, ".zpui-update.zip")

		database.InsertActionLog(&database.ActionLog{
			Category: "updater",
			Action:   "download_start",
			Details:  "Скачивание ZPUI обновления",
		})
		a.log.Info("updater", "Download started")

		runtime.EventsEmit(a.ctx, "update:download", map[string]interface{}{
			"status":  "starting",
			"percent": 0,
		})

		lastLogPct := 0
		result, err := updater.DownloadZPUIUpdate(zipPath, func(pct int, downloaded, total int64) {
			runtime.EventsEmit(a.ctx, "update:download", map[string]interface{}{
				"status":     "downloading",
				"percent":    pct,
				"downloaded": downloaded,
				"total":      total,
			})

			if pct-lastLogPct >= 10 || pct == 100 {
				lastLogPct = pct
				downloadedMB := float64(downloaded) / 1024 / 1024
				totalMB := float64(total) / 1024 / 1024
				database.InsertActionLog(&database.ActionLog{
					Category: "updater",
					Action:   "download_progress",
					Details:  fmt.Sprintf("Загружено: %d%% (%.1f МБ / %.1f МБ)", pct, downloadedMB, totalMB),
				})
			}
		})

		if err != nil {
			downloadActive = false
			database.InsertActionLog(&database.ActionLog{
				Category: "updater",
				Action:   "download_error",
				Details:  err.Error(),
			})
			a.log.Error("updater", "Download failed: "+err.Error())
			runtime.EventsEmit(a.ctx, "update:download-done", map[string]interface{}{
				"ok":    false,
				"error": err.Error(),
			})
			return
		}

		downloadActive = false
		downloadedReady = true

		database.InsertActionLog(&database.ActionLog{
			Category: "updater",
			Action:   "download_complete",
			Details:  fmt.Sprintf("Скачивание завершено, источник: %s", result.Source),
		})
		a.log.Info("updater", "Download complete: "+result.Source)

		runtime.EventsEmit(a.ctx, "update:download-done", map[string]interface{}{
			"ok":      true,
			"version": a.version,
			"source":  result.Source,
			"path":    zipPath,
		})
	})
	return okRespWithData(map[string]interface{}{
		"status": "downloading",
	})
}

// IsUpdateDownloaded проверяет, готов ли скачанный архив обновления.
func (a *App) IsUpdateDownloaded() map[string]interface{} {
	zipPath := filepath.Join(a.exeDir, ".zpui-update.zip")
	info, err := os.Stat(zipPath)
	ready := err == nil && info.Size() > 0 && downloadedReady
	return map[string]interface{}{
		"downloaded":  ready,
		"downloading": downloadActive,
		"path":        zipPath,
	}
}

// ApplyDownloadedZPUIUpdate запускает selfupdate.exe в режиме применения
// (mode 2, --from-app) и закрывает приложение. selfupdate использует уже
// скачанный .zpui-update.zip, если он есть.
func (a *App) ApplyDownloadedZPUIUpdate() map[string]interface{} {
	selfUpdate := a.findModulePath("selfupdate")
	if selfUpdate == "" {
		return errResp("selfupdate.exe не найден")
	}

	if err := executil.GuiCmd(selfUpdate, "--from-app").Start(); err != nil {
		return errResp("не удалось запустить обновление: " + err.Error())
	}

	downloadedReady = false
	a.safeGo(func() {
		time.Sleep(2 * time.Second)
		a.Quit()
	})

	return okRespWithData(map[string]interface{}{
		"status":  "updater_launched",
		"updater": "selfupdate.exe",
	})
}
