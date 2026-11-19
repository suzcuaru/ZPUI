package app

// Сборщики секций каталога ZPUI Sync (03-Секции-данных.md §2–3).
// Данные — из SQLite (internal/database), журналов (internal/logdb) и файлов
// (config.json, skip-файлы) — НЕ из internal/reports (снесён).
// Все поля snake_case; в каждом payload — collected_at (добавляет раннер).

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"golang.org/x/sys/windows/registry"
	"zpui/internal/blockcheck"
	"zpui/internal/config"
	"zpui/internal/database"
	"zpui/internal/executil"
	"zpui/internal/logdb"
	"zpui/internal/power"
	"zpui/internal/sysinfo"
	"zpui/internal/updater"
	"zpui/internal/zapret"
	"zpui/internal/zsvc"
)

// Collect — payload секции по имени операции (данные без collected_at —
// его добавляет раннер).
func (a *App) Collect(op string) (map[string]any, error) {
	switch op {
	case "data.status":
		return a.sectionStatus(), nil
	case "data.runtime":
		return a.sectionRuntime(), nil
	case "data.availability":
		return a.sectionAvailability(), nil
	case "data.resources":
		return a.sectionResources(), nil
	case "data.network":
		return a.sectionNetwork(), nil
	case "data.strategies":
		return a.sectionStrategies(), nil
	case "data.traffic":
		return a.sectionTraffic(), nil
	case "data.operator":
		return a.sectionOperator(), nil
	case "data.errors":
		return a.sectionErrors(), nil
	case "data.events":
		return a.sectionEvents(), nil
	case "data.system":
		return a.sectionSystem(), nil
	case "data.versions":
		return a.sectionVersions(), nil
	case "data.config":
		return a.sectionConfig(), nil
	case "data.exclusions":
		return a.sectionExclusions(), nil
	}
	return nil, fmt.Errorf("unknown section: %s", op)
}

// ---- data.status (~100 Б — основной heartbeat) ----

func (a *App) sectionStatus() map[string]any {
	cfg := zsvc.LoadConfig()
	return map[string]any{
		"app_running":     true,
		"watch":           cfg.Status,
		"power_saving":    power.Active(),
		"zpui_version":    a.version,
		"service_version": cfg.ServiceVersion,
		"uptime_sec":      int64(time.Since(a.bootAt) / time.Second),
		"last_exit_kind":  cfg.LastExitKind,
		"crash_count":     cfg.CrashCount,
	}
}

// ---- data.runtime (живое состояние) ----

func (a *App) sectionRuntime() map[string]any {
	status := a.zapret.GetStatus()
	zapretUp := int64(0)
	if status == zapret.StatusRunning {
		if a.zapretUpAt.Load() == 0 {
			a.zapretUpAt.Store(time.Now().Unix())
		}
		zapretUp = int64(time.Since(time.Unix(a.zapretUpAt.Load(), 0)) / time.Second)
	} else {
		a.zapretUpAt.Store(0)
	}

	tgCfg := a.cfg.GetTgProxyConfig()
	tgRunning := a.tgproxy.IsRunning()
	tgStats := map[string]any{"running": tgRunning, "port": tgCfg.Port}
	if tgRunning {
		st := a.tgproxy.GetStats()
		tgStats["active_conn"] = st["active"]
		tgStats["bytes_in"] = st["bytes_in"]
		tgStats["bytes_out"] = st["bytes_out"]
	}

	var proxyPort int
	var xdEnabled, xdXboxEnabled bool
	a.cfg.RView(func() {
		proxyPort = a.cfg.Proxy.Port
		xdEnabled = a.cfg.XboxDns.Enabled
		xdXboxEnabled = a.cfg.XboxDns.XboxEnabled
	})

	snap := map[string]any{
		"zapret":            string(status),
		"strategy":          a.cfg.GetCurrentStrategy(),
		"zapret_uptime_sec": zapretUp,
		"tgproxy":           tgStats,
		"socks":             map[string]any{"running": a.socks.IsRunning(), "port": proxyPort},
		// Плашка CRM «Xbox DNS»: состояние пресета xbox-dns.ru, а не мастер-тумблера
		// (мастер живёт в data.config.dns.enabled).
		"xbox_dns":  xdEnabled && xdXboxEnabled,
		"public_ip": a.cachedPublicIP(true),
		"local_ips": localIPs(),
		"live":      true,
	}
	a.cacheRuntimeSnapshot(snap)
	return snap
}

