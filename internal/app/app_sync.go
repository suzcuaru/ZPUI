package app

// Канал ZPUI Sync — клиентская часть (волна 3A). Заменяет v5 state.sync
// (списки), v4-телеметрию/отчёты и device.register: один эндпоинт /api/sync,
// тик 60с ± джиттер, планировщик секций (internal/syncclient), талонная
// очередь, sync.pull со списками/recommended/requests.
// Спека: Документы/Задачи/ZPUI-Sync/{02,03,06}.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"zpui/internal/config"
	"zpui/internal/database"
	"zpui/internal/power"
	"zpui/internal/syncclient"
	"zpui/internal/updater"
)

// syncDataFlags — адаптер database.SetFlag/GetFlag под интерфейс раннера.
type syncDataFlags struct{}

func (syncDataFlags) Get(key string) (string, error) { return database.GetFlag(key) }
func (syncDataFlags) Set(key, value string) error    { return database.SetFlag(key, value) }

// startSyncDataLoop — фоновый контур канала (замена stateSyncLoop +
// StartServerTelemetry). Канал включён по умолчанию и работает в том числе
// без аккаунта (анонимно, по паспорту устройства — политика 11.2026);
// мастер-тумблер — config.sync_data.channel_enabled, серверная
// функциональность целиком — config.authorization_enabled.
func (a *App) startSyncDataLoop() {
	if !a.authorizationOn() {
		a.log.Info("sync", "authorization disabled: data channel is off")
		return
	}
	if !a.cfg.GetSyncChannelEnabled() {
		a.log.Info("sync", "data channel disabled by user")
		return
	}
	runner := &syncclient.Runner{
		Backend:        syncclient.New(ServerURL, 30*time.Second),
		Dev:            func() syncclient.Dev { return a.syncDev() },
		Token:          func() string { return a.cfg.GetServerConfig().AccountToken },
		Flags:          syncDataFlags{},
		Log:            a.log,
		Sources:        a,
		Overridden:     func() syncclient.Overrides { return syncOverridesFromConfig(a.cfg.GetSyncOverrides()) },
		Stretch:        power.Stretch,
		SkipTick:       power.ServicesPaused,
		RequestGate:    time.Hour,
		OnBanned:       a.syncTokenBanned,
		OnAuthRequired: a.syncSessionExpired,
		Journal: func(who, op, result string) {
			if err := database.InsertSyncHistory(who, op, result); err != nil {
				a.log.Warn("sync", "history: "+err.Error())
			}
		},
	}
	a.syncRunnerMu.Lock()
	a.syncRunner = runner
	a.syncRunnerMu.Unlock()
	a.supervisedGo("sync-data", func() { runner.Run(a.stopCh) })
}

// syncTokenBanned — токен отозван сервером: чистим локальную сессию
// (приложение возвращается в неавторизованное состояние).
func (a *App) syncTokenBanned() {
	_ = a.cfg.UpdateServerConfig(func(s *config.ServerConfig) { s.AccountToken = "" })
	a.log.Warn("sync", "token banned by server — local session cleared")
}

// syncSessionExpired — сессия на сервере не найдена (истекла или потеряна,
// например после восстановления БД из бэкапа): гасим токен, предлагаем вход.
func (a *App) syncSessionExpired() {
	had := a.cfg.GetServerConfig().AccountToken != ""
	if !had {
		return
	}
	_ = a.cfg.UpdateServerConfig(func(s *config.ServerConfig) {
		s.AccountToken = ""
		s.Profile = nil
	})
	a.log.Warn("sync", "session expired on server — re-login required")
}

// syncOverridesFromConfig — оверрайды из конфига в формат раннера.
func syncOverridesFromConfig(in map[string]config.SyncOverrideCfg) syncclient.Overrides {
	if len(in) == 0 {
		return nil
	}
	out := make(syncclient.Overrides, len(in))
	for k, v := range in {
		out[k] = syncclient.Override{Mult: v.Mult, Off: v.Off}
	}
	return out
}

// ---------- sync.pull: применение ----------

type syncRecommendedUpdate struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
}

