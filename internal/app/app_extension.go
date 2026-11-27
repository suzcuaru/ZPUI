package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/extension"
	"zpui/internal/updater"
)

// Браузерное расширение ZPUI (спека Документы/Задачи/2026-11-24-browser-extension.md):
// установка пакета из components\extension\ в Chromium-браузеры через реестр
// и обновление пакета с сервера управления (GET /updates/extension).
//
// Обновление — как у ZPUI и zapret: скачивание и установка идут в фоне,
// прогресс — события update:download / update:install / update:install-done
// с component "extension" (модалка UpdateProgressModal). Установщик
// ZPUI-Extension-Setup (cmd\extsetup) докачивается рядом с пакетом для
// ручной установки; standalone-пакет в %LOCALAPPDATA%\ZPUI\extension — его
// работа, приложение берёт более новый из двух пакетов (extension.Local).

// extSetupExe — имя установщика расширения (совпадает с именем артефакта build.bat).
const extSetupExe = "ZPUI-Extension-Setup.exe"

// serverExtensionManifest — ответ сервера: тот же контракт, что /api/v1/updates/zpui.
type serverExtensionManifest struct {
	Version string                `json:"version"`
	URL     string                `json:"url"`
	SHA256  string                `json:"sha256"`
	Notes   string                `json:"notes"`
	Setup   *serverExtensionSetup `json:"setup"`
}

// serverExtensionSetup — standalone-установщик в манифесте (route /updates/extension/setup).
type serverExtensionSetup struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

var (
	// extUpdateMu — одно обновление/установка плагина одновременно.
	extUpdateMu sync.Mutex
	extUpdating atomic.Bool
)

func init() { extension.SetVersionCmp(updater.CompareVersions) }

func idsToStrings(v []interface{}) []string {
	out := []string{}
	for _, x := range v {
		if s, ok := x.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// uniqueStrings — без пустых и повторов, порядок сохранён.
func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// fetchExtensionManifest — свежий выпуск на сервере; nil без ошибки, когда
// сервер ответил «выпуска нет». Сервер недоступен — ошибка (не молчим).
func (a *App) fetchExtensionManifest() (*serverExtensionManifest, error) {
	if ServerURL == "" {
		return nil, errors.New("сервер управления не задан")
	}
	resp, err := serverClient(10 * time.Second).Get(ServerURL + "/updates/extension")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("сервер: http %d", resp.StatusCode)
	}
	var m serverExtensionManifest
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&m); err != nil {
		return nil, errors.New("сервер прислал не манифест")
	}
	if m.Version == "" {
		return nil, nil
	}
	return &m, nil
}

// foundBrowsers — id найденных на машине браузеров.
func foundBrowsers() []string {
	ids := []string{}
	for _, b := range extension.List("") {
		if b.Found {
			ids = append(ids, b.ID)
		}
	}
	return ids
}

// extBrowserNames — имена браузеров по id (для итога модалки).
func extBrowserNames(ids []string) []string {
	names := make([]string, 0, len(ids))
	for _, b := range extension.List("") {
		for _, id := range ids {
			if b.ID == id {
				names = append(names, b.Name)
			}
		}
	}
	return names
}

// GetExtensionInfo — пакет, браузеры и доступное обновление (GET /api/extension/info).
// version — пакет, с которым работает приложение (компонент установки либо
// standalone-каталог установщика); package_source — где он лежит: "app" | "local".
func (a *App) GetExtensionInfo() map[string]interface{} {
	_, local, source := extension.Local(a.exeDir)
	m, err := a.fetchExtensionManifest()
	latest, notes, srvErr, via := "", "", "", "none"
	if err != nil {
		srvErr = err.Error()
		a.log.Warn("extension", "server manifest: "+srvErr)
	} else if m != nil {
		latest, notes = m.Version, m.Notes
		if m.Setup != nil {
			via = "setup"
		} else {
			via = "crx"
		}
	}
	return okRespWithData(map[string]interface{}{
		"id":        extension.ID,
		"version":   local,
		"available": local != "",
		// пакет на сервере новее локального
		"update_ready":   latest != "" && (local == "" || updater.CompareVersions(latest, local) > 0),
		"update_via":     via,
		"package_source": source,
		"browsers":       extension.List(a.exeDir),
		"latest":         latest,
		"notes":          notes,
		"server_error":   srvErr,
		"updating":       extUpdating.Load(),
	})
}

