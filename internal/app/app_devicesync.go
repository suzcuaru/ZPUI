package app

// Синхронизация устройств аккаунта (решение владельца 10.11.2026): устройство
// собирает снапшот (настройки/темы/данные), жмёт gzip и кладёт на сервер
// (sync.snapshot.put, core/devicesync.go); другое устройство аккаунта после
// входа скачивает снапшот (модалка выбора источника) и применяет.
// Автопуш — раз в сутки (startDeviceSyncLoop). Лимит слотов считает сервер —
// devices_limit подписки.
//
// Состав снапшота (компактный JSON → gzip → base64):
//   config: тема, язык, автосмена, кастомные DNS;
//   themes: слоты кастомных тем + активная (themes.json);
//   data:   skip-resources.txt + пользовательские списки zapret (*-user.txt).
// Фоны/звуки тем и тяжёлые файлы не синхронизируются (лимит слота 150 КБ
// gzip на сервере); сервер блоб не парсит.

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"zpui/internal/config"
	"zpui/internal/database"
	"zpui/internal/notify"
)

const deviceSyncVersion = 1

type deviceSyncSnapshot struct {
	Version int             `json:"v"`
	Config  deviceSyncCfg   `json:"config"`
	Themes  deviceSyncTheme `json:"themes"`
	Data    deviceSyncData  `json:"data"`
}

type deviceSyncCfg struct {
	Theme        string             `json:"theme"`
	Language     string             `json:"language"`
	AutoSwitch   deviceSyncAutoSw   `json:"auto_switch"`
	DnsCustom    []config.CustomDns `json:"dns_custom"`
}

type deviceSyncAutoSw struct {
	Enabled   bool `json:"enabled"`
	Threshold int  `json:"threshold_pct"`
	Interval  int  `json:"interval_min"`
}

type deviceSyncTheme struct {
	ActiveID string            `json:"active_id"`
	Slots    []themeSlotCompact `json:"slots"`
}

type themeSlotCompact struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Theme map[string]any `json:"theme"`
}

type deviceSyncData struct {
	SkipResources string            `json:"skip_resources"`
	Lists         map[string]string `json:"lists"`
}

// deviceSyncBuild — собрать снапшот текущего устройства: gzip-байт и размер.
func (a *App) deviceSyncBuild() ([]byte, error) {
	snap := &deviceSyncSnapshot{Version: deviceSyncVersion}
	var theme, lang string
	var asw deviceSyncAutoSw
	var dnsCustom []config.CustomDns
	a.cfg.RView(func() {
		theme = a.cfg.Theme
		asw = deviceSyncAutoSw{
			Enabled:   a.cfg.AutoSwitchEnabled,
			Threshold: a.cfg.AutoSwitchThresholdPct,
			Interval:  a.cfg.AutoSwitchIntervalMin,
		}
		dnsCustom = append([]config.CustomDns{}, a.cfg.XboxDns.Custom...)
	})
	lang = a.cfg.GetLanguage()
	snap.Config = deviceSyncCfg{
		Theme: theme, Language: lang, AutoSwitch: asw, DnsCustom: dnsCustom,
	}
	snap.Themes = deviceSyncTheme{ActiveID: a.themes.ActiveID()}
	for _, slot := range a.themes.List() {
		snap.Themes.Slots = append(snap.Themes.Slots, themeSlotCompact{
			ID: slot.ID, Name: slot.Name, Theme: slot.Theme,
		})
	}

	if data, err := os.ReadFile(a.cfg.GetSkipResourcesFilePath()); err == nil {
		snap.Data.SkipResources = string(data)
	}
	snap.Data.Lists = map[string]string{}
	entries, err := os.ReadDir(a.cfg.ListsDir())
	if err == nil {
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, "-user.txt") {
				continue
			}
			if data, err := os.ReadFile(filepath.Join(a.cfg.ListsDir(), name)); err == nil {
				snap.Data.Lists[name] = string(data)
			}
		}
	}

	raw, err := json.Marshal(snap)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(raw); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// deviceSyncApply — применить снапшот другого устройства. Возвращает
// краткое описание (для тоста).
func (a *App) deviceSyncApply(snap *deviceSyncSnapshot) string {
	parts := []string{}
	if snap.Config.Theme != "" {
		_ = a.cfg.SetTheme(snap.Config.Theme)
		parts = append(parts, "тема")
	}
	if snap.Config.Language != "" {
		_ = a.cfg.Update(func(c *config.Config) {
			c.Language = snap.Config.Language
		})
		parts = append(parts, "язык")
	}
	asw := snap.Config.AutoSwitch
	if asw.Threshold > 0 || asw.Interval > 0 || asw.Enabled {
		_ = a.cfg.Update(func(c *config.Config) {
			c.AutoSwitchEnabled = asw.Enabled
			if asw.Threshold > 0 {
				c.AutoSwitchThresholdPct = asw.Threshold
			}
			if asw.Interval > 0 {
				c.AutoSwitchIntervalMin = asw.Interval
			}
		})
		parts = append(parts, "настройки")
	}
	if len(snap.Config.DnsCustom) > 0 {
		_ = a.cfg.Update(func(c *config.Config) {
			c.XboxDns.Custom = snap.Config.DnsCustom
		})
		a.dnsReconfigure()
		parts = append(parts, "DNS")
	}
	// Темы: восстановить слоты (Upsert по id) + активную.
	for _, slot := range snap.Themes.Slots {
		a.themes.Upsert(slot.ID, slot.Name, slot.Theme)
	}
	if snap.Themes.ActiveID != "" {
		_ = a.themes.Activate(snap.Themes.ActiveID)
		a.emitSafe("theme:sync", nil)
	}
	if snap.Data.SkipResources != "" {
		if err := os.WriteFile(a.cfg.GetSkipResourcesFilePath(), []byte(snap.Data.SkipResources), 0644); err == nil {
			parts = append(parts, "скипы")
		}
	}
	for name, content := range snap.Data.Lists {
		if !strings.HasSuffix(name, "-user.txt") {
			continue
		}
		path := filepath.Join(a.cfg.ListsDir(), name)
		if err := os.WriteFile(path, []byte(content), 0644); err == nil {
			parts = append(parts, name)
		}
	}
	a.emitStrategiesChanged()
	return strings.Join(parts, ", ")
}