// ApplyPull — ответ sync.pull: системные исключения, DNS-провайдеры,
// recommended (zpui/zapret), просьбы сервера. Логика записи файлов и
// флагов — переезд stateSyncOnce/dnsSyncOnce (app_v5.go).
func (a *App) ApplyPull(data json.RawMessage) {
	if len(data) == 0 {
		return
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
		Autoselect *struct {
			Version   int64           `json:"version"`
			Unchanged bool            `json:"unchanged"`
			Data      json.RawMessage `json:"data"`
		} `json:"autoselect"`
		Recommended map[string]syncRecommendedUpdate `json:"recommended"`
		Requests    struct {
			Now   []string `json:"now"`
			Skips bool     `json:"skips"`
		} `json:"requests"`
	}
	if err := json.Unmarshal(data, &pd); err != nil {
		return
	}

	now := time.Now().Format(time.RFC3339)
	changed := false

	// Системный скип-лист: при смене версии целиком заменяется файл.
	if pd.Exclusions != nil && pd.Exclusions.Version > 0 &&
		pd.Exclusions.Version != exclusionsLocalVersion() && !pd.Exclusions.Unchanged {
		if err := a.writeSystemExclusions(pd.Exclusions.Items); err != nil {
			a.log.Warn("exclusions", "write system list: "+err.Error())
		} else {
			_ = database.SetFlag("exclusions_version", strconv.FormatInt(pd.Exclusions.Version, 10))
			a.log.Info("exclusions", fmt.Sprintf("system exclusion list replaced from server (v%d): %d domains", pd.Exclusions.Version, len(pd.Exclusions.Items)))
			changed = true
		}
	}

	// Системный список DNS: файл dns-system.json заменяется ЦЕЛИКОМ
	// (удаление провайдера на сервере доезжает до клиента).
	if pd.DNSProviders != nil && pd.DNSProviders.Version > 0 && !pd.DNSProviders.Unchanged &&
		pd.DNSProviders.Version != dnsSystemLocalVersion() {
		entries := make([]config.DnsProvider, 0, len(pd.DNSProviders.Items))
		known := make(map[string]bool, len(pd.DNSProviders.Items))
		for _, p := range pd.DNSProviders.Items {
			// Контракт сервера: выключенный/заблокированный провайдер остаётся
			// в списке с blocked=true (виден, включить нельзя), удалённый —
			// отсутствует. Флаги заблокированных снимаются (known — без них).
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
				_ = database.SetFlag("dns_system_version", fmt.Sprintf("%d", pd.DNSProviders.Version))
				a.log.Info("dns", fmt.Sprintf("system DNS list replaced from server (v%d): %d providers", pd.DNSProviders.Version, len(entries)))
				a.dnsReconfigure()
				a.disableDnsOrphans(known)
			}
		}
	}

	// Алгоритм подбора стратегий: CRM ведёт набор (вкладка «Подбор»),
	// дельта по версии — как скип-лист/DNS.
	if pd.Autoselect != nil {
		a.applyAutoselectBlock(pd.Autoselect.Version, pd.Autoselect.Unchanged, pd.Autoselect.Data)
	}

	// Успешная сверка списков — фиксируем время для UI.
	_ = database.SetFlag("serverlists_sync_at", now)
	_ = database.SetFlag("exclusions_sync_at", now)
	_ = database.SetFlag("dns_providers_synced_at", now)
	_ = database.SetFlag("serverlists_last_error", "")
	_ = database.SetFlag("sync_pull_at", now)

	// Recommended: контроль-плейн обновлений. ZPUI остаётся на v1-проверке
	// (инвариант обновляемости), recommended лишь сохраняется для UI.
	// Zapret — поверх существующей логики: если сервер рекомендует версию
	// свежее установленной, выставляем флаги обновления (источник server).
	for key, rec := range pd.Recommended {
		if rec.Version == "" || rec.URL == "" {
			continue
		}
		rec.URL = a.absServerURL(rec.URL)
		raw, _ := json.Marshal(rec)
		_ = database.SetFlag("sync_recommended_"+key, string(raw))
		if key == "zapret" && !a.cfg.GetZapretSkipped() {
			if updater.IsNewer(a.zapret.GetVersion(), rec.Version) {
				a.setZapretUpdateAvailable(a.zapret.GetVersion(), rec.Version, "server")
				a.log.Info("sync", "recommended zapret update: "+rec.Version)
			}
		}
	}

	// Просьбы сервера — в журнал для UI (исполняет раннер с часовым гейтом).
	if len(pd.Requests.Now) > 0 || pd.Requests.Skips {
		if r := a.syncRunnerRef(); r != nil {
			r.AddRequestsLog(map[string]any{
				"at":       now,
				"sections": pd.Requests.Now,
				"skips":    pd.Requests.Skips,
			})
		}
		a.log.Info("sync", fmt.Sprintf("server requested sections: %v (skips=%v)", pd.Requests.Now, pd.Requests.Skips))
	}

	if changed {
		a.emitStrategiesChanged()
	}
}