// InstallExtension — поставить пакет в выбранные браузеры (POST /api/extension/install {ids});
// пустой список — во все найденные. Браузер подхватит его при следующем старте.
func (a *App) InstallExtension(ids []interface{}) map[string]interface{} {
	list := idsToStrings(ids)
	if len(list) == 0 {
		list = foundBrowsers()
	}
	if len(list) == 0 {
		return errResp("браузеры не найдены")
	}
	path, ver, _ := extension.Local(a.exeDir)
	if path == "" {
		return errResp("пакет плагина не найден")
	}
	if fails := extension.InstallPath(path, ver, list); len(fails) > 0 {
		msgs := make([]string, 0, len(fails))
		for id, msg := range fails {
			msgs = append(msgs, id+": "+msg)
		}
		a.log.Warn("extension", "install: "+strings.Join(msgs, "; "))
		return errResp("не удалось зарегистрировать: " + strings.Join(msgs, "; "))
	}
	a.log.Info("extension", "installed to: "+strings.Join(list, ", "))
	a.ensureExtLink()
	return okRespWithData(map[string]interface{}{"browsers": extBrowserNames(list)})
}

// autoInstallExtension — стартовая саморегистрация плагина: пакет идёт в
// дистрибутиве (components\extension), поэтому после установки/обновления
// программы регистрируем его во всех найденных Chromium-браузерах сами.
// Где запись уже есть — не трогаем; неудача не мешает остальным.
func (a *App) autoInstallExtension() {
	path, ver, _ := extension.Local(a.exeDir)
	if path == "" {
		return
	}
	browsers := extension.List(a.exeDir)
	targets := []string{}
	for _, b := range browsers {
		if b.Found && !b.HasExtension {
			targets = append(targets, b.ID)
		}
	}
	if len(targets) == 0 {
		return
	}
	if fails := extension.InstallPath(path, ver, targets); len(fails) > 0 {
		msgs := make([]string, 0, len(fails))
		for id, msg := range fails {
			msgs = append(msgs, id+": "+msg)
		}
		a.log.Warn("extension", "auto install: "+strings.Join(msgs, "; "))
		return
	}
	a.log.Info("extension", "auto-installed to: "+strings.Join(targets, ", "))
	a.ensureExtLink()
}

// ensureExtLink — плагин управляется через локальный MCP-сервер приложения:
// при установке включаем его, чтобы связь сработала сама, без ключа руками.
func (a *App) ensureExtLink() {
	if a.cfg.GetMcpConfig().Enabled {
		return
	}
	if err := a.cfg.SetMcpEnabled(true); err != nil {
		a.log.Warn("mcp", "enable for extension: "+err.Error())
		return
	}
	if err := a.mcpStart(); err == nil {
		a.log.Info("mcp", "enabled: browser extension installed")
	}
}

// UninstallExtension — снять регистрацию в выбранных браузерах
// (POST /api/extension/uninstall {ids}); браузер уберёт его при старте.
func (a *App) UninstallExtension(ids []interface{}) map[string]interface{} {
	list := idsToStrings(ids)
	if len(list) == 0 {
		return errResp("выберите хотя бы один браузер")
	}
	if fails := extension.Uninstall(list); len(fails) > 0 {
		msgs := make([]string, 0, len(fails))
		for id, msg := range fails {
			msgs = append(msgs, id+": "+msg)
		}
		return errResp("не удалось снять регистрацию: " + strings.Join(msgs, "; "))
	}
	a.log.Info("extension", "uninstalled from: "+strings.Join(list, ", "))
	return okResp()
}

// OpenExtensionPage — открыть в браузере страницу расширений на карточке ZPUI,
// где пользователь включает расширение после установки (POST /api/extension/open {id}).
func (a *App) OpenExtensionPage(id string) map[string]interface{} {
	if err := extension.OpenExtensionsPage(id); err != nil {
		return errResp(err.Error())
	}
	return okResp()
}

