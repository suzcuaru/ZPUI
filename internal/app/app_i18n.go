package app

import "fmt"

var notifyTr = map[string]map[string]string{
	"ru": {
		"zpui_update":                 "Доступно обновление ZPUI: %s → %s",
		"zapret_update":               "Доступно обновление: Запрет %s",
		"missing_files":               "Отсутствует файлов: %d. Откройте Настройки для переустановки.",
		"resource_drop":               "Доступность ресурсов упала до %d%%",
		"test_title":                  "ZPUI",
		"test_body":                   "Тестовое уведомление",
		"test_complete":               "Тест стратегий завершён",
		"service_critical_title":      "Служба Запрета",
		"service_critical_body":       "Не удалось восстановить службу запрета после сбоя. Откройте приложение для диагностики.",
		"recovery_success":            "Запрет восстановлен после сбоя и успешно запущен",
		"notify_youtube_dns":          "YouTube не работает из-за подмены DNS. Включите Xbox DNS в настройках.",
		"ipset_updated":               "Список IP-адресов обновлён",
		"ipset_update_failed":         "Не удалось обновить список IP-адресов",
		"dns_source_off":              "DNS-источник «%s» не отвечает и выключен",
		"asm_provider":                "Провайдер: %s",
		"asm_provider_detecting":      "Определяем провайдера…",
		"asm_provider_unknown":        "Провайдер не определён",
		"asm_server_ok":               "Получены данные сервера",
		"asm_server_fetch":            "Запрашиваем сервер…",
		"asm_server_local":            "Сервер недоступен — работаем по локальным данным",
		"asm_server_empty":            "Серверу нечего предложить — работаем по локальным данным",
		"asm_server_plan":             "Стратегий из плана нет на диске — работаем по локальным данным",
		"asm_server_none":             "На сервере нет подходящих стратегий — работаем по локальным данным",
		"asm_candidates_server":       "Кандидаты сервера: %d стратегий",
		"asm_local_data":              "Локальные данные: %d стратегий",
		"asm_local_all":               "Локальных данных нет — проверим все стратегии (%d)",
		"asm_ready":                   "К прогону: %d стратегий, порог балла %d",
		"asm_sweep":                   "Запускаем перебор: %d стратегий",
		"asm_game_selected":           "Выбрано стратегий: %d",
		"asm_game_top":                "Игровой топ: %d стратегий",
		"asm_game_run":                "Игровой прогон: %d стратегий",
		"asm_game_sweep":              "Запускаем игровой подбор: %d стратегий",
		"asm_stage_filter":            "Этап 1 — быстрый фильтр: %d стратегий (только ключевые ресурсы)",
		"asm_stage_final":             "Этап 2 — полный тест: %d финалистов",
		"asm_filter_none":             "Ни у одной стратегии ключевые ресурсы не заработали",
		"asm_base":                    "Замеряем прямой канал без обхода — база для баллов стратегий",
		"asm_check":                   "Проверяем доступность на стратегии-победителе…",
		"dns_not_applied":             "DNS-серверы не применились — тумблер DNS выключен",
		"asm_startup_sweep":           "Полный перебор: %d стратегий",
		"asm_startup_last":            "Проверяем прошлую стратегию оператора",
		"auto_switch_title":           "Автосмена стратегии",
		"tgproxy_missing":             "Компонент моста Telegram не установлен — переустановите ZPUI",
		"auto_switch_applied":         "Доступность упала до %d%% — включена стратегия %s",
		"sync_title":                  "Синхронизация устройств",
		"sync_limit":                  "Слоты синхронизации устройств исчерпаны — перенос снапшота недоступен на текущем тарифе",
		"operator_changed_title":      "Смена оператора",
		"operator_changed_applied":    "Оператор изменился: %s. Применена стратегия %s",
		"operator_changed_autoselect": "Оператор изменился: %s. Запущен подбор стратегии...",
	},
	"en": {
		"zpui_update":                 "ZPUI update available: %s → %s",
		"zapret_update":               "Update available: Zapret %s",
		"missing_files":               "Missing %d file(s). Open Settings to reinstall.",
		"resource_drop":               "Resource availability dropped to %d%%",
		"test_title":                  "ZPUI",
		"test_body":                   "Test notification",
		"test_complete":               "Strategy test complete",
		"service_critical_title":      "Zapret Service",
		"service_critical_body":       "Failed to recover zapret service after crash. Open the app for diagnostics.",
		"recovery_success":            "Zapret recovered after crash and started successfully",
		"notify_youtube_dns":          "YouTube is broken by DNS poisoning. Enable Xbox DNS in settings.",
		"ipset_updated":               "IP address list updated",
		"ipset_update_failed":         "Failed to update the IP address list",
		"dns_source_off":              "DNS source \"%s\" is not responding and has been disabled",
		"asm_provider":                "Provider: %s",
		"asm_provider_detecting":      "Detecting the provider…",
		"asm_provider_unknown":        "Provider not detected",
		"asm_server_ok":               "Server data received",
		"asm_server_fetch":            "Requesting the server…",
		"asm_server_local":            "Server unavailable — using local data",
		"asm_server_empty":            "Server has nothing to offer — using local data",
		"asm_server_plan":             "Plan strategies missing on disk — using local data",
		"asm_server_none":             "No suitable strategies on the server — using local data",
		"asm_candidates_server":       "Server candidates: %d strategies",
		"asm_local_data":              "Local data: %d strategies",
		"asm_local_all":               "No local data — testing all strategies (%d)",
		"asm_ready":                   "Ready to test: %d strategies, score threshold %d",
		"asm_sweep":                   "Starting the sweep: %d strategies",
		"asm_game_selected":           "Selected strategies: %d",
		"asm_game_top":                "Game top: %d strategies",
		"asm_game_run":                "Game run: %d strategies",
		"asm_game_sweep":              "Starting the game selection: %d strategies",
		"asm_stage_filter":            "Stage 1 — quick filter: %d strategies (key resources only)",
		"asm_stage_final":             "Stage 2 — full test: %d finalists",
		"asm_filter_none":             "Key resources failed on every strategy",
		"asm_base":                    "Measuring the direct channel without bypass — the baseline for strategy scores",
		"asm_check":                   "Checking availability on the winning strategy…",
		"dns_not_applied":             "DNS servers could not be applied — DNS toggle turned off",
		"asm_startup_sweep":           "Full sweep: %d strategies",
		"asm_startup_last":            "Testing the operator's previous strategy",
		"auto_switch_title":           "Auto strategy switch",
		"tgproxy_missing":             "Telegram bridge component is not installed — reinstall ZPUI",
		"auto_switch_applied":         "Availability dropped to %d%% — switched to %s",
		"sync_title":                  "Device sync",
		"sync_limit":                  "Device sync slots are used up — snapshot transfer is not available on this plan",
		"operator_changed_title":      "Operator changed",
		"operator_changed_applied":    "Operator changed: %s. Applied strategy %s",
		"operator_changed_autoselect": "Operator changed: %s. Strategy selection started...",
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

// trCur — перевод по языку пользователя: строки пользовательских событий
// (автоподбор и т.п.) локализуются на стороне Go.
func (a *App) trCur(key string, args ...interface{}) string {
	return tr(a.cfg.GetLanguage(), key, args...)
}