// cacheRuntimeSnapshot — дублируем снапшот в флаги БД (last-known для
// этапа B, когда цикл переедет в службу): live-флаг и возраст по collected_at.
func (a *App) cacheRuntimeSnapshot(snap map[string]any) {
	raw, err := json.Marshal(snap)
	if err != nil {
		return
	}
	_ = database.SetFlag("sync_runtime_cache", string(raw))
}

// cachedPublicIP — публичный IP с кешем на 30 минут (секция runtime ходит
// раз в 5 минут — внешний сервис дёргать с такой частотой нельзя).
func (a *App) cachedPublicIP(refresh bool) string {
	if raw, err := database.GetFlag("sync_public_ip_at"); err == nil && raw != "" {
		if t, perr := time.Parse(time.RFC3339, raw); perr == nil && time.Since(t) < 30*time.Minute {
			if ip, err := database.GetFlag("sync_public_ip"); err == nil && ip != "" {
				return ip
			}
		}
	}
	if !refresh {
		ip, _ := database.GetFlag("sync_public_ip")
		return ip
	}
	bc := a.cfg.GetBlockCheckConfig()
	checker := blockcheck.NewChecker(bc.CheckTCP, bc.CheckTLS, bc.CheckHTTP, bc.TimeoutSec)
	ip := checker.GetProviderInfo().IP
	if ip != "" {
		_ = database.SetFlag("sync_public_ip", ip)
		_ = database.SetFlag("sync_public_ip_at", time.Now().Format(time.RFC3339))
	}
	return ip
}

func localIPs() []string {
	ifaces, err := net.InterfaceAddrs()
	if err != nil {
		return []string{}
	}
	var ips []string
	for _, addr := range ifaces {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() {
			continue
		}
		// link-local и multicast — служебный мусор
		if ipnet.IP.IsLinkLocalUnicast() || ipnet.IP.IsLinkLocalMulticast() {
			continue
		}
		if v4 := ipnet.IP.To4(); v4 != nil && v4[0] == 169 && v4[1] == 254 {
			continue
		}
		ips = append(ips, ipnet.IP.String())
	}
	if ips == nil {
		ips = []string{}
	}
	return ips
}

// ---- дифф-хелперы ----

// sinceCursor — граница «после курсора»; пустой курсор — последние сутки.
func sinceCursor(cursor string) time.Time {
	if t, err := time.Parse(time.RFC3339, cursor); err == nil && !t.IsZero() {
		return t
	}
	return time.Now().Add(-24 * time.Hour)
}

// downsample прореживает историю до max точек с сохранением формы.
func downsample[T any](rows []T, max int) []T {
	if len(rows) <= max {
		return rows
	}
	k := (len(rows) + max - 1) / max
	out := make([]T, 0, max+1)
	for i := 0; i < len(rows); i += k {
		out = append(out, rows[i])
	}
	if (len(rows)-1)%k != 0 {
		out = append(out, rows[len(rows)-1])
	}
	return out
}

// ---- data.availability (дифф) ----

func syncAvailabilityRows(cursor string) []map[string]any {
	since := sinceCursor(cursor)
	rows, err := database.GetAvailabilityHistory(since)
	if err != nil {
		return nil
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		pct := 0.0
		if r.TotalResources > 0 {
			pct = float64(r.OKResources) * 100 / float64(r.TotalResources)
		}
		out = append(out, map[string]any{
			"ts":    r.Timestamp.Format(time.RFC3339),
			"kind":  r.Type,
			"ok":    r.OKResources,
			"total": r.TotalResources,
			"pct":   pct,
		})
	}
	return downsample(out, 2000)
}

func (a *App) sectionAvailability() map[string]any {
	cursor := a.cursorValue("data.availability")
	return map[string]any{
		"cursor": cursor,
		"rows":   syncAvailabilityRows(cursor),
	}
}

// ---- data.resources (дифф, суточные агрегаты по хостам) ----