// UpdateExtension — скачать выпуск с сервера и поставить плагин в браузеры
// (POST /api/extension/update {ids}): ids — выбор пользователя; сверх них
// обновляются браузеры, где плагин уже стоит; пусто и нигде не стоит — все
// найденные. Отвечает сразу, ход — событиями update:download / update:install /
// update:install-done с component "extension".
func (a *App) UpdateExtension(ids []interface{}) map[string]interface{} {
	if !extUpdateMu.TryLock() {
		return errResp("плагин уже устанавливается или обновляется")
	}
	extUpdating.Store(true)
	pick := uniqueStrings(idsToStrings(ids))
	a.safeGo(func() {
		defer extUpdateMu.Unlock()
		defer extUpdating.Store(false)
		res := a.extensionUpdateFlow(pick)
		if res["ok"] == true {
			runtime.EventsEmit(a.getContext(), "update:install-done", res)
			return
		}
		a.log.Error("extension", "update failed: "+fmt.Sprint(res["error"]))
		runtime.EventsEmit(a.getContext(), "update:install-done", res)
	})
	return okResp()
}

func extFail(msg string) map[string]interface{} {
	return map[string]interface{}{"component": "extension", "ok": false, "error": msg}
}

func emitExtInstall(a *App, pct int, step string) {
	runtime.EventsEmit(a.getContext(), "update:install", map[string]interface{}{
		"component": "extension",
		"step":      step,
		"percent":   pct,
	})
}

// extensionUpdateFlow — скачивание пакета с прогрессом, проверка хеша,
// запись пакета и регистрация в браузерах.
func (a *App) extensionUpdateFlow(pick []string) map[string]interface{} {
	const comp = "extension"
	m, err := a.fetchExtensionManifest()
	if err != nil {
		return extFail("сервер управления недоступен: " + err.Error())
	}
	if m == nil {
		return extFail("на сервере нет выпуска плагина")
	}
	// Пакет исполняется в браузере пользователя: без хеша не ставим.
	if len(m.SHA256) != 64 {
		return extFail("сервер не отдал хеш пакета — установка отменена")
	}

	// Скачивание: пакет + установщик (если сервер его отдал) — одним прогрессом,
	// чтобы полза шла по реальному объёму.
	setupSize := m.Setup.Size
	if m.Setup != nil && setupSize <= 0 {
		setupSize = 1 << 20 // размер не заявлен — ориентир, прогресс не остановится
	}
	total := int64(64 << 10)
	if m.Setup != nil {
		total += setupSize
	}
	var done int64
	lastPct := -1
	report := func(delta, unknown int64) {
		if unknown > 0 {
			done = unknown
		} else {
			done += delta
		}
		pct := int(done * 100 / total)
		if pct > 99 {
			pct = 99
		}
		if pct != lastPct {
			lastPct = pct
			emitDownload(a, comp, "downloading", pct, done, total)
		}
	}

	tmp, err := os.CreateTemp("", "zpui-extension-*.crx")
	if err != nil {
		return extFail(err.Error())
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	emitDownload(a, comp, "starting", 0, 0, 0)
	a.log.Info("extension", fmt.Sprintf("update: downloading package v%s from server", m.Version))
	if err := downloadServerFileProgress(ServerURL+"/updates/extension/download", tmpPath, 64<<20, func(n int64) {
		report(0, n)
	}); err != nil {
		return extFail("не удалось скачать пакет: " + err.Error())
	}
	sha, err := updater.ComputeSHA256(tmpPath)
	if err != nil || !strings.EqualFold(sha, m.SHA256) {
		a.log.Error("extension", "package sha256 mismatch")
		return extFail("хеш пакета не совпал — установка отменена")
	}
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return extFail(err.Error())
	}
	if got, verr := extension.ReadVersionBytes(data); verr != nil || got != m.Version {
		return extFail("пакет на сервере повреждён или не той версии")
	}

	// Установщик докачивается best-effort: для работы плагина в браузерах
	// он не нужен (регистрирует само приложение).
	if m.Setup != nil && len(m.Setup.SHA256) == 64 {
		if err := a.extDownloadSetup(m, func(n int64) { report(n, -1) }); err != nil {
			a.log.Warn("extension", "setup installer not refreshed: "+err.Error())
		}
	}
	done = total
	emitDownload(a, comp, "downloading", 100, done, total)
	emitDownloadDone(a, comp, map[string]interface{}{"ok": true, "version": m.Version})

	// Установка: пакет → компонент установки (каталог может быть закрыт на
	// запись — тогда standalone-каталог), затем регистрация в браузерах.
	emitExtInstall(a, 15, "Запись пакета плагина")
	if err := extension.WritePackage(a.exeDir, data); err != nil {
		a.log.Warn("extension", "components dir not writable, using standalone: "+err.Error())
		if err := extension.WriteLocalPackage(data); err != nil {
			return extFail("не удалось сохранить пакет: " + err.Error())
		}
	}
	path, ver, _ := extension.Local(a.exeDir)
	if path == "" {
		return extFail("не удалось сохранить пакет")
	}

	// Целевые браузеры: выбор пользователя + где уже стоит; никого — все найденные.
	targets := append([]string{}, pick...)
	for _, b := range extension.List(a.exeDir) {
		if b.HasExtension {
			targets = append(targets, b.ID)
		}
	}
	if len(targets) == 0 {
		targets = foundBrowsers()
	}
	if len(targets) == 0 {
		return extFail("браузеры не найдены")
	}
	targets = uniqueStrings(targets)

	emitExtInstall(a, 60, "Регистрация в браузерах")
	if fails := extension.InstallPath(path, ver, targets); len(fails) > 0 {
		msgs := make([]string, 0, len(fails))
		for id, msg := range fails {
			msgs = append(msgs, id+": "+msg)
		}
		return extFail("не удалось зарегистрировать: " + strings.Join(msgs, "; "))
	}
	emitExtInstall(a, 100, "")
	a.log.Info("extension", "updated to "+ver+" in "+strings.Join(targets, ", "))
	a.ensureExtLink()
	return map[string]interface{}{
		"component": "extension",
		"ok":        true,
		"version":   ver,
		"browsers":  extBrowserNames(targets),
	}
}