// absServerURL — относительный путь сервера в абсолютный URL.
func (a *App) absServerURL(u string) string {
	if u == "" || (len(u) > 8 && (u[:8] == "https://" || (len(u) > 7 && u[:7] == "http://"))) {
		return u
	}
	return ServerURL + u
}

// syncRunnerRef — текущий раннер канала (nil, если контур не запущен).
func (a *App) syncRunnerRef() *syncclient.Runner {
	a.syncRunnerMu.Lock()
	defer a.syncRunnerMu.Unlock()
	return a.syncRunner
}

// ---------- LastRowTs: фолбэк курсора диффов ----------

// LastRowTs — метка последней строки диффа (когда сервер не вернул ack):
// курсор двигаем к последней отправленной строке, чтобы не зациклиться.
func (a *App) LastRowTs(op string) string {
	switch op {
	case "data.availability":
		rows := syncAvailabilityRows(a.cursorValue(op))
		if n := len(rows); n > 0 {
			ts, _ := rows[n-1]["ts"].(string)
			return ts
		}
	case "data.resources":
		rows, err := database.GetResourceDailySince(func() string {
			c := a.cursorValue(op)
			if len(c) == 10 {
				return c
			}
			return ""
		}())
		if err == nil && len(rows) > 0 {
			return rows[len(rows)-1].Date
		}
	case "data.errors":
		rows := syncErrorRows(a.cursorValue(op))
		if n := len(rows); n > 0 {
			ts, _ := rows[n-1]["ts"].(string)
			return ts
		}
	case "data.events":
		rows := syncEventRows(a.cursorValue(op))
		if n := len(rows); n > 0 {
			ts, _ := rows[n-1]["ts"].(string)
			return ts
		}
	}
	return ""
}

func (a *App) cursorValue(op string) string {
	if r := a.syncRunnerRef(); r != nil {
		return r.CursorOf(op)
	}
	return ""
}

// ---------- биндинги (волна 3B, web/) ----------

// GetSyncInfo — статус канала ZPUI Sync: онлайн, последний тик, RTT,
// каталог секций с интервалами и оверрайдами, журнал просьб сервера.
func (a *App) GetSyncInfo() map[string]interface{} {
	sections := []map[string]interface{}{}
	ov := syncclient.Overrides(ovAsOverrides(a.cfg.GetSyncOverrides()))
	for _, s := range syncclient.Catalog() {
		iv := ov.Effective(s)
		entry := map[string]interface{}{
			"op":        s.Op,
			"interval":  int(iv / time.Second),
			"on_change": s.OnChange,
			"diff":      s.Diff,
			"off":       ov.Disabled(s.Op),
		}
		if last, err := database.GetFlag("sync_last_" + syncclient.SectionKey(s.Op)); err == nil && last != "" {
			entry["last_ok"] = last
		}
		sections = append(sections, entry)
	}
	out := map[string]interface{}{
		"enabled":     a.authorizationOn() && a.cfg.GetSyncChannelEnabled(),
		"server":      ServerURL,
		"sections":    sections,
		"recommended": map[string]interface{}{},
	}
	if r := a.syncRunnerRef(); r != nil {
		st := r.Status()
		out["online"] = st.Online
		out["last_tick"] = st.LastTick.Format(time.RFC3339)
		out["rtt_ms"] = st.RTTms
		out["last_error"] = st.LastError
		out["next_after"] = st.NextAfter
		if st.TicketOp != "" {
			out["ticket_op"] = st.TicketOp
		}
		out["requests_log"] = st.RequestsLog
		rec := map[string]interface{}{}
		for _, k := range []string{"zpui", "zapret"} {
			if raw, err := database.GetFlag("sync_recommended_" + k); err == nil && raw != "" {
				var m map[string]interface{}
				if json.Unmarshal([]byte(raw), &m) == nil {
					rec[k] = m
				}
			}
		}
		out["recommended"] = rec
	} else {
		out["online"] = false
		out["requests_log"] = []map[string]any{}
	}
	return okRespWithData(out)
}