func (a *App) sectionResources() map[string]any {
	cursor := a.cursorValue("data.resources")
	since := cursor
	if len(since) != 10 {
		since = time.Now().AddDate(0, 0, -8).Format("2006-01-02")
	}
	rows, err := database.GetResourceDailySince(since)
	out := []map[string]any{}
	if err == nil {
		for _, r := range rows {
			if cursor != "" && r.Date <= cursor {
				continue
			}
			out = append(out, map[string]any{
				"date":     r.Date,
				"operator": r.OperatorKey,
				"host":     r.Host,
				"total":    r.ChecksTotal,
				"ok":       r.ChecksOK,
				// доступность = pct, аптайм = тот же суточный процент
				// (проверки раз в 5–10 минут, пропусков нет)
				"pct":    r.Pct,
				"uptime": r.Pct,
				"key":    zapret.IsKeyTarget(r.Host, ""),
			})
		}
	}
	// Сводка по основным ресурсам (как при подборе): доступность из
	// последнего кеша проверки. Сервер пока может просто складывать.
	keys := map[string]any{"total": 0, "ok": 0, "pct": -1}
	a.resourceCacheMu.Lock()
	rep := a.resourceCache
	a.resourceCacheMu.Unlock()
	if rep != nil {
		total, ok := 0, 0
		for _, r := range rep.Default {
			if !zapret.IsKeyTarget(r.Name, r.URL) {
				continue
			}
			total++
			if r.OK {
				ok++
			}
		}
		if total > 0 {
			keys = map[string]any{"total": total, "ok": ok, "pct": float64(ok) / float64(total) * 100}
		}
	}
	return map[string]any{"cursor": cursor, "rows": out, "keys": keys}
}

// ---- data.network ----

func (a *App) sectionNetwork() map[string]any {
	latest, _ := database.GetLatestYandexMetric()
	var latestM map[string]any
	rows := []map[string]any{}
	if latest != nil {
		latestM = map[string]any{
			"at":         latest.Timestamp.Format(time.RFC3339),
			"ping_ms":    latest.PingMs,
			"speed_mbps": latest.SpeedMbps,
		}
		if hist, err := database.GetYandexMetrics(time.Now().Add(-24*time.Hour), 200); err == nil {
			for _, m := range hist {
				rows = append(rows, map[string]any{
					"at":         m.Timestamp.Format(time.RFC3339),
					"ping_ms":    m.PingMs,
					"speed_mbps": m.SpeedMbps,
				})
			}
		}
	}
	return map[string]any{"latest": latestM, "rows": downsample(rows, 200)}
}

// ---- data.strategies ----

// parseAutoTestNet — пинг/скорость из сохранённых результатов автоподбора.
func parseAutoTestNet(raw string) (pingMs int64, speedMbps float64) {
	if raw == "" {
		return 0, 0
	}
	var results []struct {
		AvgMs           int64   `json:"avg_ms"`
		YandexPingMs    int64   `json:"yandex_ping_ms"`
		YandexSpeedMbps float64 `json:"yandex_speed_mbps"`
	}
	if err := json.Unmarshal([]byte(raw), &results); err != nil {
		return 0, 0
	}
	for _, r := range results {
		ping := r.YandexPingMs
		if ping == 0 {
			ping = r.AvgMs
		}
		return ping, r.YandexSpeedMbps
	}
	return 0, 0
}

func (a *App) sectionStrategies() map[string]any {
	op, err := database.GetCurrentOperator()
	if err != nil || op == nil || op.OperatorKey == "" {
		return map[string]any{"rows": []map[string]any{}, "games": []map[string]any{}}
	}
	list, err := database.GetOperatorStrategies(op.OperatorKey)
	rows := []map[string]any{}
	recorded := map[string]bool{}
	if err == nil {
		for _, s := range list {
			if s.AvailabilityPct < 0 {
				continue // стратегия ни разу не тестировалась
			}
			recorded[s.Strategy] = true
			ping, speed := parseAutoTestNet(s.AutoTestResults)
			lastApplied := ""
			if s.TestedAt != nil {
				lastApplied = s.TestedAt.Format(time.RFC3339)
			}
			row := map[string]any{
				"strategy":     s.Strategy,
				"availability": s.AvailabilityPct,
				"use_count":    s.UseCount,
				"source":       s.LastSource,
				"last_applied": lastApplied,
				"active":       s.IsActive,
				"ping_ms":      ping,
				"speed_mbps":   speed,
			}
			// компоненты тира для CRM: ключевые ресурсы и слабое звено из
			// последнего сохранённого теста этой стратегии (zapret.score)
			if info, ok := a.zapret.ScoreFromStoredResults(s.AutoTestResults, s.Strategy); ok && info.KeyResources >= 0 {
				row["key_resources"] = info.KeyResources
				if info.Weakest != "" {
					row["weakest"] = info.Weakest
				}
			}
			rows = append(rows, row)
		}
	}
	games := []map[string]any{}
	if gl, err := database.GetStrategyGames(); err == nil {
		for _, g := range gl {
			if !recorded[g.Strategy] {
				continue
			}
			games = append(games, map[string]any{
				"strategy": g.Strategy,
				"score":    g.Score,
				"ping_ms":  g.PingMs,
				"ok":       g.Ok,
				"total":    g.Total,
				"gf_tcp":   g.GfTCP,
				"gf_udp":   g.GfUDP,
			})
		}
	}
	out := map[string]any{"operator_key": op.OperatorKey, "rows": rows, "games": games}
	// Имя и ASN оператора — серверу для реестра sync_operators и канонизации
	// по ASN (те же данные, что шлёт data.operator; старые серверы поле
	// игнорируют).
	out["operator_name"] = op.OperatorName
	if info, err := database.GetOperatorInfo(op.OperatorKey); err == nil && info != nil && info.ASN != "" {
		out["operator_asn"] = info.ASN
	}
	return out
}

