package app

import "fmt"

var notifyTr = map[string]map[string]string{
	"ru": {
		"zpui_update":            "Доступно обновление ZPUI: %s → %s",
		"zapret_update":          "Доступно обновление: Запрет %s",
		"missing_files":          "Отсутствует файлов: %d. Откройте Настройки для переустановки.",
		"resource_drop":          "Доступность ресурсов упала до %d%%",
		"test_title":             "ZPUI",
		"test_body":              "Тестовое уведомление",
		"test_complete":          "Тест стратегий завершён",
		"service_critical_title": "Служба Запрета",
		"service_critical_body":  "Не удалось восстановить службу запрета после сбоя. Откройте приложение для диагностики.",
		"recovery_success":       "Запрет восстановлен после сбоя и успешно запущен",
	},
	"en": {
		"zpui_update":            "ZPUI update available: %s → %s",
		"zapret_update":          "Update available: Zapret %s",
		"missing_files":          "Missing %d file(s). Open Settings to reinstall.",
		"resource_drop":          "Resource availability dropped to %d%%",
		"test_title":             "ZPUI",
		"test_body":              "Test notification",
		"test_complete":          "Strategy test complete",
		"service_critical_title": "Zapret Service",
		"service_critical_body":  "Failed to recover zapret service after crash. Open the app for diagnostics.",
		"recovery_success":       "Zapret recovered after crash and started successfully",
	},
}

func tr(lang, key string, args ...interface{}) string {
	m, ok := notifyTr[lang]
	if !ok {
		m = notifyTr["en"]
	}
	s, ok := m[key]
	if !ok {
		s = notifyTr["en"][key]
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}