// GetSyncHistory — история запросов канала: period = today|7d|30d
// (старше 30 дней записи не хранятся), свежие сверху.
func (a *App) GetSyncHistory(period string) map[string]interface{} {
	now := time.Now()
	var since time.Time
	switch period {
	case "7d":
		since = now.Add(-7 * 24 * time.Hour)
	case "30d":
		since = now.Add(-30 * 24 * time.Hour)
	default:
		since = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	}
	rows, err := database.GetSyncHistory(since, 2000)
	if err != nil {
		return errResp(err.Error())
	}
	out := make([]map[string]interface{}, 0, len(rows))
	for _, e := range rows {
		out = append(out, map[string]interface{}{
			"ts":     e.TS.Format(time.RFC3339),
			"who":    e.Who,
			"op":     e.Op,
			"result": e.Result,
		})
	}
	return okRespWithData(map[string]interface{}{"rows": out})
}

func ovAsOverrides(in map[string]config.SyncOverrideCfg) map[string]syncclient.Override {
	out := make(map[string]syncclient.Override, len(in))
	for k, v := range in {
		out[k] = syncclient.Override{Mult: v.Mult, Off: v.Off}
	}
	return out
}

// SetSyncOverrides — мастер-тумблер канала и оверрайды секций
// {channel_enabled: bool, overrides: {status: {mult:2, off:false}, ...}}.
func (a *App) SetSyncOverrides(body map[string]interface{}) map[string]interface{} {
	if body == nil {
		return errResp("bad_request")
	}
	enabled, _ := body["channel_enabled"].(bool)
	overrides := map[string]config.SyncOverrideCfg{}
	if raw, ok := body["overrides"].(map[string]interface{}); ok {
		for name, v := range raw {
			m, _ := v.(map[string]interface{})
			if m == nil {
				continue
			}
			entry := config.SyncOverrideCfg{}
			if f, ok := m["mult"].(float64); ok {
				entry.Mult = int(f)
			}
			if b, ok := m["off"].(bool); ok {
				entry.Off = b
			}
			if entry.Mult != 0 || entry.Off {
				overrides[name] = entry
			}
		}
	}
	if err := a.cfg.SetSyncOverrides(enabled, overrides); err != nil {
		return errResp("save: " + err.Error())
	}
	a.log.Info("sync", fmt.Sprintf("overrides saved: enabled=%v sections=%d", enabled, len(overrides)))
	return okResp()
}

// SaveSyncSnapshot — локальный экспорт всех секций каталога в JSON-файл
// (замена отчёту для ручной пересылки в поддержку).
func (a *App) SaveSyncSnapshot() map[string]interface{} {
	snap := map[string]interface{}{
		"generated": time.Now().Format(time.RFC3339),
		"zpui":      a.version,
		"sections":  map[string]interface{}{},
	}
	secs := snap["sections"].(map[string]interface{})
	for _, s := range syncclient.Catalog() {
		payload, err := a.Collect(s.Op)
		if err != nil {
			secs[s.Op] = map[string]interface{}{"error": err.Error()}
			continue
		}
		secs[s.Op] = payload
	}
	content, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return errResp("snapshot: " + err.Error())
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return errResp(err.Error())
	}
	dir := filepath.Join(home, "Downloads")
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "ZPUI_Snapshot_"+time.Now().Format("2006-01-02_150405")+".json")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return errResp("save: " + err.Error())
	}
	a.log.Info("sync", "snapshot saved: "+path)
	return okRespWithData(map[string]interface{}{"path": path})
}
