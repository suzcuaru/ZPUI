package app

// Серверная функциональность ZPUI: доступность сервера управления, профиль
// аккаунта. Телеметрия/отчёты снесены (волна 3A): данные уходят через канал
// ZPUI Sync (app_sync.go), транспорт — POST /api/sync (см. Документы/Задачи/
// ZPUI-Sync/02-Протокол.md). Осталась v1-проверка обновлений ZPUI
// (app_server_updates.go) — инвариант обновляемости.

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"zpui/internal/config"
	"zpui/internal/database"
	"zpui/internal/power"
)

var ServerURL = "http://127.0.0.1:4430"

// ServerURLValue — адрес сервера для диагностики (-diag).
func ServerURLValue() string { return ServerURL }

// Доступность сервера управления для индикатора в статус-баре.
var (
	serverAvailMu   sync.Mutex
	serverAvailable bool
	serverPingAt    time.Time
)

// authorizationOn — включена ли серверная функциональность (config.json:
// authorization_enabled).
func (a *App) authorizationOn() bool {
	return a.cfg.GetAuthorizationEnabled()
}

// PingServer — доступен ли сервер управления (кэш на 60с, фон не блокирует UI).
// Сетевой код отдаётся ТОЛЬКО при реальной сетевой ошибке (dial/TLS/timeout):
// любой HTTP-ответ, включая 401/404/5xx, означает «сервер жив».
func (a *App) PingServer() map[string]interface{} {
	if !a.authorizationOn() {
		return map[string]interface{}{"available": false, "reason": "authorization disabled"}
	}
	serverAvailMu.Lock()
	cached := time.Since(serverPingAt) < 60*time.Second
	avail := serverAvailable
	serverAvailMu.Unlock()
	if !cached {
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(ServerURL + "/healthz")
		if resp != nil {
			resp.Body.Close()
		}
		if err != nil {
			// Ошибки транспорта: 404/5xx сюда не попадают.
			serverAvailMu.Lock()
			serverAvailable = false
			serverAvailMu.Unlock()
			return map[string]interface{}{"available": false, "url": ServerURL, "error": netErrCode(err)}
		}
		avail = true
		serverAvailMu.Lock()
		serverAvailable = avail
		serverPingAt = time.Now()
		serverAvailMu.Unlock()
	}
	return map[string]interface{}{"available": avail, "url": ServerURL}
}

// startServerPingLoop — держит кэш доступности сервера (serverAvailable)
// свежим в фоне: индикатор в футере и /api/server/status читают `available`
// без ожидания сети. Экономный режим растягивает опрос.
func (a *App) startServerPingLoop() {
	ping := func() {
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(ServerURL + "/healthz")
		if resp != nil {
			resp.Body.Close()
		}
		serverAvailMu.Lock()
		serverAvailable = err == nil
		serverPingAt = time.Now()
		serverAvailMu.Unlock()
	}
	time.Sleep(5 * time.Second)
	ping()
	for {
		select {
		case <-a.stopCh:
			return
		case <-time.After(power.Stretch(60 * time.Second)):
		}
		if !a.authorizationOn() {
			continue
		}
		ping()
	}
}

func (a *App) GetServerStatus() map[string]interface{} {
	serverAvailMu.Lock()
	avail := serverAvailable
	serverAvailMu.Unlock()
	return map[string]interface{}{
		"url":         ServerURL,
		"account_set": a.cfg.GetServerConfig().AccountToken != "",
		"enabled":     a.authorizationOn(),
		// «Больше не показывать» у предложения авторизоваться при запуске.
		"auth_prompt_dismissed": a.cfg.GetServerConfig().AuthPromptDismissed,
		// Фактическая доступность сервера управления (кэш PingServer).
		"available": avail,
	}
}

// SetAuthPromptDismissed — «Больше не показывать» у предложения
// авторизоваться при запуске без учётной записи.
func (a *App) SetAuthPromptDismissed(on bool) map[string]interface{} {
	if err := a.cfg.UpdateServerConfig(func(s *config.ServerConfig) { s.AuthPromptDismissed = on }); err != nil {
		return errResp("config: " + err.Error())
	}
	return okResp()
}

// GetYandexNetMetrics — история замеров Интернетметра (скорость+пинг)
// для графиков монитора и блока статистики. Хранение ≤1 час: окно чтения
// клампится к 60 минутам (записи старше часа чистятся при вставке).
func (a *App) GetYandexNetMetrics(minutes int) map[string]interface{} {
	if minutes <= 0 || minutes > 60 {
		minutes = 60
	}
	since := time.Now().Add(-time.Duration(minutes) * time.Minute)
	hist, err := database.GetYandexMetrics(since, 500)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	if hist == nil {
		hist = []database.YandexMetric{}
	}
	latest, _ := database.GetLatestYandexMetric()
	return map[string]interface{}{
		"metrics": hist,
		"latest":  latest,
		"count":   len(hist),
	}
}

// netErrCode — сетевые ошибки маппятся в коды isNetError-набора фронтенда
// (офлайн-оверлей «сервер недоступен»): timeout, connection_refused,
// server_unreachable, network.
func netErrCode(err error) string {
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "timeout"), strings.Contains(s, "deadline exceeded"), strings.Contains(s, "tls handshake"):
		return "timeout"
	case strings.Contains(s, "connection refused"):
		return "connection_refused"
	case strings.Contains(s, "no such host"), strings.Contains(s, "unreachable"), strings.Contains(s, "connection reset"), strings.Contains(s, "broken pipe"), strings.Contains(s, "canceled"):
		return "server_unreachable"
	default:
		return "network"
	}
}

// AccountMe — профиль аккаунта (op account.get через /api/sync):
// {login, email, subscription_id, created_at, subscription{active, id, name,
// price, ends_at, days_left, состав}}; при недоступном сервере — из кеша.
func (a *App) AccountMe() map[string]interface{} {
	return a.V4AccountGet()
}
