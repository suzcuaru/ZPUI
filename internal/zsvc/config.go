package zsvc

import (
	"time"

	"golang.org/x/sys/windows/registry"
)

const (
	PipeName    = `\\.\pipe\ZPUI-Service`
	ServiceName = "ZPUI"
	TaskName    = "ZPUI"

	regKeyPath     = `Software\ZPUI\Service`
	installKeyPath = `Software\ZPUI`
	installDirName = "InstallDir"
)

// BundleVersion — версия службы, которую ожидает приложение (инжектится
// ldflags при сборке ZPUI.exe; сверяется с ответом IPC ping).
var BundleVersion = "dev"

const (
	StatusIdle     = "idle"
	StatusWatching = "watching"
	StatusPaused   = "paused"
	StatusFailed   = "failed"
)

const (
	ExitNone   = "none"
	ExitClean  = "clean"
	ExitCrash  = "crash"
	ExitLogoff = "logoff"
)

const (
	crashWindow = time.Hour
	crashLimit  = 5
)

// Config — состояние службы в ветке HKLM\Software\ZPUI\Service.
// Реестр — единственный источник истины; IPC-клиенты и watchdog читают его.
type Config struct {
	Autostart           bool
	Hidden              bool
	AppPath             string
	ServiceVersion      string
	Status              string
	LastAppStart        time.Time
	LastExitKind        string
	LastExitAt          time.Time
	CrashCount          int
	CrashWindowStart    time.Time
	WatchdogPausedUntil time.Time
	LastCrashReason     string
}

// DefaultConfig — значения при отсутствии ветки/полей в реестре.
func DefaultConfig() Config {
	return Config{
		Autostart:    true,
		Hidden:       false,
		AppPath:      "",
		Status:       StatusIdle,
		LastExitKind: ExitNone,
		CrashCount:   0,
	}
}

// LoadConfig читает ветку службы; отсутствующие поля берутся из дефолтов.
func LoadConfig() Config {
	c := DefaultConfig()
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, regKeyPath, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return c
	}
	defer k.Close()
	if v, _, err := k.GetIntegerValue("Autostart"); err == nil {
		c.Autostart = v != 0
	}
	if v, _, err := k.GetIntegerValue("Hidden"); err == nil {
		c.Hidden = v != 0
	}
	if v, _, err := k.GetStringValue("AppPath"); err == nil {
		c.AppPath = v
	}
	if v, _, err := k.GetStringValue("ServiceVersion"); err == nil {
		c.ServiceVersion = v
	}
	if v, _, err := k.GetStringValue("Status"); err == nil && isValidStatus(v) {
		c.Status = v
	}
	c.LastAppStart = getUnixValue(k, "LastAppStart")
	if v, _, err := k.GetStringValue("LastExitKind"); err == nil && isValidExitKind(v) {
		c.LastExitKind = v
	}
	c.LastExitAt = getUnixValue(k, "LastExitAt")
	if v, _, err := k.GetIntegerValue("CrashCount"); err == nil {
		c.CrashCount = int(v)
	}
	c.CrashWindowStart = getUnixValue(k, "CrashWindowStart")
	c.WatchdogPausedUntil = getUnixValue(k, "WatchdogPausedUntil")
	if v, _, err := k.GetStringValue("LastCrashReason"); err == nil {
		c.LastCrashReason = v
	}
	return c
}

// SaveConfig записывает все поля ветки службы.
func SaveConfig(c Config) error {
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, regKeyPath, registry.SET_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return err
	}
	defer k.Close()
	_ = k.SetDWordValue("Autostart", boolToDWORD(c.Autostart))
	_ = k.SetDWordValue("Hidden", boolToDWORD(c.Hidden))
	_ = k.SetStringValue("AppPath", c.AppPath)
	_ = k.SetStringValue("ServiceVersion", c.ServiceVersion)
	_ = k.SetStringValue("Status", c.Status)
	_ = k.SetQWordValue("LastAppStart", timeToUnix(c.LastAppStart))
	_ = k.SetStringValue("LastExitKind", c.LastExitKind)
	_ = k.SetQWordValue("LastExitAt", timeToUnix(c.LastExitAt))
	_ = k.SetDWordValue("CrashCount", uint32(c.CrashCount))
	_ = k.SetQWordValue("CrashWindowStart", timeToUnix(c.CrashWindowStart))
	_ = k.SetQWordValue("WatchdogPausedUntil", timeToUnix(c.WatchdogPausedUntil))
	_ = k.SetStringValue("LastCrashReason", c.LastCrashReason)
	return nil
}

// InstallDir возвращает каталог установки из HKLM\Software\ZPUI\InstallDir
// (значение пишет pkginst при установке/обновлении).
func InstallDir() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, installKeyPath, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue(installDirName)
	if err != nil {
		return ""
	}
	return v
}

func isValidStatus(s string) bool {
	switch s {
	case StatusIdle, StatusWatching, StatusPaused, StatusFailed:
		return true
	}
	return false
}

func isValidExitKind(s string) bool {
	switch s {
	case ExitNone, ExitClean, ExitCrash, ExitLogoff:
		return true
	}
	return false
}

func getUnixValue(k registry.Key, name string) time.Time {
	v, _, err := k.GetIntegerValue(name)
	if err != nil || v == 0 {
		return time.Time{}
	}
	return time.Unix(int64(v), 0)
}

func boolToDWORD(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

func timeToUnix(t time.Time) uint64 {
	if t.IsZero() {
		return 0
	}
	return uint64(t.Unix())
}
