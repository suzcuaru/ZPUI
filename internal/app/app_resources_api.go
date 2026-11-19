package app

import (
	"sort"
	"time"
	"zpui/internal/blockcheck"
	"zpui/internal/database"
	"zpui/internal/zapret"

)


// ============================================================
// RESOURCE STATUS
// ============================================================

// resourceDetailsItem — строка модалки панели ресурсов: текущий статус
// последней проверки + аптайм (процент успешных проверок за сутки из БД).
type resourceDetailsItem struct {
	Name    string  `json:"name"`
	OK      bool    `json:"ok"`
	Reason  string  `json:"reason,omitempty"`
	Uptime  float64 `json:"uptime"`     // % за 24ч; -1 — данных нет
	Latency int64   `json:"latency_ms"` // отклик последней проверки, мс
	Key     bool    `json:"key,omitempty"`
}

// ResourceDetails — содержимое для модалок панелей дашборда.
// typ: standard | user | key («основные ресурсы» — как при подборе:
// discord/youtube/steam из стандартного списка). Ответ: checked_at_unix +
// items с текущим статусом и аптаймом.
func (a *App) ResourceDetails(typ string) map[string]interface{} {
	a.resourceCacheMu.Lock()
	report := a.resourceCache
	checkedAt := a.resourceCacheTime
	a.resourceCacheMu.Unlock()

	if report == nil {
		return okRespWithData(map[string]interface{}{"checked_at_unix": int64(0), "items": []resourceDetailsItem{}})
	}

	var src []blockcheck.BulkResult
	switch typ {
	case "user":
		src = report.User
	case "key":
		for _, r := range report.Default {
			if zapret.IsKeyTarget(r.Name, r.URL) {
				src = append(src, r)
			}
		}
	default:
		src = report.Default
	}

	// Аптайм за сутки одним запросом: группируем по host+type в памяти.
	upt := map[string]float64{}
	if hist, err := database.GetResourceAvailabilityHistory(time.Now().Add(-24*time.Hour), ""); err == nil {
		type agg struct{ total, ok int }
		m := map[string]*agg{}
		for _, h := range hist {
			k := h.Type + ":" + h.Host
			a2 := m[k]
			if a2 == nil {
				a2 = &agg{}
				m[k] = a2
			}
			a2.total++
			if h.Ok {
				a2.ok++
			}
		}
		for k, a2 := range m {
			if a2.total > 0 {
				upt[k] = float64(a2.ok) / float64(a2.total) * 100
			}
		}
	}

	items := make([]resourceDetailsItem, 0, len(src))
	for _, r := range src {
		it := resourceDetailsItem{
			Name:    r.Name,
			OK:      r.OK,
			Reason:  r.Verdict,
			Uptime:  -1,
			Latency: r.LatencyMs,
			Key:     zapret.IsKeyTarget(r.Name, r.URL),
		}
		if u, ok := upt[typ+":"+r.Name]; ok && typ != "key" {
			it.Uptime = u
		} else if u, ok := upt["standard:"+r.Name]; ok {
			it.Uptime = u
		}
		items = append(items, it)
	}
	return okRespWithData(map[string]interface{}{
		"checked_at_unix": checkedAt.Unix(),
		"items":           items,
	})
}

// GetResourceStatus returns cached resource status.
// NEVER triggers a new check - only returns what is in cache.
// Checks are done by startResourceMonitor (5/10 min) and RefreshResourceStatus (manual).
// Frontend polls this every 30s to get checking_now flag + cached data.
func (a *App) GetResourceStatus() map[string]interface{} {
	a.resourceCacheMu.Lock()
	report := a.resourceCache
	checkedAt := a.resourceCacheTime
	a.resourceCacheMu.Unlock()

	if report == nil {
		return map[string]interface{}{
			"default":         []blockcheck.BulkResult{},
			"user":            []blockcheck.BulkResult{},
			"checked_at":      "",
			"checked_at_unix": 0,
			"cached":          false,
			"checking_now":    a.IsCheckingNow(),
		}
	}

	// Always return cached data (even if old). Stale = true if older than 30s.
	stale := time.Since(checkedAt) > 30*time.Second
	return a.buildResourceStatusResponse(report, checkedAt, stale)
}

