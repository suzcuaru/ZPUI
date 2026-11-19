package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"zpui/internal/pkginst"
	"zpui/internal/zsvc"
)

// Интеграция приложения со службой ZPUI (watchdog): установка/обновление exe
// службы при старте, hello/exit-уведомления. Автозапуск пользователя ведёт
// приложение (Run-ключ); служба занимается только краш-восстановлением.
// Всё best-effort: служба может быть не установлена или не отвечать — это
// штатная ситуация, тумблеры и жизненный цикл имеют локальные фолбэки.

const (
	svcBundleRel   = `components\service\ZPUI-Service.exe`
	svcPingTimeout = 3 * time.Second
	svcCallTimeout = 2 * time.Second
	svcExitTimeout = time.Second
	svcUpdateTTL   = 600
	// svcStartReadyTimeout — сколько ждём готовности pipe после старта службы
	// (repair/upgrade): SCM стартует процесс до секунды, pipe поднимается позже.
	svcStartReadyTimeout = 15 * time.Second
)

// svcStartup — фоновый модуль "service" в Startup: контакт со службой + hello.
func (a *App) svcStartup() {
	a.svcEnsureInstalled()
	a.svcHello()
}

// svcEnsureInstalled гарантирует, что установлена и запущена служба нужной
// версии: нет — установить; есть, но не отвечает — восстановить; версия не та —
// обновить exe. Только при повышенных правах и наличии exe службы в
// компонентах — на dev-машине без компонентов служба не ставится.
func (a *App) svcEnsureInstalled() {
	if !pkginst.IsElevated() {
		a.log.Debug("service", "not elevated — service install/upgrade skipped")
		return
	}
	bundle := filepath.Join(a.exeDir, svcBundleRel)
	if _, err := os.Stat(bundle); err != nil {
		a.log.Debug("service", "service exe missing in components — skipped")
		return
	}
	exe := getExePath()
	if exe == "" {
		return
	}

	installed, err := zsvc.IsInstalled()
	if err != nil {
		a.log.Warn("service", "IsInstalled: "+err.Error())
		return
	}

	if !installed {
		if err := zsvc.Install(bundle); err != nil {
			a.log.Error("service", "install failed: "+err.Error())
			return
		}
		if err := zsvc.Start(); err != nil {
			a.log.Error("service", "start failed: "+err.Error())
			return
		}
		a.log.Info("service", "service installed and started")
	} else if !a.svcAlive(svcPingTimeout) {
		a.svcRepair(bundle)
	}

	version, ok := a.svcPing(svcPingTimeout)
	if !ok || version != zsvc.BundleVersion {
		a.svcUpgrade(bundle)
		version, ok = a.svcPing(svcPingTimeout)
	}
	if !ok {
		a.svcRepair(bundle)
		version, ok = a.svcPing(svcPingTimeout)
	}
	if !ok {
		a.log.Error("service", "service did not answer ping after repair")
		return
	}
	a.log.Info("service", "service alive, version "+version)

	// Канонизация конфига SCM (путь, имя, тип запуска без отложенного старта,
	// recovery-действия): exe-апгрейд конфиг не трогает, а миграции типа запуска
	// у ранее установленных служб иначе не дошли бы. Идемпотентно, best-effort.
	if err := zsvc.Install(bundle); err != nil {
		a.log.Warn("service", "config canonicalization failed: "+err.Error())
	}

	// Службе нужен путь приложения для краш-восстановления (внутренняя
	// задача перезапуска) — автозапуск пользователя она не ведёт.
	if cfg := zsvc.LoadConfig(); cfg.AppPath == "" {
		cfg.AppPath = exe
		_ = zsvc.SaveConfig(cfg)
	}

	a.svcStatusDiagnostics()
}

