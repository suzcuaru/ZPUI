//go:build windows

package notify

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// extractIconFile выгружает встроенную иконку приложения в %APPDATA%\ZPUI\ —
// иконка в шапке уведомления (строка с именем приложения) берётся Windows
// из AUMID-регистрации и должна существовать как файл на диске.
func extractIconFile() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		logWarn("icon dir error: " + err.Error())
		return ""
	}
	dir = filepath.Join(dir, "ZPUI")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logWarn("icon mkdir error: " + err.Error())
		return ""
	}
	iconPath := filepath.Join(dir, "appicon.ico")
	if err := os.WriteFile(iconPath, iconICO, 0o644); err != nil {
		logWarn("icon write error: " + err.Error())
		return ""
	}
	_ = os.Remove(filepath.Join(dir, "notify-icon.png"))
	return iconPath
}

// resetAumidIconValues удаляет прежние значения иконки AUMID из реестра:
// go-toast не перезаписывает существующие, поэтому без сброса старый
// фиолетовый фон остался бы у обновлённых установок навсегда.
func resetAumidIconValues(aumid string) {
	key, err := registry.OpenKey(registry.CURRENT_USER,
		filepath.Join("SOFTWARE", "Classes", "AppUserModelId", aumid),
		registry.SET_VALUE)
	if err != nil {
		return
	}
	defer key.Close()
	_ = key.DeleteValue("IconUri")
	_ = key.DeleteValue("IconBackgroundColor")
}
