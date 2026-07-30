package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/config"
	"zpui/internal/executil"
	"zpui/internal/logger"
	"zpui/internal/updater"
	"zpui/internal/zapret"
)

type App struct {
	ctx     context.Context
	cfg     *config.Config
	log     *logger.Logger
	zapret  *zapret.Manager
	version string
	exeDir  string
	mode    string
}

func NewApp(cfg *config.Config, log *logger.Logger, zm *zapret.Manager, version, exeDir, mode string) *App {
	return &App{
		cfg: cfg, log: log, zapret: zm,
		version: version, exeDir: exeDir, mode: mode,
	}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

type srcVer struct {
	Version string `json:"version"`
	Avail   bool   `json:"avail"`
}

type targetState struct {
	Current      string `json:"current"`
	UpdateNeeded bool   `json:"update_needed"`
	BestVersion  string `json:"best_version,omitempty"`
	GitHub       srcVer `json:"github"`
	Cloud        srcVer `json:"cloud"`
}

type moduleState struct {
	Name        string `json:"name"`
	Current     string `json:"current"`
	Latest      string `json:"latest"`
	NeedsUpdate bool   `json:"needs_update"`
}

type updaterState struct {
	Mode      string        `json:"mode"`
	Version   string        `json:"version"`
	Theme     string        `json:"theme"`
	CheckedAt string        `json:"checked_at"`
	ZPUI      targetState   `json:"zpui"`
	Zapret    targetState   `json:"zapret"`
	Modules   []moduleState `json:"modules"`
}

// GetState возвращает полное состояние для UI апдейтера: режим, версии
// ZPUI и Zapret с двумя источниками (GitHub/Облако) и список модулей.
func (a *App) GetState() updaterState {
	zs := a.zapret.FetchVersionSources()
	zap := targetState{
		Current: zs.Current,
		Cloud:   srcVer{Version: zs.CloudVersion, Avail: zs.CloudAvail},
	}
	if info, err := a.zapret.CheckForUpdates(); err == nil {
		zap.UpdateNeeded = info.UpdateNeeded
	} else {
		zap.UpdateNeeded = a.targetNeedsUpdate(zs.Current, zs.CloudVersion)
	}

	zpuiCur := a.readInstalledZPUIVersion()

	yaRV, _ := updater.FetchRemoteVersionsYandex(updater.YandexPublicURL)

	zpui := targetState{Current: zpuiCur}
	if yaRV != nil {
		zpui.Cloud = srcVer{Version: yaRV.ZPUI, Avail: yaRV.ZPUI != ""}
	}
	zpui.UpdateNeeded = a.targetNeedsUpdate(zpuiCur, zpui.Cloud.Version)

	return updaterState{
		Mode:      a.mode,
		Version:   a.version,
		Theme:     a.cfg.GetTheme(),
		CheckedAt: time.Now().Format("02.01.2006 15:04:05"),
		ZPUI:      zpui,
		Zapret:    zap,
		Modules:   a.buildModules(yaRV),
	}
}

func (a *App) targetNeedsUpdate(current, cloud string) bool {
	if current == "" || current == "unknown" {
		return false
	}
	if cloud != "" && updater.IsNewer(current, cloud) {
		return true
	}
	return false
}

func (a *App) buildModules(ya *updater.RemoteVersions) []moduleState {
	manifest := readVersionsManifest(a.exeDir)
	local := map[string]string{
		"selfupdate": manifest.SelfUpdate,
		"report":     manifest.Report,
		"security":   manifest.Security,
	}
	yaMap := remoteMap(ya)

	order := []string{"selfupdate", "report", "security"}
	var out []moduleState
	for _, key := range order {
		cur := local[key]
		latest := yaMap[key]
		out = append(out, moduleState{
			Name: key, Current: cur, Latest: latest,
			NeedsUpdate: cur != "" && latest != "" && updater.IsNewer(cur, latest),
		})
	}
	return out
}

func remoteMap(rv *updater.RemoteVersions) map[string]string {
	m := map[string]string{}
	if rv != nil {
		m["selfupdate"] = rv.SelfUpdate
		m["report"] = rv.Report
		m["security"] = rv.Security
	}
	return m
}

// readInstalledZPUIVersion читает поле zpui из versions.json в папке приложения.
func (a *App) readInstalledZPUIVersion() string {
	return readVersionsManifest(a.exeDir).ZPUI
}

type versionsManifest struct {
	ZPUI       string `json:"zpui"`
	SelfUpdate string `json:"selfupdate"`
	Report     string `json:"report"`
	Security   string `json:"security"`
}

func readVersionsManifest(exeDir string) versionsManifest {
	m := versionsManifest{}
	data, err := os.ReadFile(filepath.Join(exeDir, "versions.json"))
	if err != nil {
		return m
	}
	json.Unmarshal(data, &m)
	return m
}

// GetReleaseInfo возвращает информацию о релизе для встроенной модалки.
func (a *App) GetReleaseInfo(target string) map[string]interface{} {
	if target == "zapret" {
		info, err := updater.GetZapretReleaseInfo()
		if err != nil {
			return map[string]interface{}{"error": err.Error()}
		}
		return releaseMap(info)
	}
	info, err := updater.GetReleaseInfo()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return releaseMap(info)
}

func releaseMap(i *updater.ReleaseInfo) map[string]interface{} {
	return map[string]interface{}{
		"tag_name":     i.TagName,
		"name":         i.Name,
		"body":         i.Body,
		"published_at": i.PublishedAt,
		"html_url":     i.HTMLURL,
	}
}

// DownloadZPUI скачивает архив ZPUI с прогрессом (события dl:progress / dl:done).
// В событиях передаётся версия и источник, чтобы UI показывал «скачивается vX…».
func (a *App) DownloadZPUI() {
	zipPath := filepath.Join(a.exeDir, ".zpui-update.zip")
	go func() {
		info := updater.FetchZPUIUpdateInfo()
		a.log.Info("selfupdate", fmt.Sprintf("ZPUI download start: v%s from %s", info.Version, info.Source))
		emit := func(status string, pct int, downloaded, total int64) {
			runtime.EventsEmit(a.ctx, "dl:progress", map[string]interface{}{
				"target": "zpui", "status": status, "percent": pct,
				"downloaded": downloaded, "total": total,
				"version": info.Version, "source": info.Source,
			})
		}
		emit("starting", 0, 0, 0)
		_, err := updater.DownloadZPUIUpdate(zipPath, func(pct int, downloaded, total int64) {
			emit("downloading", pct, downloaded, total)
		})
		if err != nil {
			a.log.Error("selfupdate", "ZPUI download failed: "+err.Error())
			runtime.EventsEmit(a.ctx, "dl:done", map[string]interface{}{
				"target": "zpui", "ok": false, "error": err.Error(), "code": classifyError(err.Error()),
			})
			return
		}
		a.log.Info("selfupdate", fmt.Sprintf("ZPUI downloaded v%s from %s", info.Version, info.Source))
		runtime.EventsEmit(a.ctx, "dl:done", map[string]interface{}{
			"target": "zpui", "ok": true, "path": zipPath,
			"version": info.Version, "source": info.Source,
		})
	}()
}

// DownloadZapret скачивает архив zapret с прогрессом.
func (a *App) DownloadZapret() {
	go func() {
		// Сначала проверяем — нужна ли версия и какой источник выбран,
		// чтобы показать пользователю ЧТО именно качается.
		info, err := a.zapret.CheckForUpdates()
		if err != nil {
			a.log.Error("selfupdate", "Zapret check failed: "+err.Error())
			runtime.EventsEmit(a.ctx, "dl:done", map[string]interface{}{
				"target": "zapret", "ok": false, "error": err.Error(), "code": classifyError(err.Error()),
			})
			return
		}
		if !info.UpdateNeeded {
			a.log.Info("selfupdate", "Zapret уже актуален, обновление не требуется")
			runtime.EventsEmit(a.ctx, "dl:done", map[string]interface{}{
				"target": "zapret", "ok": true, "no_update": true,
			})
			return
		}
		a.log.Info("selfupdate", fmt.Sprintf("Zapret download start: v%s from %s", info.LatestVersion, info.Source))
		emit := func(status string, pct int, downloaded, total int64) {
			runtime.EventsEmit(a.ctx, "dl:progress", map[string]interface{}{
				"target": "zapret", "status": status, "percent": pct,
				"downloaded": downloaded, "total": total,
				"version": info.LatestVersion, "source": info.Source,
			})
		}
		emit("starting", 0, 0, 0)
		path, err := a.zapret.DownloadUpdateInfo(info, func(downloaded, total int64) {
			pct := 0
			if total > 0 {
				pct = int(downloaded * 100 / total)
			}
			emit("downloading", pct, downloaded, total)
		})
		if err != nil {
			a.log.Error("selfupdate", "Zapret download failed: "+err.Error())
			runtime.EventsEmit(a.ctx, "dl:done", map[string]interface{}{
				"target": "zapret", "ok": false, "error": err.Error(), "code": classifyError(err.Error()),
			})
			return
		}
		a.log.Info("selfupdate", fmt.Sprintf("Zapret downloaded v%s from %s", info.LatestVersion, info.Source))
		runtime.EventsEmit(a.ctx, "dl:done", map[string]interface{}{
			"target": "zapret", "ok": true, "path": path,
			"version": info.LatestVersion, "source": info.Source,
		})
	}()
}

// ApplyZPUI применяет обновление ZPUI (события apply:progress / apply:done).
func (a *App) ApplyZPUI() {
	go func() {
		runtime.EventsEmit(a.ctx, "apply:progress", map[string]interface{}{
			"target": "zpui", "status": "starting", "percent": 0,
		})
		err := a.runZPUIApply()
		if err != nil {
			a.log.Error("selfupdate", "ZPUI apply failed: "+err.Error())
			runtime.EventsEmit(a.ctx, "apply:done", map[string]interface{}{
				"target": "zpui", "ok": false, "error": err.Error(),
			})
			return
		}
		runtime.EventsEmit(a.ctx, "apply:done", map[string]interface{}{
			"target": "zpui", "ok": true,
		})
	}()
}

// ApplyZapret применяет обновление zapret из скачанного архива.
func (a *App) ApplyZapret() {
	go func() {
		tempZip := filepath.Join(os.TempDir(), "zapret-update.zip")
		err := a.zapret.InstallFromZip(tempZip, func(step string, pct int) {
			runtime.EventsEmit(a.ctx, "apply:progress", map[string]interface{}{
				"target": "zapret", "status": step, "percent": pct,
			})
		})
		if err != nil {
			a.log.Error("selfupdate", "Zapret apply failed: "+err.Error())
			runtime.EventsEmit(a.ctx, "apply:done", map[string]interface{}{
				"target": "zapret", "ok": false, "error": err.Error(), "code": classifyError(err.Error()),
			})
			return
		}
		runtime.EventsEmit(a.ctx, "apply:done", map[string]interface{}{
			"target": "zapret", "ok": true,
		})
	}()
}

func classifyError(msg string) string {
	m := strings.ToLower(msg)
	switch {
	case strings.Contains(m, "скач") || strings.Contains(m, "download") || strings.Contains(m, "http") || strings.Contains(m, "нет доступных url"):
		return "UPD-E001"
	case strings.Contains(m, "распак") || strings.Contains(m, "extract") || strings.Contains(m, "заняты") || strings.Contains(m, "zip"):
		return "UPD-E002"
	case strings.Contains(m, "checksum") || strings.Contains(m, "sha") || strings.Contains(m, "целост") || strings.Contains(m, "вериф"):
		return "UPD-E003"
	case strings.Contains(m, "останов") || strings.Contains(m, "stop") || strings.Contains(m, "kill") || strings.Contains(m, "процесс"):
		return "UPD-E004"
	case strings.Contains(m, "service") || strings.Contains(m, "служб") || strings.Contains(m, "sc "):
		return "UPD-E005"
	case strings.Contains(m, "restore") || strings.Contains(m, "восстан"):
		return "UPD-E006"
	case strings.Contains(m, "apply") || strings.Contains(m, "установ") || strings.Contains(m, "install"):
		return "UPD-E007"
	default:
		return "UPD-E999"
	}
}

// QuitApp закрывает окно апдейтера.
func (a *App) QuitApp() {
	a.log.Info("selfupdate", "quit requested")
	go func() {
		executil.HiddenCmd("taskkill", "/IM", "selfupdate.exe", "/F").Run()
	}()
}

// CloseZPUI завершает основное приложение ZPUI.
// Вызывается из selfupdate, когда решение о закрытии принято
// (например, перед применением обновления ZPUI).
func (a *App) CloseZPUI() {
	a.log.Info("selfupdate", "closing ZPUI...")
	go func() {
		executil.HiddenCmd("taskkill", "/IM", "ZPUI.exe", "/F").Run()
	}()
}

// emit helper for apply progress text.
func (a *App) applyProgress(status string, pct int) {
	runtime.EventsEmit(a.ctx, "apply:progress", map[string]interface{}{
		"target": "zpui", "status": status, "percent": pct,
	})
}

func (a *App) runZPUIApply() error {
	return applyZPUI(a)
}