// svcAwaitReady — ждёт готовности службы после старта: exe запускается SCM
// не мгновенно, pipe-сервер поднимается позже процесса — немедленный пинг
// после Start трактовал «ещё поднимается» как поломку и запускал лишний
// цикл repair/upgrade (эпизод 29.10).
func (a *App) svcAwaitReady(timeout time.Duration) (string, bool) {
	deadline := time.Now().Add(timeout)
	for {
		if v, ok := a.svcPing(svcPingTimeout); ok {
			return v, true
		}
		if time.Now().After(deadline) {
			return "", false
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// svcRepair — служба установлена, но не отвечает: stop → перерегистрация exe
// из компонентов → start. Заодно чинит битый путь бинар в SCM (например,
// если служба когда-то была поставлена с путём к ZPUI.exe вместо ZPUI-Service.exe).
func (a *App) svcRepair(bundle string) {
	a.log.Warn("service", "service not responding — repairing: stop → reinstall → start")
	if err := zsvc.Stop(); err != nil {
		a.log.Warn("service", "repair stop: "+err.Error())
	}
	if err := zsvc.Install(bundle); err != nil {
		a.log.Error("service", "repair install: "+err.Error())
		return
	}
	if err := zsvc.Start(); err != nil {
		a.log.Error("service", "repair start: "+err.Error())
		return
	}
	if v, ok := a.svcAwaitReady(svcStartReadyTimeout); ok {
		a.log.Info("service", "service repaired, version "+v)
	} else {
		a.log.Warn("service", "service restarted but not answering ping yet")
	}
}

// svcUpgrade обновляет exe установленной службы из components: stop → копирование
// поверх зарегистрированного в SCM пути → start. Совпадение путей — копирование
// не нужно (pkginst ставит службу прямо из components).
func (a *App) svcUpgrade(bundle string) {
	dst, err := zsvc.BinaryPath()
	if err != nil || dst == "" {
		a.log.Warn("service", "binary path unavailable: "+err.Error())
		return
	}
	if !strings.EqualFold(filepath.Clean(dst), filepath.Clean(bundle)) {
		if err := zsvc.Stop(); err != nil {
			a.log.Warn("service", "stop before upgrade: "+err.Error())
		}
		if err := copyFile(bundle, dst); err != nil {
			a.log.Error("service", "upgrade copy: "+err.Error())
			return
		}
	}
	if err := zsvc.Start(); err != nil {
		a.log.Error("service", "start after upgrade: "+err.Error())
		return
	}
	a.svcAwaitReady(svcStartReadyTimeout)
	a.log.Info("service", "service exe upgraded from components")
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o755)
}

// svcStatusDiagnostics — стартовая диагностика состояния службы: цикл крашей
// приложения (failed) и краш при прошлом выходе видны в логе/тосте сразу.
func (a *App) svcStatusDiagnostics() {
	ctx, cancel := context.WithTimeout(context.Background(), svcCallTimeout)
	raw, err := a.svc.Status(ctx)
	cancel()
	if err != nil {
		a.log.Debug("service", "status: "+err.Error())
		return
	}
	var st struct {
		Status          string `json:"status"`
		LastExitKind    string `json:"last_exit_kind"`
		LastCrashReason string `json:"last_crash_reason"`
		CrashCount      int    `json:"crash_count"`
	}
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return
	}
	switch {
	case st.Status == zsvc.StatusFailed:
		a.log.Error("service", "служба в состоянии failed после серии крашей приложения: "+st.LastCrashReason)
	case st.LastExitKind == zsvc.ExitCrash:
		a.log.Warn("service", "прошлый выход приложения — краш: "+st.LastCrashReason)
	}
}

// svcHello сообщает службе о запуске приложения (pid, версия, флаги свёрнутого
// запуска и пользовательского автозапуска — последний службе нужен как гард
// краш-восстановления, HKCU пользователя она не видит).
func (a *App) svcHello() {
	pid := os.Getpid()
	ctx, cancel := context.WithTimeout(context.Background(), svcCallTimeout)
	err := a.svc.Hello(ctx, pid, a.version, a.startHidden, autostartEnabled())
	cancel()
	if err != nil {
		a.log.Debug("service", fmt.Sprintf("hello pid=%d: %s", pid, err.Error()))
		return
	}
	a.log.Info("service", fmt.Sprintf("hello: pid=%d version=%s hidden=%v", pid, a.version, a.startHidden))
}

// svcNotifying шлёт службе уведомление о штатном завершении (exiting) или
// перезапуске (restarting): watchdog не посчитает смерть процесса крашем.
func (a *App) svcNotifying(kind string) {
	if a.svc == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), svcExitTimeout)
	defer cancel()
	var err error
	if kind == "restarting" {
		err = a.svc.Restarting(ctx)
	} else {
		err = a.svc.Exiting(ctx)
	}
	if err != nil {
		a.log.Debug("service", kind+": "+err.Error())
	}
}