// extDownloadSetup — скачать ZPUI-Extension-Setup.exe с сервера, проверить
// размер и SHA-256 (exe исполняется на машине пользователя) и положить рядом
// с пакетом. Прогресс — по факту скачанного.
func (a *App) extDownloadSetup(m *serverExtensionManifest, onBytes func(int64)) error {
	dir := filepath.Join(a.exeDir, "components", "extension")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(dir, extSetupExe)
	part := dst + ".part"
	if err := downloadServerFileProgress(ServerURL+"/updates/extension/setup", part, 256<<20, onBytes); err != nil {
		os.Remove(part)
		return err
	}
	st, err := os.Stat(part)
	if err != nil || (m.Setup.Size > 0 && st.Size() != m.Setup.Size) {
		os.Remove(part)
		return errors.New("размер установщика не совпал")
	}
	sha, err := updater.ComputeSHA256(part)
	if err != nil || !strings.EqualFold(sha, m.Setup.SHA256) {
		os.Remove(part)
		return errors.New("хеш установщика не совпал")
	}
	if err := os.Rename(part, dst); err != nil {
		os.Remove(part)
		return err
	}
	a.log.Info("extension", fmt.Sprintf("setup: installer refreshed (%s)", m.Setup.File))
	return nil
}

// downloadServerFile — потоковая загрузка с сервера управления в файл.
func downloadServerFile(url, dst string, limit int64) error {
	return downloadServerFileProgress(url, dst, limit, nil)
}

// downloadServerFileProgress — то же с обратной связью по числу прочитанных байт.
func downloadServerFileProgress(url, dst string, limit int64, onBytes func(int64)) error {
	resp, err := serverClient(5 * time.Minute).Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("сервер: http %d", resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	var n int64
	if onBytes != nil {
		buf := make([]byte, 64<<10)
		for {
			k, rerr := resp.Body.Read(buf)
			if k > 0 && n < limit {
				w, werr := f.Write(buf[:min(int64(k), limit-n)])
				n += int64(w)
				if werr != nil {
					err = werr
					break
				}
				onBytes(n)
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				err = rerr
				break
			}
		}
	} else {
		n, err = io.Copy(f, io.LimitReader(resp.Body, limit))
	}
	cerr := f.Close()
	if err != nil {
		return err
	}
	return cerr
}
