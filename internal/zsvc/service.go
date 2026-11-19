package zsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"

	"zpui/internal/executil"
)

// Service — реализация svc.Handler: реестр-инициализация, pipe-сервер и
// watchdog; подходит и для SCM (svc.Run), и для -console режима (StartWorkers).
type Service struct {
	// Version — версия exe службы (ldflags), пишется в реестр на старте.
	Version string

	log       *SvcLog
	wd        *Watchdog
	cancel    context.CancelFunc
	startedAt time.Time
}

// New создаёт обработчик службы с заданной версией exe.
func New(version string) *Service {
	return &Service{Version: version}
}

// Execute — точка входа svc.Handler.
func (s *Service) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptSessionChange
	changes <- svc.Status{State: svc.StartPending, WaitHint: 5000}
	s.StartWorkers(context.Background())
	changes <- svc.Status{State: svc.Running, Accepts: accepted}
	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending, WaitHint: 5000}
			s.StopWorkers()
			return false, 0
		case svc.SessionChange:
			if c.EventType == windows.WTS_SESSION_LOGOFF {
				s.log.Info("session change: logoff")
				s.wd.SetLogoff()
			}
		}
	}
	return false, 0
}

// StartWorkers инициализирует состояние и запускает pipe-сервер и watchdog.
// Публично: используется и в -console режиме.
func (s *Service) StartWorkers(ctx context.Context) {
	s.log = OpenLog()
	s.startedAt = time.Now()
	s.log.Info("service starting v" + s.Version)
	cfg := LoadConfig()
	cfg.ServiceVersion = s.Version
	if cfg.Status == StatusWatching || cfg.Status == StatusPaused {
		cfg.Status = StatusIdle
	}
	if err := SaveConfig(cfg); err != nil {
		s.log.Error("start config save: " + err.Error())
	}
	s.wd = NewWatchdog(s.log)
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	ps := &pipeServer{handler: s.handleCommand, log: s.log}
	go s.wd.Run(ctx)
	go ps.serve(ctx)
	go s.dnsSupervisor(ctx)
}

// StopWorkers останавливает pipe-сервер и watchdog.
func (s *Service) StopWorkers() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.log != nil {
		s.log.Info("service stopped")
	}
}

// handleCommand — диспетчер IPC-команд (одна строка → одна строка JSON).
func (s *Service) handleCommand(cmd ipcCommand) string {
	switch cmd.Name {
	case "ping":
		return respOKVersion(s.Version)
	case "status":
		return s.statusJSON()
	case "app-hello":
		pid, version, hidden, autostart, err := parseHelloArg(cmd.Arg)
		if err != nil {
			return respErr("bad_request")
		}
		s.wd.SetHello(pid, version, hidden, autostart)
		return respOK()
	case "app-exiting":
		s.wd.SetExiting()
		return respOK()
	case "app-restarting":
		s.wd.SetRestarting()
		return respOK()
	case "app-updating":
		ttl, err := parseUpdatingArg(cmd.Arg)
		if err != nil {
			return respErr("bad_request")
		}
		s.wd.SetUpdating(ttl)
		return respOK()
	case "restart-app":
		hidden, err := parseRestartArg(cmd.Arg)
		if err != nil {
			return respErr("bad_request")
		}
		if cmd.Arg == "" {
			hidden = LoadConfig().Hidden
		}
		go s.restartApp(hidden)
		return respOK()
	case "close-app":
		go s.closeApp()
		return respOK()
	default:
		return respErr("unknown_command")
	}
}

// restartApp — ждёт завершения процесса до 10с, затем taskkill /F и запуск
// через внутреннюю задачу аварийного перезапуска (выполняется в отдельной
// горутине). Автозапуск пользователя служба не трогает — только краш-восстановление.
func (s *Service) restartApp(hidden bool) {
	s.wd.SetRestarting()
	pid := s.wd.HelloPid()
	if pid != 0 {
		deadline := time.Now().Add(restartWaitKill)
		for appProcessAlive(pid) && time.Now().Before(deadline) {
			time.Sleep(250 * time.Millisecond)
		}
	}
	if pid != 0 && appProcessAlive(pid) {
		s.wd.killImage(appImageName)
	}
	cfg := LoadConfig()
	cfg.Hidden = hidden
	if err := SaveConfig(cfg); err != nil {
		s.log.Error("restart-app config save: " + err.Error())
	}
	if cfg.AppPath == "" {
		s.log.Error("restart-app: app path unknown")
		return
	}
	if !RestoreTaskExists() {
		if err := CreateRestoreTask(cfg.AppPath, hidden); err != nil {
			s.log.Error("restart-app create restore task: " + err.Error())
			return
		}
	}
	if err := RunRestoreTask(); err != nil {
		s.log.Error("restart-app: " + err.Error())
		return
	}
	s.log.Info(fmt.Sprintf("restart-app done (hidden=%v)", hidden))
}

// closeApp — штатное закрытие приложения (graceful, без /F).
func (s *Service) closeApp() {
	pid := s.wd.HelloPid()
	s.wd.SetExiting()
	var err error
	if pid != 0 {
		err = executil.HiddenCmd("taskkill", "/PID", fmt.Sprint(pid)).Run()
	} else {
		err = executil.HiddenCmd("taskkill", "/IM", appImageName).Run()
	}
	if err != nil {
		s.log.Error("close-app: " + err.Error())
	} else {
		s.log.Info("close-app: graceful close requested")
	}
}

// statusJSON — полный статус службы для IPC status (время — unix-секунды, 0 = не задано).
func (s *Service) statusJSON() string {
	cfg := LoadConfig()
	pid, version, active := s.wd.Snapshot()
	engInstalled, engAutostart, engRunning, engRevives, engLastRevive := s.wd.EngineSnapshot()
	m := map[string]interface{}{
		"ok":                    true,
		"version":               s.Version,
		"bundle_version":        BundleVersion,
		"service_version":       cfg.ServiceVersion,
		"status":                cfg.Status,
		"autostart":             cfg.Autostart,
		"task_exists":           TaskExists(),
		"hidden":                cfg.Hidden,
		"app_path":              cfg.AppPath,
		"app_pid":               pid,
		"app_version":           version,
		"app_hello_active":      active,
		"last_app_start":        unixSec(cfg.LastAppStart),
		"last_exit_kind":        cfg.LastExitKind,
		"last_exit_at":          unixSec(cfg.LastExitAt),
		"crash_count":           cfg.CrashCount,
		"crash_window_start":    unixSec(cfg.CrashWindowStart),
		"watchdog_paused_until": unixSec(cfg.WatchdogPausedUntil),
		"last_crash_reason":     cfg.LastCrashReason,
		"started_at":            unixSec(s.startedAt),
		"engine": map[string]interface{}{
			"installed":   engInstalled,
			"autostart":   engAutostart,
			"running":     engRunning,
			"revives":     engRevives,
			"last_revive": unixSec(engLastRevive),
		},
	}
	b, err := json.Marshal(m)
	if err != nil {
		return respErr("status_marshal_failed")
	}
	return string(b) + "\n"
}

func unixSec(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}