// ---- data.traffic ----

func (a *App) sectionTraffic() map[string]any {
	snaps, err := database.GetSnapshots(time.Now().Add(-24 * time.Hour))
	if err != nil || len(snaps) == 0 {
		return map[string]any{}
	}
	var peakDL, peakUL float64
	var totalDL, totalUL int64
	for _, s := range snaps {
		if s.DLSpeed > peakDL {
			peakDL = s.DLSpeed
		}
		if s.ULSpeed > peakUL {
			peakUL = s.ULSpeed
		}
		totalDL += s.TotalDL
		totalUL += s.TotalUL
	}
	// Скорости снапшотов — в байтах/с.
	return map[string]any{
		"peak_down_mbps": peakDL * 8 / 1e6,
		"peak_up_mbps":   peakUL * 8 / 1e6,
		"total_down_mb":  float64(totalDL) / (1 << 20),
		"total_up_mb":    float64(totalUL) / (1 << 20),
		"samples":        len(snaps),
	}
}

// ---- data.operator ----

func (a *App) sectionOperator() map[string]any {
	out := map[string]any{"known": []map[string]any{}}
	op, err := database.GetCurrentOperator()
	if err == nil && op != nil && op.OperatorKey != "" {
		m := map[string]any{"key": op.OperatorKey, "name": op.OperatorName}
		if info, err := database.GetOperatorInfo(op.OperatorKey); err == nil && info != nil {
			m["asn"] = info.ASN
			m["isp"] = info.ISP
			m["city"] = info.City
		}
		out["operator"] = m
	}
	known := []map[string]any{}
	if ops, err := database.GetAllOperatorInfo(); err == nil {
		for _, o := range ops {
			known = append(known, map[string]any{
				"key":        o.Key,
				"first_seen": o.FirstSeen.Format(time.RFC3339),
				"last_seen":  o.LastSeen.Format(time.RFC3339),
			})
		}
	}
	out["known"] = known
	return out
}

// ---- data.errors (дифф) ----

func syncErrorRows(cursor string) []map[string]any {
	since := sinceCursor(cursor)
	entries, err := logdb.QueryRecentErrors(200)
	if err != nil {
		return nil
	}
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		ts := normalizeLogTs(e.Timestamp)
		if t, err := time.Parse(time.RFC3339, ts); err == nil && !t.After(since) {
			continue
		}
		out = append(out, map[string]any{
			"ts":      ts,
			"level":   e.Level,
			"source":  e.Table,
			"message": e.Message,
		})
	}
	return out
}

func (a *App) sectionErrors() map[string]any {
	cursor := a.cursorValue("data.errors")
	return map[string]any{
		"cursor": cursor,
		"limit":  200,
		"rows":   syncErrorRows(cursor),
	}
}

// normalizeLogTs — время записи журнала к RFC3339 (logdb хранит строкой).
// Строки без зоны — локальное время машины (logdb.Insert пишет time.Now()
// без офсета): парсим в time.Local, иначе Go считает их UTC и на сервере
// время ошибок уезжает на величину смещения (для МСК +3 часа).
func normalizeLogTs(ts string) string {
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.Format(time.RFC3339)
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(layout, ts, time.Local); err == nil {
			return t.Format(time.RFC3339)
		}
	}
	return ts
}

