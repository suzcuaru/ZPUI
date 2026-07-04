package main

import "fmt"

var notifyTr = map[string]map[string]string{
	"ru": {
		"zpui_update":    "Доступно обновление ZPUI: %s → %s",
		"zapret_update":  "Доступно обновление Zapret: %s → %s",
		"missing_files":  "Отсутствует файлов: %d. Откройте Настройки для переустановки.",
		"resource_drop":  "Доступность ресурсов упала до %d%%",
		"test_title":     "ZPUI",
		"test_body":      "Тестовое уведомление",
		"test_complete":  "Тест стратегий завершён",
	},
	"en": {
		"zpui_update":    "ZPUI update available: %s → %s",
		"zapret_update":  "Zapret update available: %s → %s",
		"missing_files":  "Missing %d file(s). Open Settings to reinstall.",
		"resource_drop":  "Resource availability dropped to %d%%",
		"test_title":     "ZPUI",
		"test_body":      "Test notification",
		"test_complete":  "Strategy test complete",
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