// deviceSyncParse — распаковать payload сервера (base64 gzip JSON).
func deviceSyncParse(payloadB64 string) (*deviceSyncSnapshot, error) {
	raw, err := base64.StdEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, err
	}
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	var snap deviceSyncSnapshot
	if err := json.NewDecoder(gz).Decode(&snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

// DeviceSyncList — GET /api/account/sync/devices: снапшоты устройств аккаунта
// (для модалки выбора источника после входа).
func (a *App) DeviceSyncList() map[string]interface{} {
	tok := a.cfg.GetServerConfig().AccountToken
	if tok == "" {
		return errResp("not_authorized")
	}
	data, e := a.syncCallMap("sync.snapshots", tok, nil)
	if e != nil {
		return syncErrResp(e)
	}
	return okRespWithData(data)
}

// DeviceSyncPush — POST /api/account/sync/push: собрать снапшот и положить
// на сервер (авто-пуш раз в сутки делает то же без вызова фронта).
func (a *App) DeviceSyncPush() map[string]interface{} {
	n, e := a.deviceSyncPushOnce()
	if e != nil {
		return errResp(e.Error())
	}
	return okRespWithData(map[string]interface{}{"bytes": n})
}

// deviceSyncPushOnce — сборка+загрузка снапшота; возвращает размер payload'а.
func (a *App) deviceSyncPushOnce() (int, error) {
	tok := a.cfg.GetServerConfig().AccountToken
	if tok == "" {
		return 0, fmt.Errorf("not_authorized")
	}
	gzBytes, err := a.deviceSyncBuild()
	if err != nil {
		return 0, err
	}
	payload := base64.StdEncoding.EncodeToString(gzBytes)
	name := ""
	if host, herr := os.Hostname(); herr == nil {
		name = host
	}
	_, e := a.syncCallMap("sync.snapshot.put", tok, map[string]any{
		"payload": payload, "name": name,
	})
	if e != nil {
		return 0, fmt.Errorf("%s", e.Code)
	}
	_ = database.SetFlag("device_sync_push_at", time.Now().Format(time.RFC3339))
	return len(gzBytes), nil
}

// DeviceSyncRestore — POST /api/account/sync/restore {"device_id"}: скачать
// снапшот устройства и применить (настройки/темы/данные).
func (a *App) DeviceSyncRestore(deviceID string) map[string]interface{} {
	tok := a.cfg.GetServerConfig().AccountToken
	if tok == "" {
		return errResp("not_authorized")
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return errResp("device_id required")
	}
	data, e := a.syncCallMap("sync.snapshot.get", tok, map[string]any{"device_id": deviceID})
	if e != nil {
		return syncErrResp(e)
	}
	payload, _ := data["payload"].(string)
	if payload == "" {
		return errResp("empty_snapshot")
	}
	snap, err := deviceSyncParse(payload)
	if err != nil {
		return errResp("bad_snapshot: " + err.Error())
	}
	applied := a.deviceSyncApply(snap)
	return okRespWithData(map[string]interface{}{"applied": applied})
}

// startDeviceSyncLoop — авто-пуш раз в сутки (решение владельца 10.11.2026):
// авторизованное устройство обновляет свой снапшот; первый пуш — через 2
// минуты после старта (дать сети/синку подняться).
func (a *App) startDeviceSyncLoop() {
	push := func() {
		if a.cfg.GetServerConfig().AccountToken == "" {
			return
		}
		if _, err := a.deviceSyncPushOnce(); err != nil {
			a.log.Warn("devicesync", "push: "+err.Error())
			// device_limit (слоты исчерпаны) — юзеру один раз тост-подсказка
			// (решение владельца 10.11.2026); прочие ошибки — только в лог.
			if strings.Contains(err.Error(), "device_limit") && flagStr("device_sync_limit_toast") == "" {
				_ = database.SetFlag("device_sync_limit_toast", time.Now().Format(time.RFC3339))
				lang := a.cfg.GetLanguage()
				_ = notify.Show("ZPUI · "+tr(lang, "sync_title"), tr(lang, "sync_limit"))
			}
		}
	}
	if last := parseTimeFlag(flagStr("device_sync_push_at")); !last.IsZero() &&
		time.Since(last) < 24*time.Hour {
		// свежий снапшот уже на сервере — ставим будильник на остаток суток
		time.AfterFunc(24*time.Hour-time.Since(last)+time.Minute, push)
	} else {
		time.AfterFunc(2*time.Minute, push)
	}
	t := time.NewTicker(24 * time.Hour)
	for range t.C {
		push()
	}
}

// flagStr — строка из app_flags ("" при ошибке).
func flagStr(key string) string {
	v, _ := database.GetFlag(key)
	return v
}