// ---- data.events (дифф) ----

func syncEventRows(cursor string) []map[string]any {
	since := sinceCursor(cursor)
	logs, err := database.GetActionLogs("", 200, 0)
	if err != nil {
		return nil
	}
	out := make([]map[string]any, 0, len(logs))
	for _, l := range logs {
		if !l.Timestamp.After(since) {
			continue
		}
		out = append(out, map[string]any{
			"ts":       l.Timestamp.Format(time.RFC3339),
			"category": l.Category,
			"action":   l.Action,
			"details":  l.Details,
		})
	}
	return out
}

func (a *App) sectionEvents() map[string]any {
	cursor := a.cursorValue("data.events")
	rows := syncEventRows(cursor)
	auth := []map[string]any{}
	if logs, err := database.GetActionLogs("auth", 30, 0); err == nil {
		for _, l := range logs {
			auth = append(auth, map[string]any{
				"ts":      l.Timestamp.Format(time.RFC3339),
				"action":  l.Action,
				"account": l.Details,
			})
		}
	}
	return map[string]any{"cursor": cursor, "rows": rows, "auth": auth}
}

// ---- data.system ----

func (a *App) sectionSystem() map[string]any {
	sr := sysinfo.GetSystemResources()
	osName := a.osVersion() // чистое имя из реестра — то же, что в паспорте устройства
	osBuild := ""
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE); err == nil {
		cb, _, _ := k.GetStringValue("CurrentBuildNumber")
		ub, _, _ := k.GetIntegerValue("UBR")
		k.Close()
		if cb = strings.TrimSpace(cb); cb != "" {
			osBuild = cb + "." + strconv.FormatUint(ub, 10)
		}
	}
	cpu := "Unknown"
	if out, err := executil.HiddenCmd("reg", "query",
		`HKLM\HARDWARE\DESCRIPTION\System\CentralProcessor\0`,
		"/v", "ProcessorNameString").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if idx := strings.Index(line, "REG_SZ"); idx >= 0 {
				cpu = strings.TrimSpace(line[idx+6:])
				break
			}
		}
	}

	usage := []map[string]any{}
	if metrics, err := database.GetAppMetrics(time.Now().AddDate(0, 0, -14), 200); err == nil {
		for _, m := range metrics {
			usage = append(usage, map[string]any{
				"ts":           m.Timestamp.Format(time.RFC3339),
				"cpu_pct":      m.CPUPercent,
				"ws_mb":        m.WorkingSetMB,
				"threads":      m.Threads,
				"power_saving": m.PowerSaving,
			})
		}
	}
	return map[string]any{
		"os":            osName,
		"os_build":      osBuild,
		"cpu":           cpu,
		"cores":         sr.NumCores,
		"ram_total_mb":  int64(sr.SystemRAMMB + 0.5),
		"battery":       power.HasBattery(),
		"os_uptime_sec": osUptimeSec(),
		"usage":         downsample(usage, 200),
	}
}

// osUptimeSec — аптайм системы (GetTickCount64, мс → сек; хватает на 24 ч
// секцию: 49 суток до переполнения счётчика — сама Windows раньше ребутнется).
func osUptimeSec() int64 {
	return int64(windows.DurationSinceBoot() / time.Second)
}

// ---- data.versions ----

func (a *App) sectionVersions() map[string]any {
	comps := []map[string]any{}
	if cvs, err := database.GetAllComponentVersions(); err == nil {
		for _, cv := range cvs {
			if cv.ID == "report" || cv.ID == "zapret" {
				continue // report снесён (3A); zapret уходит верхнеуровневым полем
			}
			comps = append(comps, map[string]any{
				"id":        cv.ID,
				"installed": cv.InstalledVersion,
				"remote":    cv.RemoteVersion,
				"stale":     cv.RemoteVersion != "" && cv.InstalledVersion != "" && updater.IsNewer(cv.RemoteVersion, cv.InstalledVersion),
			})
		}
	}
	return map[string]any{
		"zpui":       a.version,
		"zapret":     a.zapret.GetVersion(),
		"arch":       runtime.GOARCH,
		"components": comps,
	}
}

// ---- data.config (анонимизированный снапшот настроек) ----

