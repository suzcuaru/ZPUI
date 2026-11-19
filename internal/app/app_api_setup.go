package app

import (
	"net"
	"os"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows/registry"

	"zpui/internal/executil"
)

// zapretServiceOurs — служба «zapret» принадлежит нашей установке: ImagePath
// указывает в наш каталог zapret. Служба zapret2-эпохи тоже наша — сносить её
// как «стороннюю» нельзя (на машинах с прежней установкой гасились служба и
// драйверы WinDivert рабочего движка).
func (a *App) zapretServiceOurs() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\zapret`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	img, _, err := k.GetStringValue("ImagePath")
	if err != nil {
		return false
	}
	img = strings.Trim(strings.TrimPrefix(strings.TrimSpace(img), `\??\`), `"`)
	ours := strings.ToLower(a.cfg.GetZapretPath())
	return ours != "" && strings.Contains(strings.ToLower(img), ours)
}

// DetectThirdPartyZapret проверяет, есть ли на системе сторонний zapret
// (установленный как служба Windows), но отсутствующий в нашей локальной папке.
func (a *App) DetectThirdPartyZapret() map[string]interface{} {
	hasLocal := a.HasLocalZapret()
	if hasLocal {
		return map[string]interface{}{
			"has_local":          true,
			"has_third_party":    false,
			"third_party_detail": "",
		}
	}

	// Проверяем через службу
	output, err := executil.RunWithTimeout(5*time.Second, "sc", "query", "zapret")
	if err == nil && strings.Contains(output, "zapret") {
		if a.zapretServiceOurs() {
			return map[string]interface{}{
				"has_local":          false,
				"has_third_party":    false,
				"third_party_detail": "Служба zapret принадлежит ZPUI",
			}
		}
		detail := extractServiceDetail(output)
		return map[string]interface{}{
			"has_local":          false,
			"has_third_party":    true,
			"third_party_detail": detail,
		}
	}

	// Проверяем через процессы
	procOut, _ := executil.RunWithTimeout(5*time.Second, "tasklist", "/FI", "IMAGENAME eq winws.exe", "/NH")
	if strings.Contains(procOut, "winws.exe") {
		return map[string]interface{}{
			"has_local":          false,
			"has_third_party":    true,
			"third_party_detail": "Обнаружен запущенный процесс winws.exe (сторонний zapret)",
		}
	}

	return map[string]interface{}{
		"has_local":          false,
		"has_third_party":    false,
		"third_party_detail": "",
	}
}

func extractServiceDetail(output string) string {
	lines := strings.Split(output, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "STATE") {
			parts := strings.SplitN(l, ":", 2)
			if len(parts) == 2 {
				return "Служба zapret: " + strings.TrimSpace(parts[1])
			}
		}
	}
	return "Служба zapret установлена"
}

// RemoveThirdPartyZapret удаляет сторонний zapret (службу, процесс, драйверы)
func (a *App) RemoveThirdPartyZapret() map[string]interface{} {
	a.log.Info("setup", "Removing third-party zapret...")
	if a.zapretServiceOurs() {
		a.log.Info("setup", "Служба zapret принадлежит ZPUI — снос пропущен")
		return okResp()
	}
	a.zapret.Teardown()
	a.log.Info("setup", "Third-party zapret removed")
	return okResp()
}

// CheckServiceHealth проверяет состояние службы zapret.
// При проблемах проходит 3 фазы восстановления:
//
//	Фаза 1 — 3 попытки запуска
//	Фаза 2 — переустановка службы + 3 попытки
//	Фаза 3 — скачивание с Яндекс.Диска + установка + 3 попытки
//
// Лог пишется в startup.log в реальном времени.
func (a *App) CheckServiceHealth() map[string]interface{} {
	res := a.RunStartupChecks(a.startupLog)

	// Копируем лог в поле log для фронтенда
	var steps []string
	if a.startupLog != nil {
		data, _ := os.ReadFile(a.startupLog.Path())
		for _, line := range strings.Split(string(data), "\n") {
			if line != "" {
				steps = append(steps, line)
			}
		}
	}
	res["log"] = steps

	// Эмитим прогресс во фронтенд
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "setup:progress", map[string]interface{}{
			"phase":   "service",
			"message": res["status"],
			"current": len(steps),
			"total":   0,
			"percent": 0,
		})
	}

	return res
}

// isNetworkAvailable проверяет доступность сети быстрым TCP-подключением.
// Используется в CheckServiceHealth и AutoInstallZapret, чтобы не пытаться
// скачивать zapret при отсутствии интернета.
func (a *App) isNetworkAvailable() bool {
	conn, err := net.DialTimeout("tcp", "google.com:80", 3*time.Second)
	if err != nil {
		a.log.Warn("setup", "isNetworkAvailable: no network detected")
		return false
	}
	conn.Close()
	return true
}
