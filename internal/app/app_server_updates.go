package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/notify"
	"zpui/internal/updater"
)

// serverClient — HTTP-клиент для загрузок с сервера управления.
func serverClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

// ---- Обновление ZPUI с сервера (v1: GET /api/v1/updates/zpui) ----
// Инвариант обновляемости: эта проверка остаётся навсегда, независимо от
// канала данных (ZPUI Sync). Фолбэк при недоступности сервера — Яндекс.Диск
// (internal/updater).

// serverZPUIManifest — манифест обновления ZPUI с сервера управления:
// файл раздаётся через /api/v1/updates/zpui/download?id= (стрим).
type serverZPUIManifest struct {
	Version string
	URL     string // абсолютный URL файла
	SHA256  string
	File    string // база имени файла установщика
	Notes   string
	Force   bool
	IsTest  bool
}

// fetchServerZPUIManifest — опрос манифеста обновлений на сервере.
// reachable=false → сервер недоступен (сеть, таймаут, 5xx, мусорный ответ):
// вызывающий уходит в фолбэк на Яндекс.Диск. reachable=true при m==nil —
// сервер ответил и обновлений для нас нет: это окончательный ответ,
// Яндекс не ищем.
func (a *App) fetchServerZPUIManifest() (m *serverZPUIManifest, reachable bool) {
	if ServerURL == "" || !a.authorizationOn() {
		return nil, false
	}
	client := serverClient(20 * time.Second)
	manifestURL := ServerURL + "/api/v1/updates/zpui?device_id=" + url.QueryEscape(a.cfg.GetSyncInstanceID())
	resp, err := client.Get(manifestURL)
	if err != nil {
		a.log.Warn("updater", "server manifest unreachable: "+err.Error())
		return nil, false
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		a.log.Warn("updater", "server manifest read: "+err.Error())
		return nil, false
	}
	if resp.StatusCode >= 300 {
		a.log.Warn("updater", fmt.Sprintf("server manifest: http %d", resp.StatusCode))
		return nil, false
	}
	var body struct {
		Status  string `json:"status"`
		Error   string `json:"error"`
		Version string `json:"version"`
		URL     string `json:"url"`
		SHA256  string `json:"sha256"`
		Notes   string `json:"notes"`
		Force   bool   `json:"force"`
		IsTest  bool   `json:"is_test"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		a.log.Warn("updater", "server manifest: bad json")
		return nil, false
	}
	// «Обновлений нет» (not_found / пустая версия) — окончательный ответ:
	// сервер доступен, Яндекс не ищем.
	if body.Error != "" || body.Version == "" || body.URL == "" {
		return nil, true
	}
	abs := body.URL
	if !strings.HasPrefix(abs, "http://") && !strings.HasPrefix(abs, "https://") {
		abs = ServerURL + abs
	}
	return &serverZPUIManifest{
		Version: body.Version,
		URL:     abs,
		SHA256:  body.SHA256,
		File:    "ZPUI-Setup-" + body.Version,
		Notes:   body.Notes,
		Force:   body.Force,
		IsTest:  body.IsTest,
	}, true
}

// forceInstallServerUpdate — скачивает установщик с сервера, проверяет SHA-256
// и запускает установку тем же Wait-Process помощником, что и ручное обновление.
func (a *App) forceInstallServerUpdate(url, version, sha256, fileBase string) {
	if url == "" {
		a.log.Warn("updater", "force update: empty url")
		return
	}
	name := strings.TrimSpace(fileBase)
	if name == "" {
		name = updater.ZPUISetupFileName(version)
	}
	tmp := filepath.Join(os.TempDir(), name+".part")
	resp, err := serverClient(15 * time.Minute).Get(url)
	if err != nil {
		a.log.Warn("updater", "force update download failed: "+err.Error())
		return
	}
	defer resp.Body.Close()
	f, err := os.Create(tmp)
	if err != nil {
		a.log.Warn("updater", "force update temp file: "+err.Error())
		return
	}
	_, err = io.Copy(f, io.LimitReader(resp.Body, 1<<30))
	f.Close()
	if err != nil {
		os.Remove(tmp)
		a.log.Warn("updater", "force update write failed: "+err.Error())
		return
	}
	sha, shaErr := updater.ComputeSHA256(tmp)
	switch {
	case shaErr != nil:
		os.Remove(tmp)
		a.log.Error("updater", "force update sha256 compute failed: "+shaErr.Error())
		return
	case sha256 != "" && !equalFold(sha, sha256):
		os.Remove(tmp)
		a.log.Error("updater", "force update sha256 mismatch")
		return
	case sha256 == "":
		a.log.Warn("updater", "force update без SHA-256 — файл не проверен: "+sha)
	}
	dest := filepath.Join(os.TempDir(), name)
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		a.log.Warn("updater", "force update rename failed: "+err.Error())
		return
	}

	notify.Show("ZPUI", fmt.Sprintf("Устанавливается обязательное обновление %s", version))
	wruntime.EventsEmit(a.ctx, "server:force-update", map[string]interface{}{"version": version})
	a.log.Info("updater", "force update installing: "+version)

	updateFlagsSet(func() {
		updateState.setupPath = dest
		updateState.setupVersion = version
	})
	a.ApplyZPUISetup()
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// osVersion — версия Windows из реестра (ProductName + DisplayVersion).
// Нюанс Microsoft: у Windows 11 ProductName в реестре до сих пор
// «Windows 10 …» — реальную версию различаем по номеру сборки (≥ 22000).
func (a *App) osVersion() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return "Windows"
	}
	defer k.Close()
	pn, _, _ := k.GetStringValue("ProductName")
	dv, _, _ := k.GetStringValue("DisplayVersion")
	cb, _, _ := k.GetStringValue("CurrentBuildNumber")
	build := 0
	for _, ch := range cb {
		if ch < '0' || ch > '9' {
			break
		}
		build = build*10 + int(ch-'0')
	}
	if pn == "" {
		return "Windows"
	}
	if build >= 22000 {
		pn = strings.Replace(pn, "Windows 10", "Windows 11", 1)
	}
	if dv != "" {
		return pn + " (" + dv + ")"
	}
	return pn
}