// svcUpdating ставит watchdog службы на паузу на время обновления приложения.
func (a *App) svcUpdating(ttlSec int) {
	if a.svc == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), svcExitTimeout)
	defer cancel()
	if err := a.svc.Updating(ctx, ttlSec); err != nil {
		a.log.Debug("service", "updating: "+err.Error())
	}
}

// svcAlive — отвечает ли служба на ping за timeout.
func (a *App) svcAlive(timeout time.Duration) bool {
	_, ok := a.svcPing(timeout)
	return ok
}

// GetZsvcPanel — статус службы ZPUI для панели в настройках: установлена,
// отвечает, версия, время старта и счётчик крашей (из IPC status).
func (a *App) GetZsvcPanel() map[string]interface{} {
	installed, err := zsvc.IsInstalled()
	if err != nil || !installed {
		return okRespWithData(map[string]interface{}{"installed": false})
	}
	out := map[string]interface{}{"installed": true, "name": zsvc.ServiceName}
	if a.svc == nil {
		a.svc = zsvc.NewClient()
	}
	ctx, cancel := context.WithTimeout(context.Background(), svcPingTimeout)
	defer cancel()
	raw, err := a.svc.Status(ctx)
	if err != nil {
		out["running"] = false
		return okRespWithData(out)
	}
	out["running"] = true
	var st struct {
		Version         string `json:"version"`
		Status          string `json:"status"`
		StartedAt       int64  `json:"started_at"`
		CrashCount      int    `json:"crash_count"`
		LastExitKind    string `json:"last_exit_kind"`
		LastExitAt      int64  `json:"last_exit_at"`
		LastCrashReason string `json:"last_crash_reason"`
		AppPID          int    `json:"app_pid"`
		AppVersion      string `json:"app_version"`
		Autostart       bool   `json:"autostart"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &st) != nil {
		return okRespWithData(out)
	}
	if st.Version != "" {
		out["version"] = st.Version
	}
	if st.Status != "" {
		out["state"] = st.Status
	}
	if st.StartedAt > 0 {
		out["started_at"] = st.StartedAt
		out["uptime_sec"] = int64(time.Since(time.Unix(st.StartedAt, 0)).Seconds())
	}
	out["crash_count"] = st.CrashCount
	if st.LastExitKind != "" {
		out["last_exit_kind"] = st.LastExitKind
	}
	if st.LastExitAt > 0 {
		out["last_exit_at"] = st.LastExitAt
	}
	if st.LastCrashReason != "" {
		out["last_crash_reason"] = st.LastCrashReason
	}
	if st.AppPID > 0 {
		out["app_pid"] = st.AppPID
	}
	if st.AppVersion != "" {
		out["app_version"] = st.AppVersion
	}
	out["autostart"] = st.Autostart
	return okRespWithData(out)
}

func (a *App) svcPing(timeout time.Duration) (string, bool) {
	if a.svc == nil {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	version, err := a.svc.Ping(ctx)
	if err != nil {
		return "", false
	}
	return version, true
}