func (a *App) sectionConfig() map[string]any {
	settings := a.configSnapshotSettings()
	raw, _ := json.Marshal(settings)
	return map[string]any{
		"hash":     "sha256:" + sha256hex(string(raw)),
		"settings": settings,
	}
}

// configSnapshotSettings — анонимизированный состав настроек (03 §3):
// has_auth вместо учётных данных, адреса DNS — сетевая конфигурация, не ПДн.
func (a *App) configSnapshotSettings() map[string]any {
	bc := a.cfg.GetBlockCheckConfig()
	tg := a.cfg.GetTgProxyConfig()
	var xd config.XboxDnsConfig
	var theme, lang string
	var asEnabled bool
	var asThreshold, asInterval int
	var prEnabled bool
	var prPort int
	var autoStart, powerSaving bool
	a.cfg.RView(func() {
		xd = a.cfg.XboxDns
		theme = a.cfg.Theme
		asEnabled = a.cfg.AutoSwitchEnabled
		asThreshold = a.cfg.AutoSwitchThresholdPct
		asInterval = a.cfg.AutoSwitchIntervalMin
		prEnabled = a.cfg.Proxy.Enabled
		prPort = a.cfg.Proxy.Port
		autoStart = a.cfg.AutoStart
		powerSaving = a.cfg.PowerSaving
	})
	lang = a.cfg.GetLanguage()
	primary, secondary := "", ""
	if ips := xd.PlainIPs(); len(ips) > 0 {
		ips := withoutCustomDnsIPs(xd, ips)
		if len(ips) > 0 {
			primary = ips[0]
			if len(ips) > 1 {
				secondary = ips[1]
			}
		}
	}
	return map[string]any{
		"theme":    theme,
		"language": lang,
		"auto_switch": map[string]any{
			"enabled":   asEnabled,
			"threshold": asThreshold,
			"interval":  asInterval,
		},
		"proxy": map[string]any{
			"enabled":  prEnabled,
			"port":     prPort,
			"has_auth": false,
		},
		"dns": map[string]any{
			"enabled":   xd.Enabled,
			"primary":   primary,
			"secondary": secondary,
			// Флаги пресетов: CRM видит, что именно включено. Пользовательские
			// записи наружу не передаются (приватность) — только факт мастера
			// и адреса применённого резолвера.
			"xbox_enabled": xd.XboxEnabled,
			"providers":    enabledDnsProviderIDs(xd),
		},
		"tgproxy": map[string]any{
			"enabled":     tg.Enabled,
			"port":        tg.Port,
			"fake_tls":    tg.FakeTLSDomain != "",
			"fallback_cf": tg.FallbackCF,
		},
		"autostart":             autoStart,
		"power_saving":          powerSaving,
		"resource_interval_min": a.cfg.GetResourceCheckInterval(),
		"block_check": map[string]any{
			"tcp":  bc.CheckTCP,
			"tls":  bc.CheckTLS,
			"http": bc.CheckHTTP,
		},
	}
}

// enabledDnsProviderIDs — включённые базовые провайдеры (по системному
// списку dns-system.json).
func enabledDnsProviderIDs(x config.XboxDnsConfig) []string {
	out := []string{}
	for _, p := range config.Providers() {
		if x.Providers[p.ID] {
			out = append(out, p.ID)
		}
	}
	return out
}

// withoutCustomDnsIPs — адреса пользовательских записей наружу не идут
// (приватность): частные резолверы вроде NextDNS/AdGuard идентифицируют юзера.
func withoutCustomDnsIPs(x config.XboxDnsConfig, ips []string) []string {
	custom := make(map[string]bool, len(x.Custom)*2)
	for _, c := range x.Custom {
		custom[strings.ToLower(strings.TrimSpace(c.Primary))] = true
		custom[strings.ToLower(strings.TrimSpace(c.Secondary))] = true
	}
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		if custom[strings.ToLower(strings.TrimSpace(ip))] {
			continue
		}
		out = append(out, ip)
	}
	return out
}

// ---- data.exclusions ----

func (a *App) sectionExclusions() map[string]any {
	skipUser := ""
	if data, err := os.ReadFile(a.cfg.GetSkipResourcesFilePath()); err == nil {
		skipUser = string(data)
	}
	return map[string]any{
		"skip_user":        skipUser,
		"skip_sys_version": exclusionsLocalVersion(),
	}
}
