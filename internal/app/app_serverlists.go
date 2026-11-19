package app

// Версии серверных списков и их сверка (переезд из app_v5.go, волна 3A):
// локальные версии хранятся в флагах БД, sync.pull в канале ZPUI Sync
// (app_sync.go) присылает дельты по курсору `have`.

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"zpui/internal/config"
	"zpui/internal/database"
)

// serverListsLocalVersion — версия набора, записанная локально (0 = нет).
func serverListsLocalVersion() int64 {
	return parseFlagInt64("serverlists_version")
}

// serverListsInfoMap — версия набора, время последней успешной сверки
// с сервером и доступность серверных данных (подписка).
func (a *App) serverListsInfoMap() map[string]interface{} {
	// «Версия серверных списков» для UI — версия системного скип-листа
	// (exclusions): наборы general/exclude снесены на сервере.
	synced, _ := database.GetFlag("exclusions_sync_at")
	lastErr, _ := database.GetFlag("serverlists_last_error")
	return map[string]interface{}{
		"version":              exclusionsLocalVersion(),
		"synced_at":            synced,
		"exclusions_version":   exclusionsLocalVersion(),
		"exclusions_synced_at": synced,
		"last_error":           lastErr,
		"available":            a.serverDataAllowed(),
	}
}

// serverDataAllowed — для UI: серверные данные (исключения) показываются
// авторизованному пользователю с активной подпиской. Сам синк этим гейтом
// НЕ ограничен: сервер отдаёт исключения любому зарегистрированному
// устройству с валидным токеном.
func (a *App) serverDataAllowed() bool {
	if ServerURL == "" || !a.authorizationOn() {
		return false
	}
	sc := a.cfg.GetServerConfig()
	if sc.AccountToken == "" {
		return false
	}
	if p := sc.Profile; p != nil && p.Subscription != nil {
		if until, err := time.Parse(time.RFC3339, p.Subscription.Until); err == nil {
			return until.After(time.Now())
		}
	}
	return false
}

// exclusionsLocalVersion — версия исключений, известная клиенту.
func exclusionsLocalVersion() int64 {
	return parseFlagInt64("exclusions_version")
}

// dnsSystemLocalVersion — версия системного списка DNS, записанная локально.
func dnsSystemLocalVersion() int64 {
	return parseFlagInt64("dns_system_version")
}

// parseTimeFlag — RFC3339-время из флага (нулевое = не было).
func parseTimeFlag(v string) time.Time {
	t, _ := time.Parse(time.RFC3339, v)
	return t
}

func parseFlagInt64(key string) int64 {
	v, err := database.GetFlag(key)
	if err != nil || v == "" {
		return 0
	}
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}

// RefreshServerLists — принудительный sync.pull с пустым have: сервер отдаёт
// блоки exclusions и dns_providers целиком, клиент применяет их по правилам
// фонового канала (ApplyPull): замена системных файлов + флаги версий.
func (a *App) RefreshServerLists() map[string]interface{} {
	data, e := a.syncCallMap("sync.pull", a.cfg.GetServerConfig().AccountToken, map[string]any{
		"have": map[string]int64{},
	})
	if e != nil {
		a.log.Warn("serverlists", "refresh: "+e.Code)
		return errResp(e.Code)
	}
	if data == nil {
		return errResp("empty_pull")
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return errResp("bad_response")
	}
	var pd struct {
		Exclusions *struct {
			Version   int64    `json:"version"`
			Unchanged bool     `json:"unchanged"`
			Items     []string `json:"items"`
		} `json:"exclusions"`
		DNSProviders *struct {
			Version   int64 `json:"version"`
			Unchanged bool  `json:"unchanged"`
			Items     []struct {
				ID      string   `json:"id"`
				Name    string   `json:"name"`
				DoH     string   `json:"doh"`
				Blocked bool     `json:"blocked"`
				Reason  string   `json:"reason"`
				IPv4    []string `json:"ipv4"`
			} `json:"items"`
		} `json:"dns_providers"`
	}
	if err := json.Unmarshal(raw, &pd); err != nil {
		return errResp("bad_response")
	}

	if pd.Exclusions != nil && pd.Exclusions.Version > 0 &&
		pd.Exclusions.Version != exclusionsLocalVersion() && !pd.Exclusions.Unchanged {
		if err := a.writeSystemExclusions(pd.Exclusions.Items); err != nil {
			a.log.Warn("exclusions", "write system list: "+err.Error())
		} else {
			_ = database.SetFlag("exclusions_version", strconv.FormatInt(pd.Exclusions.Version, 10))
			a.log.Info("exclusions", fmt.Sprintf("system exclusion list refreshed from server (v%d): %d domains", pd.Exclusions.Version, len(pd.Exclusions.Items)))
		}
	}

	if pd.DNSProviders != nil && pd.DNSProviders.Version > 0 && !pd.DNSProviders.Unchanged &&
		pd.DNSProviders.Version != dnsSystemLocalVersion() {
		entries := make([]config.DnsProvider, 0, len(pd.DNSProviders.Items))
		known := make(map[string]bool, len(pd.DNSProviders.Items))
		for _, p := range pd.DNSProviders.Items {
			// Выключенный/заблокированный провайдер остаётся в списке (виден,
			// включить нельзя), удалённый — исчезает. Флаги заблокированных
			// снимаются (known — без них).
			if p.ID == "" || len(p.IPv4) == 0 {
				continue
			}
			entries = append(entries, config.DnsProvider{
				ID: p.ID, Name: p.Name, IPv4: p.IPv4, DoH: p.DoH,
				Blocked: p.Blocked, Reason: p.Reason,
			})
			if !p.Blocked {
				known[p.ID] = true
			}
		}
		if len(entries) > 0 {
			if err := config.SetSystemProviders(entries); err != nil {
				a.log.Warn("dns", "write system list: "+err.Error())
			} else {
				_ = database.SetFlag("dns_system_version", strconv.FormatInt(pd.DNSProviders.Version, 10))
				a.log.Info("dns", fmt.Sprintf("system DNS list refreshed from server (v%d): %d providers", pd.DNSProviders.Version, len(entries)))
				a.dnsReconfigure()
				a.disableDnsOrphans(known)
			}
		}
	}

	now := time.Now().Format(time.RFC3339)
	_ = database.SetFlag("serverlists_sync_at", now)
	_ = database.SetFlag("exclusions_sync_at", now)
	_ = database.SetFlag("dns_providers_synced_at", now)
	_ = database.SetFlag("serverlists_last_error", "")

	return okRespWithData(map[string]interface{}{
		"dns_version":          dnsSystemLocalVersion(),
		"dns_synced_at":        now,
		"exclusions_version":   exclusionsLocalVersion(),
		"exclusions_synced_at": now,
	})
}