// RefreshResourceStatus starts a manual check ASYNCHRONOUSLY.
// Returns immediately with checking_now=true so frontend can show
// "checking..." and disable the button. Frontend polls GetResourceStatus
// every 2-3s to see when checking_now becomes false (check complete).
func (a *App) RefreshResourceStatus() map[string]interface{} {
	// If already checking, just return current state
	if a.IsCheckingNow() {
		return a.GetResourceStatus()
	}

	// Start check in background
	go a.doResourceCheckAndSave()

	// Return immediately with checking_now=true
	a.resourceCacheMu.Lock()
	report := a.resourceCache
	checkedAt := a.resourceCacheTime
	a.resourceCacheMu.Unlock()

	if report == nil {
		return map[string]interface{}{
			"default":      []blockcheck.BulkResult{},
			"user":         []blockcheck.BulkResult{},
			"checking_now": true,
			"started":      true,
		}
	}
	resp := a.buildResourceStatusResponse(report, checkedAt, false)
	resp["started"] = true
	return resp
}

// buildResourceStatusResponse assembles response with check metadata:
//
//	checked_at      - ISO time of last check
//	checked_at_unix - unix timestamp (for "N sec ago")
//	cached          - true if response from cache
//	default / user  - result arrays
func (a *App) buildResourceStatusResponse(report *blockcheck.BulkReport, checkedAt time.Time, cached bool) map[string]interface{} {
	return map[string]interface{}{
		"default":                 report.Default,
		"user":                    report.User,
		"checked_at":              checkedAt.Format("2006-01-02 15:04:05"),
		"checked_at_unix":         checkedAt.Unix(),
		"cached":                  cached,
		"checking_now":            a.IsCheckingNow(),
		"resource_check_interval": a.cfg.GetResourceCheckInterval(),
	}
}

// ============================================================
// AVAILABILITY HISTORY
// ============================================================

func (a *App) GetAvailabilityHistory(hours int, typ string) map[string]interface{} {
	if hours <= 0 {
		hours = 24
	}
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	all, err := database.GetAvailabilityHistory(since)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	if all == nil {
		all = []database.AvailabilityRecord{}
	}
	// Фильтр по типу; type=key — агрегат по основным ресурсам (discord/
	// youtube/steam) из per-host проверок:_pct на каждый такт проверки.
	var records []database.AvailabilityRecord
	if typ == "key" {
		hist, err := database.GetResourceAvailabilityHistory(since, "")
		if err == nil {
			type agg struct{ total, ok int }
			byTs := map[time.Time]*agg{}
			for _, h := range hist {
				if h.Type != "standard" || !zapret.IsKeyTarget(h.Host, "") {
					continue
				}
				a2 := byTs[h.Timestamp]
				if a2 == nil {
					a2 = &agg{}
					byTs[h.Timestamp] = a2
				}
				a2.total++
				if h.Ok {
					a2.ok++
				}
			}
			for ts, a2 := range byTs {
				if a2.total == 0 {
					continue
				}
				records = append(records, database.AvailabilityRecord{
					Timestamp:      ts,
					Type:           "key",
					TotalResources: a2.total,
					OKResources:    a2.ok,
					Pct:            float64(a2.ok) / float64(a2.total) * 100,
				})
			}
			sort.Slice(records, func(i, j int) bool { return records[i].Timestamp.Before(records[j].Timestamp) })
		}
	} else {
		for _, r := range all {
			if typ == "" || typ == "all" || r.Type == typ {
				records = append(records, r)
			}
		}
	}
	return map[string]interface{}{
		"records": records,
		"count":   len(records),
	}
}
