package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/config"
	"zpui/internal/database"
	"zpui/internal/executil"
	"zpui/internal/notify"
	"zpui/internal/xboxdns"
)

func xboxdnsSystemDNS() []xboxdns.AdapterDNS {
	return xboxdns.SystemDNS()
}

// ============================================================
// DNS (модель провайдеров: базовые + пользовательские записи,
// применяется plain; DoH-пары регистрируются в Windows best-effort)
// ============================================================

// GetXboxDnsConfig — текущая конфигурация DNS (модель config.xbox_dns).
func (a *App) GetXboxDnsConfig() interface{} {
	return a.cfg.GetXboxDnsConfig()
}

// getDnsConfigData — тело ответа конфигурации DNS для UI: состояние
// master, провайдеры с адресами, пользовательские записи, health.
func (a *App) getDnsConfigData() map[string]interface{} {
	cfg := a.cfg.GetXboxDnsConfig()
	provs := make([]map[string]interface{}, 0, len(config.Providers()))
	for _, p := range config.Providers() {
		provs = append(provs, map[string]interface{}{
			"id":      p.ID,
			"name":    p.Name,
			"enabled": cfg.Providers[p.ID],
			"ipv4":    p.IPv4,
			"doh":     p.DoH,
			"blocked": p.Blocked,
			"reason":  p.Reason,
		})
	}
	custom := make([]map[string]interface{}, 0, len(cfg.Custom))
	for _, c := range cfg.Custom {
		custom = append(custom, map[string]interface{}{
			"id":        c.ID,
			"name":      c.Name,
			"primary":   c.Primary,
			"secondary": c.Secondary,
			"doh":       c.DoH,
			"enabled":   c.Enabled,
		})
	}
	resp := okRespWithData(map[string]interface{}{
		"enabled":          cfg.Enabled,
		"xbox_enabled":     cfg.XboxEnabled,
		"providers":        provs,
		"custom":           custom,
		"servers":          cfg.PlainIPs(),
		"applying":         a.dnsApplying.Load(),
		"applying_scope":   a.dnsApplyScopeNow(),
		"locked_custom":    dnsAnyProviderOn(cfg),
		"locked_providers": dnsAnyCustomOn(cfg),
	})
	if h := a.xboxDns.CachedHealth(); h != nil {
		resp["health"] = map[string]interface{}{
			"servers":    h.Servers,
			"ok":         h.OK,
			"checked_at": h.CheckedAt.Format(time.RFC3339),
		}
	}
	return resp
}

// GetDnsConfig — конфигурация DNS для страницы (формат okResp).
func (a *App) GetDnsConfig() map[string]interface{} {
	return a.getDnsConfigData()
}

// кеш последнего серверного списка снесён: список провайдеров живёт в
// локальной таблице config.Providers() (её заменяет канал Sync целиком).

// GetDnsRemoteProviders — метаданные системного списка DNS-провайдеров для
// вкладки: версия и время последнего синка (флаги канала Sync). Сам список
// приносит канал ZPUI Sync — ApplyPull заменяет dns-system.json целиком и
// гасит провайдеров-сирот, поэтому сетевого хода здесь больше нет: открытие
// вкладки не ходит на сервер и не переприменяет системный DNS.
func (a *App) GetDnsRemoteProviders() map[string]interface{} {
	data := a.getDnsConfigData()
	if v, err := database.GetFlag("dns_system_version"); err == nil && v != "" {
		if n, perr := strconv.ParseInt(v, 10, 64); perr == nil {
			data["version"] = n
		}
	}
	if v, err := database.GetFlag("dns_providers_synced_at"); err == nil && v != "" {
		data["synced_at"] = v
	}
	return data
}

// disableDnsOrphans — включённые флаги провайдеров, которых нет в серверном
// списке (known), снимаются, их адреса уходят из системы переприменением
// остатка источников — как при обычном выключении тумблера. true = были сироты.
func (a *App) disableDnsOrphans(known map[string]bool) bool {
	// Пустой серверный список — признак битого/неполного ответа, а не
	// основание выключать все источники.
	if len(known) == 0 {
		return false
	}
	cfg := a.cfg.GetXboxDnsConfig()
	prev := cfg
	removed := false
	for id := range cfg.Providers {
		if !known[id] {
			delete(cfg.Providers, id)
			removed = true
		}
	}
	if !removed {
		return false
	}
	a.syncDnsDoH(prev, cfg)
	if err := a.applyDnsSources(cfg, prev); err != nil {
		a.log.Warn("dns", "orphan providers disable: "+err.Error())
	}
	return true
}

// SetDnsConfig — легаси-маршрут (мастер/старые клиенты): патчит
// enabled/xbox_enabled/servers, провайдеры и записи сохраняет как есть.
// Ключ ipv6 из старых клиентов игнорируется — IPv6-DNS больше не поддерживается.
// Патч xbox_enabled (вкладка DNS) переприменяет ИСТОЧНИКИ — иначе включение
// пресета применяло легаси-список servers вместо актуального набора и «ломало»
// вкладку. Явный servers[] (старые клиенты) — прежнее поведение.
func (a *App) SetDnsConfig(opts map[string]interface{}) map[string]interface{} {
	cfg := a.cfg.GetXboxDnsConfig()
	prev := cfg
	wasEnabled := cfg.Enabled
	serversPatched := false

	if v, ok := opts["enabled"].(bool); ok {
		cfg.Enabled = v
	}
	// xbox_enabled больше не патчится: пресет Xbox DNS выведен из базовых
	// провайдеров и глушится при старте (см. config.Load).
	if raw, ok := opts["servers"].([]interface{}); ok {
		serversPatched = true
		servers := make([]config.DnsServer, 0, len(raw))
		for _, item := range raw {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			addr, _ := m["addr"].(string)
			typ, _ := m["type"].(string)
			enabled, _ := m["enabled"].(bool)
			addr = strings.TrimSpace(addr)
			if addr == "" {
				continue
			}
			if !config.ValidServerType(typ) {
				return errResp("invalid_type: " + typ)
			}
			if !config.ValidServerAddr(typ, addr) {
				return errResp("invalid_server: " + addr)
			}
			servers = append(servers, config.DnsServer{Addr: addr, Type: typ, Enabled: enabled})
		}
		cfg.Servers = servers
	}

	if serversPatched {
		if err := a.cfg.SetXboxDnsConfig(cfg); err != nil {
			return errResp(err.Error())
		}
		a.log.Info("dns", "DNS config saved (legacy servers)")

		if err := a.applyDnsEnabled(cfg.Enabled); err != nil {
			if cfg.Enabled && !wasEnabled {
				if err2 := a.cfg.SetXboxDnsEnabled(false); err2 != nil {
					a.log.Warn("dns", "rollback SetXboxDnsEnabled: "+err2.Error())
				}
			}
			return errResp(err.Error())
		}
		return okResp()
	}

	a.syncDnsDoH(prev, cfg)
	if err := a.applyDnsSources(cfg, prev); err != nil {
		if dnsUnreachableErr(err) {
			return errResp("dns_unreachable")
		}
		return errResp(err.Error())
	}
	a.ensureDnsStateHonest()
	return a.getDnsConfigData()
}

// SetXboxDnsConfig — совместимость со старым маршрутом: делегирует SetDnsConfig
// (легаси-поля primary_dns и пр. игнорируются).
func (a *App) SetXboxDnsConfig(opts map[string]interface{}) map[string]interface{} {
	return a.SetDnsConfig(opts)
}

// GetCurrentDns — системные DNS адаптеров и факт применения нашей
// конфигурации на первичном адаптере (источник истины для состояния UI).
// static_ip — ФАКТ конфигурации адаптера (EnableDHCP из реестра): страница
// DNS держит единственный тумблер статического IP (из прокси убран).
func (a *App) GetCurrentDns() map[string]interface{} {
	cfg := a.dnsReconfigure()
	adapters := xboxdns.SystemDNS()
	v4, v6 := xboxdns.SystemDNSFlat()
	active := a.xboxDns.IsSetInSystem()
	staticIP := false
	ip, subnet := "", ""
	if name := xboxdns.PrimaryAdapterName(); name != "" {
		staticIP = xboxdns.AdapterHasStaticIPv4(name)
		if v, mask, _, err := currentIPv4Config(name); err == nil {
			ip = v
			subnet = subnetKey(v, mask)
		}
	}
	return okRespWithData(map[string]interface{}{
		"adapters":  adapters,
		"ipv4":      v4,
		"ipv6":      v6,
		"active":    active,
		"enabled":   cfg.Enabled,
		"static_ip": staticIP,
		"ip":        ip,
		"subnet":    subnet,
		"adapter":   xboxdns.PrimaryAdapterName(),
	})
}

// ============================================================
// Тумблеры модели провайдеров
// ============================================================

// dnsApplyBegin — захват «слота» применения DNS: одно применение за раз.
// false — применение уже идёт (фронт отвечает dns_busy).
func (a *App) dnsApplyBegin(scope string) bool {
	if !a.dnsApplying.CompareAndSwap(false, true) {
		return false
	}
	a.dnsApplyScope.Store(scope)
	return true
}

func (a *App) dnsApplyScopeNow() string {
	s, _ := a.dnsApplyScope.Load().(string)
	return s
}

// dnsAnyProviderOn / dnsAnyCustomOn — признак «семейство источников активно»;
// семейства взаимоисключающие, активное лочит тумблеры другого (UI-флаги locked_*).
func dnsAnyProviderOn(cfg config.XboxDnsConfig) bool {
	for _, on := range cfg.Providers {
		if on {
			return true
		}
	}
	return false
}

func dnsAnyCustomOn(cfg config.XboxDnsConfig) bool {
	for i := range cfg.Custom {
		if cfg.Custom[i].Enabled {
			return true
		}
	}
	return false
}

// enabledProviders — только включённые ключи карты провайдеров (для снимка
// «что было включено до статики»).
func enabledProviders(m map[string]bool) map[string]bool {
	out := map[string]bool{}
	for id, on := range m {
		if on {
			out[id] = true
		}
	}
	return out
}

// dnsSourceLocked — включение источника отклоняется, пока идёт применение
// противоположного семейства (фронт мог прислать тумблеры одновременно):
// конфиг не трогаем, фронт пересинхронизируется по dns:apply.
func (a *App) dnsSourceLocked(family string) bool {
	if !a.dnsApplying.Load() {
		return false
	}
	scope := a.dnsApplyScopeNow()
	switch family {
	case "provider":
		return strings.HasPrefix(scope, "custom:") || scope == "auto_off"
	case "custom":
		return strings.HasPrefix(scope, "provider:") || scope == "auto_off"
	}
	return false
}

// dnsApplyRun — фоновая работа применения: сериализуется dnsApplyMu, итог —
// событие dns:apply {phase: done|failed, scope, error?} и сброс слота.
func (a *App) dnsApplyRun(scope string, work func() error) {
	a.dnsApplyMu.Lock()
	errText := ""
	if err := work(); err != nil {
		errText = err.Error()
		a.log.Warn("dns", "apply "+scope+": "+errText)
	}
	a.dnsApplyMu.Unlock()
	a.dnsApplying.Store(false)
	fields := map[string]interface{}{"phase": "done", "scope": scope}
	if errText != "" {
		fields["phase"] = "failed"
		fields["error"] = errText
	}
	runtime.EventsEmit(a.getContext(), "dns:apply", fields)
}

// dnsUnreachableErr — включение отклонено pre-flight'ом: сами серверы
// недоступны (фронт показывает отдельный тост).
func dnsUnreachableErr(err error) bool {
	return strings.HasPrefix(err.Error(), "DNS-серверы недоступны")
}

// customDohMap — пары «IP → DoH-шаблон» включённых записей.
func customDohMap(cfg config.XboxDnsConfig) map[string]string {
	out := map[string]string{}
	for _, c := range cfg.Custom {
		if !c.Enabled || c.DoH == "" {
			continue
		}
		for _, a := range []string{c.Primary, c.Secondary} {
			if a = strings.TrimSpace(a); config.ValidDnsIP(a) {
				out[strings.ToLower(a)] = c.DoH
			}
		}
	}
	return out
}

// providerDohMap — пары «IP → DoH-шаблон» включённых базовых провайдеров.
func providerDohMap(cfg config.XboxDnsConfig) map[string]string {
	out := map[string]string{}
	for _, p := range config.Providers() {
		if !cfg.Providers[p.ID] || p.DoH == "" {
			continue
		}
		for _, a := range p.IPv4 {
			if config.ValidDnsIP(a) {
				out[strings.ToLower(a)] = p.DoH
			}
		}
	}
	return out
}

// dnsDohMap — все пары «IP → DoH» конфигурации: провайдеры + записи.
func dnsDohMap(cfg config.XboxDnsConfig) map[string]string {
	out := providerDohMap(cfg)
	for ip, tpl := range customDohMap(cfg) {
		out[ip] = tpl
	}
	return out
}

// syncDnsDoH приводит DoH-регистрации Windows (netsh dns encryption) к
// текущему набору (провайдеры + записи): новые/изменённые — добавляются,
// исчезнувшие — снимаются. Ошибки не критичны (система без поддержки DoH).
func (a *App) syncDnsDoH(prev, next config.XboxDnsConfig) {
	oldM, newM := dnsDohMap(prev), dnsDohMap(next)
	for ip, tpl := range oldM {
		if newM[ip] == tpl {
			continue
		}
		xboxdns.DeleteDoHServer(ip)
	}
	for ip, tpl := range newM {
		if oldM[ip] == tpl {
			continue
		}
		if err := xboxdns.SetDoHServer(ip, tpl); err != nil {
			a.log.Warn("dns", "netsh doh add "+ip+": "+err.Error())
		}
	}
}

// applyDnsSources переприменяет системный DNS по флагам источников cfg.
// Источники есть — pre-flight и прописывание (master=true); источников нет —
// восстановление сохранённых/DHCP (master=false). Ошибка включения
// откатывает конфиг на prev.
func (a *App) applyDnsSources(cfg, prev config.XboxDnsConfig) error {
	if len(cfg.EnabledSourceIDs()) == 0 {
		cfg.Enabled = false
		if err := a.cfg.SetXboxDnsConfig(cfg); err != nil {
			return err
		}
		if err := a.applyDnsEnabled(false); err != nil {
			a.log.Warn("dns", "restore: "+err.Error())
		}
		return nil
	}
	cfg.Enabled = true
	if err := a.cfg.SetXboxDnsConfig(cfg); err != nil {
		return err
	}
	if err := a.applyDnsEnabled(true); err != nil {
		if err2 := a.cfg.SetXboxDnsConfig(prev); err2 != nil {
			a.log.Warn("dns", "rollback sources: "+err2.Error())
		}
		return err
	}
	return nil
}

// ToggleDnsProvider — тумблер базового провайдера (оба адреса сразу).
// Валиден id из локальной таблицы или из последнего серверного списка.
// Источники «провайдеры» и «статика» взаимоисключающие: включение провайдера
// снимает включённые пользовательские записи (список записей сохраняется).
// ensureDnsStateHonest — мастер-тумблер DNS не должен врать: если в конфиге
// включён, а в системе DNS нет (пустой снапшот, упавшее восстановление) —
// один ретрай применения и, при неудаче, честное выключение флага.
func (a *App) ensureDnsStateHonest() {
	cfgC := a.cfg.GetXboxDnsConfig()
	if !cfgC.Enabled || a.xboxDns.IsSetInSystem() {
		return
	}
	a.log.Warn("dns", "master enabled but DNS missing in system — retry apply")
	time.Sleep(3 * time.Second)
	again := a.cfg.GetXboxDnsConfig()
	if err := a.applyDnsSources(again, again); err == nil && a.xboxDns.IsSetInSystem() {
		a.log.Info("dns", "retry applied — DNS state ok")
		return
	}
	if err := a.cfg.SetXboxDnsEnabled(false); err != nil {
		a.log.Warn("dns", "honest disable: "+err.Error())
	}
	if ctx := a.getContext(); ctx != nil {
		runtime.EventsEmit(ctx, "xboxdns:auto_disabled", map[string]any{"reason": a.trCur("dns_not_applied")})
	}
	a.log.Warn("dns", "DNS could not be applied — master flag turned off")
}

func (a *App) ToggleDnsProvider(id string, on bool) map[string]interface{} {
	known := false
	for _, p := range config.Providers() {
		if p.ID == id {
			known = true
			// провайдер отключён сервером (стабильно не отвечает): включать
			// нельзя — UI показывает подсказку с причиной
			if on && p.Blocked {
				return errResp("provider_blocked: " + p.Reason)
			}
			break
		}
	}
	if !known {
		return errResp("unknown_provider: " + id)
	}
	cfg := a.cfg.GetXboxDnsConfig()
	prev := cfg
	if cfg.Providers == nil {
		cfg.Providers = map[string]bool{}
	}
	if on {
		if a.dnsSourceLocked("provider") {
			return errResp("source_locked")
		}
		if dnsAnyCustomOn(cfg) {
			for i := range cfg.Custom {
				cfg.Custom[i].Enabled = false
			}
			cfg.ProvidersBeforeCustom = nil
			if err := a.cfg.SetDnsProvidersBeforeCustom(nil); err != nil {
				a.log.Warn("dns", "providers snapshot reset: "+err.Error())
			}
			a.log.Info("dns", "provider "+id+" enabled — custom DNS entries disabled (sources are exclusive)")
		}
		cfg.Providers[id] = true
	} else {
		delete(cfg.Providers, id)
	}
	scope := "provider:" + id
	a.dnsApplyBegin(scope)
	go a.dnsApplyRun(scope, func() error {
		a.syncDnsDoH(prev, cfg)
		if err := a.applyDnsSources(cfg, prev); err != nil {
			if dnsUnreachableErr(err) {
				return fmt.Errorf("dns_unreachable")
			}
			return err
		}
		a.ensureDnsStateHonest()
		return nil
	})
	return okRespWithData(map[string]interface{}{"applying": true, "scope": scope})
}

// ToggleDnsCustom — тумблер пользовательской записи. Источники «статика» и
// «провайдеры» взаимоисключающие: включение записи снимает всех базовых
// провайдеров.
func (a *App) ToggleDnsCustom(id string, on bool) map[string]interface{} {
	cfg := a.cfg.GetXboxDnsConfig()
	prev := cfg
	hit := false
	for i := range cfg.Custom {
		if cfg.Custom[i].ID == id {
			cfg.Custom[i].Enabled = on
			hit = true
			break
		}
	}
	if !hit {
		return errResp("unknown_custom: " + id)
	}
	if on {
		if a.dnsSourceLocked("custom") {
			return errResp("source_locked")
		}
		if dnsAnyProviderOn(cfg) {
			snap := enabledProviders(cfg.Providers)
			cfg.ProvidersBeforeCustom = snap
			if err := a.cfg.SetDnsProvidersBeforeCustom(snap); err != nil {
				a.log.Warn("dns", "providers snapshot: "+err.Error())
			}
			cfg.Providers = map[string]bool{}
			a.log.Info("dns", "custom "+id+" enabled — DNS providers disabled (sources are exclusive)")
		}
	} else if !dnsAnyCustomOn(cfg) {
		snap := a.cfg.GetDnsProvidersBeforeCustom()
		cfg.ProvidersBeforeCustom = nil
		if len(snap) > 0 {
			cfg.Providers = snap
			a.log.Info("dns", "custom "+id+" disabled — DNS providers restored from snapshot")
		}
		if err := a.cfg.SetDnsProvidersBeforeCustom(nil); err != nil {
			a.log.Warn("dns", "providers snapshot reset: "+err.Error())
		}
	}
	scope := "custom:" + id
	a.dnsApplyBegin(scope)
	go a.dnsApplyRun(scope, func() error {
		a.syncDnsDoH(prev, cfg)
		if err := a.applyDnsSources(cfg, prev); err != nil {
			if dnsUnreachableErr(err) {
				return fmt.Errorf("dns_unreachable")
			}
			return err
		}
		a.ensureDnsStateHonest()
		return nil
	})
	return okRespWithData(map[string]interface{}{"applying": true, "scope": scope})
}

// SaveDnsCustom — создание/изменение пользовательской записи.
// id="" — новая запись (enable обычно false). Включённая запись или
// изменение включённой переприменяют системный DNS. Системные записи
// (sys:*) редактированию не подлежат — только тумблер.
func (a *App) SaveDnsCustom(id, name, primary, secondary, doh string, enable bool) map[string]interface{} {
	if strings.HasPrefix(id, "sys:") {
		return errResp("system_entry")
	}
	name = strings.TrimSpace(name)
	primary = strings.TrimSpace(primary)
	secondary = strings.TrimSpace(secondary)
	doh = strings.TrimSpace(doh)
	if name == "" {
		return errResp("empty_name")
	}
	if !config.ValidDnsIP(primary) {
		return errResp("invalid_primary")
	}
	if secondary != "" && !config.ValidDnsIP(secondary) {
		return errResp("invalid_secondary")
	}
	if doh != "" && !config.ValidDohTemplate(doh) {
		return errResp("invalid_doh")
	}

	cfg := a.cfg.GetXboxDnsConfig()
	prev := cfg
	wasEnabled := false
	if id == "" {
		entry := config.CustomDns{
			ID:        config.NewCustomDnsID(),
			Name:      name,
			Primary:   primary,
			Secondary: secondary,
			DoH:       doh,
			Enabled:   enable,
		}
		cfg.Custom = append(cfg.Custom, entry)
	} else {
		hit := false
		for i := range cfg.Custom {
			if cfg.Custom[i].ID == id {
				wasEnabled = cfg.Custom[i].Enabled
				cfg.Custom[i].Name = name
				cfg.Custom[i].Primary = primary
				cfg.Custom[i].Secondary = secondary
				cfg.Custom[i].DoH = doh
				cfg.Custom[i].Enabled = enable
				hit = true
				break
			}
		}
		if !hit {
			return errResp("unknown_custom: " + id)
		}
	}
	if !wasEnabled && !enable {
		if err := a.cfg.SetXboxDnsConfig(cfg); err != nil {
			return errResp(err.Error())
		}
		return a.getDnsConfigData()
	}
	if enable {
		if a.dnsSourceLocked("custom") {
			return errResp("source_locked")
		}
		if dnsAnyProviderOn(cfg) {
			snap := enabledProviders(cfg.Providers)
			cfg.ProvidersBeforeCustom = snap
			if err := a.cfg.SetDnsProvidersBeforeCustom(snap); err != nil {
				a.log.Warn("dns", "providers snapshot: "+err.Error())
			}
			cfg.Providers = map[string]bool{}
			a.log.Info("dns", "custom entry saved enabled — DNS providers disabled (sources are exclusive)")
		}
	}
	a.syncDnsDoH(prev, cfg)
	if err := a.applyDnsSources(cfg, prev); err != nil {
		if dnsUnreachableErr(err) {
			return errResp("dns_unreachable")
		}
		return errResp(err.Error())
	}
	a.ensureDnsStateHonest()
	return a.getDnsConfigData()
}

// DeleteDnsCustom — удаление пользовательской записи; если она была
// включена — переприменение остатка. Системные записи (sys:*) не удаляются.
func (a *App) DeleteDnsCustom(id string) map[string]interface{} {
	if strings.HasPrefix(id, "sys:") {
		return errResp("system_entry")
	}
	cfg := a.cfg.GetXboxDnsConfig()
	prev := cfg
	wasEnabled := false
	custom := cfg.Custom[:0]
	for _, c := range cfg.Custom {
		if c.ID == id {
			wasEnabled = c.Enabled
			continue
		}
		custom = append(custom, c)
	}
	if len(custom) == len(cfg.Custom) {
		return errResp("unknown_custom: " + id)
	}
	cfg.Custom = custom
	if !wasEnabled {
		if err := a.cfg.SetXboxDnsConfig(cfg); err != nil {
			return errResp(err.Error())
		}
		return a.getDnsConfigData()
	}
	if !dnsAnyCustomOn(cfg) {
		snap := a.cfg.GetDnsProvidersBeforeCustom()
		cfg.ProvidersBeforeCustom = nil
		if len(snap) > 0 {
			cfg.Providers = snap
			a.log.Info("dns", "custom "+id+" deleted — DNS providers restored from snapshot")
		}
		if err := a.cfg.SetDnsProvidersBeforeCustom(nil); err != nil {
			a.log.Warn("dns", "providers snapshot reset: "+err.Error())
		}
	}
	a.syncDnsDoH(prev, cfg)
	if err := a.applyDnsSources(cfg, prev); err != nil {
		if dnsUnreachableErr(err) {
			return errResp("dns_unreachable")
		}
		return errResp(err.Error())
	}
	a.ensureDnsStateHonest()
	return a.getDnsConfigData()
}

// ============================================================
// Применение и обслуживание
// ============================================================

// dnsKnownOursIPs — все адреса, которые продукт вообще может прописать в
// систему: базовые провайдеры (включая выключенные), пресет сервиса,
// легаси-адреса и пользовательские записи. Сверка снимков и чистка остатков.
func (a *App) dnsKnownOursIPs() []string {
	out := make([]string, 0, 24)
	out = append(out, config.DnsServiceIPv4Primary, config.DnsServiceIPv4Secondary)
	out = append(out, config.LegacyDnsServiceIPv4...)
	for _, p := range config.Providers() {
		out = append(out, p.IPv4...)
	}
	cfg := a.cfg.GetXboxDnsConfig()
	for _, c := range cfg.Custom {
		if c.Primary != "" {
			out = append(out, c.Primary)
		}
		if c.Secondary != "" {
			out = append(out, c.Secondary)
		}
	}
	return out
}

// dnsReconfigure — список адресов для менеджера: IPv4 включённых источников
// (провайдеры, пресет, записи). IPv6 не применяется: при применении менеджер
// снимает статический v6-DNS с адаптера. Заодно обновляет вселенную «наших»
// адресов — она нужна снимку и чистке остатков даже при пустом наборе.
func (a *App) dnsReconfigure() config.XboxDnsConfig {
	cfg := a.cfg.GetXboxDnsConfig()
	a.xboxDns.Configure(cfg.PlainIPs())
	a.xboxDns.SetKnownOurs(a.dnsKnownOursIPs())
	return cfg
}

// applyDnsEnabled переключает системный DNS: включение — pre-flight живости
// серверов и прописывание на первичный адаптер (со снимком предыдущих
// адресов), выключение — восстановление сохранённых/DHCP.
func (a *App) applyDnsEnabled(enabled bool) error {
	a.dnsReconfigure()

	if enabled {
		if err := a.xboxDns.Enable(); err != nil {
			return err
		}
		a.persistDnsPrevious()
		a.invalidateSysCaches()
		a.log.Info("dns", "system DNS applied (primary adapter)")
		return nil
	}

	if err := a.xboxDns.Disable(); err != nil {
		a.log.Warn("dns", "restore: "+err.Error())
	}
	a.persistDnsPrevious()
	a.invalidateSysCaches()
	a.log.Info("dns", "system DNS restored")
	return nil
}

// persistDnsPrevious — снимок DNS первичного адаптера в config.json.
func (a *App) persistDnsPrevious() {
	if err := a.cfg.SetXboxDnsPrevious(a.xboxDns.Previous()); err != nil {
		a.log.Warn("dns", "persist previous DNS: "+err.Error())
	}
}

// dnsSyncApplied — сверка набора на первичном адаптере, пока наш DNS включён:
// недостающие настроенные адреса добавляются, лишние наши (выключенные
// источники, легаси-адреса сервиса) убираются. Если нашего DNS на адаптере
// нет вовсе — не вмешиваемся (включением управляет тумблер).
func (a *App) dnsSyncApplied() {
	cfg := a.cfg.GetXboxDnsConfig()
	if !cfg.Enabled {
		return
	}
	primary := xboxdns.PrimaryAdapterName()
	if primary == "" {
		return
	}
	want := cfg.PlainIPs()
	if len(want) == 0 {
		return
	}
	wantSet := make(map[string]bool, len(want))
	for _, ip := range want {
		wantSet[strings.ToLower(ip)] = true
	}
	knownOurs := make(map[string]bool, len(want)+16)
	for ip := range wantSet {
		knownOurs[strings.ToLower(ip)] = true
	}
	for _, p := range config.Providers() {
		for _, ip := range p.IPv4 {
			knownOurs[strings.ToLower(ip)] = true
		}
	}
	for _, ip := range []string{config.DnsServiceIPv4Primary, config.DnsServiceIPv4Secondary} {
		knownOurs[strings.ToLower(ip)] = true
	}
	for _, ip := range config.LegacyDnsServiceIPv4 {
		knownOurs[strings.ToLower(ip)] = true
	}
	for _, c := range cfg.Custom {
		if c.Primary != "" {
			knownOurs[strings.ToLower(strings.TrimSpace(c.Primary))] = true
		}
		if c.Secondary != "" {
			knownOurs[strings.ToLower(strings.TrimSpace(c.Secondary))] = true
		}
	}

	cur := xboxdns.AdapterStaticDNS(primary)
	oursOnAdapter := 0
	var missing, extra []string
	seen := map[string]bool{}
	for _, ip := range cur {
		low := strings.ToLower(ip)
		if seen[low] {
			continue
		}
		seen[low] = true
		if wantSet[low] {
			oursOnAdapter++
			continue
		}
		if knownOurs[low] {
			extra = append(extra, ip)
		}
	}
	for _, ip := range want {
		if !seen[strings.ToLower(ip)] {
			missing = append(missing, ip)
		}
	}
	if oursOnAdapter == 0 || (len(missing) == 0 && len(extra) == 0) {
		return
	}
	a.log.Info("dns", fmt.Sprintf("DNS set drift on %s: missing=%v extra=%v — repairing", primary, missing, extra))
	a.dnsReconfigure()
	if err := a.xboxDns.Repair(); err != nil {
		a.log.Warn("dns", "sync repair: "+err.Error())
	}
}

// dnsStartupRepair — починка при старте: (1) если на первичном адаптере
// стоит 127.0.0.1/::1 — артефакт удалённого encrypted-режима, нашего
// локального резолвера больше не существует: вернуть сохранённое/DHCP;
// (2) на виртуальных адаптерах (Tailscale, vEthernet/WSL, …) наш пресет
// от старых версий сбрасывается в DHCP — он ломал MagicDNS и WSL;
// (3) если применение выключено, но на первичном адаптере остались наши
// адреса (сбой восстановления, мусор в снимке старых версий) — снять их.
func (a *App) dnsStartupRepair() {
	a.dnsReconfigure()
	a.xboxDns.SetPrevious(a.cfg.GetXboxDnsPrevious())

	primary := xboxdns.PrimaryAdapterName()
	if primary != "" && xboxdns.HasLoopbackDNS(primary) {
		a.log.Warn("dns", "loopback DNS on primary adapter (leftover of removed encrypted mode), restoring")
		_ = a.cfg.SetXboxDnsEnabled(false)
		if err := a.xboxDns.Disable(); err != nil {
			a.log.Warn("dns", "loopback restore: "+err.Error())
		}
		a.persistDnsPrevious()
	}
	a.xboxDns.CleanupForeignAdapters(primary)

	if !a.cfg.GetXboxDnsConfig().Enabled && a.xboxDns.HasOurDNSInSystem() {
		a.log.Warn("dns", "our DNS addresses on primary adapter while disabled — cleaning up")
		if err := a.xboxDns.Disable(); err != nil {
			a.log.Warn("dns", "leftover cleanup: "+err.Error())
		}
		a.persistDnsPrevious()
	}
}

// dnsReapplyStartup — старт при включённом тумблере и отсутствии нашего DNS
// в системе (перезапуск приложения всегда снимает DNS): переприменяем.
// Ретраи 3×10с — сеть и адаптеры после логона/сна поднимаются с задержкой.
// При неудаче флаг НЕ гасится: ретрай повторит netwatch при появлении сети.
func (a *App) dnsReapplyStartup() {
	for attempt := 1; attempt <= 3; attempt++ {
		if !a.cfg.GetXboxDnsConfig().Enabled {
			return
		}
		if err := a.applyDnsEnabled(true); err != nil {
			a.log.Warn("dns", fmt.Sprintf("startup re-apply %d/3 failed: %v", attempt, err))
		} else {
			a.log.Info("dns", "system DNS re-applied on startup")
			return
		}
		select {
		case <-a.stopCh:
			return
		case <-time.After(10 * time.Second):
		}
	}
}

// dnsReapplyOnOnline — сеть вернулась, а включённый DNS в системе не
// прописан (стартовые ретраи не успели): одна попытка переприменить.
func (a *App) dnsReapplyOnOnline() {
	if !a.cfg.GetXboxDnsConfig().Enabled || a.xboxDns.IsSetInSystem() {
		return
	}
	if err := a.applyDnsEnabled(true); err != nil {
		a.log.Warn("dns", "re-apply after network restore failed: "+err.Error())
		return
	}
	a.log.Info("dns", "system DNS re-applied after network restore")
}

// dnsVerifyApplied — проверка живости прописанного DNS: отвечает и есть
// включённые источники — флаг enabled подтверждается (без источников флаг
// не трогаем — не включаем мастер, погашенный сторожем); не отвечает —
// принудительное отключение (восстановление сохранённых/DHCP) и сброс всех
// тумблеров источников. Нашего DNS в системе нет — выходим молча: включённый
// тумблер переприменят стартовые ретраи и netwatch при появлении сети,
// гасить флаг здесь нельзя.
func (a *App) dnsVerifyApplied() {
	if !a.xboxDns.IsSetInSystem() {
		return
	}
	cfg := a.cfg.GetXboxDnsConfig()
	if len(cfg.EnabledSourceIDs()) > 0 {
		_ = a.cfg.SetXboxDnsEnabled(true)
	}
	if err := a.xboxDns.VerifyServers(); err != nil {
		a.log.Warn("dns", "system DNS servers do not respond, disabling: "+err.Error())
		// Гасим все источники: после отключения тумблеры не должны «висеть».
		cfg = a.cfg.GetXboxDnsConfig()
		cfg.LastActive = cfg.EnabledSourceIDs()
		cfg.XboxEnabled = false
		cfg.Providers = map[string]bool{}
		for i := range cfg.Custom {
			cfg.Custom[i].Enabled = false
		}
		_ = a.cfg.SetXboxDnsConfig(cfg)
		_ = a.cfg.SetXboxDnsEnabled(false)
		if err := a.xboxDns.Disable(); err != nil {
			a.log.Warn("dns", "watchdog restore: "+err.Error())
		}
		a.persistDnsPrevious()
		a.log.Info("dns", "DNS force-disabled (servers unreachable), sources parked to last_active")
		return
	}
	a.log.Info("dns", "system DNS detected and verified")
}

// dnsHealthProbeList — список адресов для цикла доступности: включённые
// источники плюс все известные (базовые провайдеры, серверные, пользовательские
// записи, пресет Xbox) — вкладка DNS показывает живость каждого, даже
// выключенного. Дубликаты убираются без учёта регистра.
func (a *App) dnsHealthProbeList(enabled []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, 24)
	add := func(ip string) {
		ip = strings.ToLower(strings.TrimSpace(ip))
		if ip == "" || seen[ip] {
			return
		}
		if !config.ValidDnsIP(ip) {
			return
		}
		seen[ip] = true
		out = append(out, ip)
	}
	for _, ip := range enabled {
		add(ip)
	}
	add(config.DnsServiceIPv4Primary)
	add(config.DnsServiceIPv4Secondary)
	for _, p := range config.Providers() {
		for _, ip := range p.IPv4 {
			add(ip)
		}
	}
	for _, c := range a.cfg.GetXboxDnsConfig().Custom {
		add(c.Primary)
		add(c.Secondary)
	}
	return out
}

// dnsSourcesAutoOff — включённый источник, все адреса которого не ответили
// два цикла проверки подряд, выключается сам: остальные источники продолжают
// работать, системе переприменяется остаток. Лог + уведомление + событие.
func (a *App) dnsSourcesAutoOff(h *xboxdns.Health) {
	cfg := a.cfg.GetXboxDnsConfig()
	if !cfg.Enabled {
		return
	}
	okByIP := map[string]bool{}
	for _, s := range h.Servers {
		okByIP[strings.ToLower(s.Server)] = s.OK
	}
	allDead := func(ips []string) bool {
		seen := false
		for _, ip := range ips {
			ok, probed := okByIP[strings.ToLower(strings.TrimSpace(ip))]
			if !probed {
				continue
			}
			seen = true
			if ok {
				return false
			}
		}
		return seen
	}

	type key struct {
		kind, id string
	}
	var dead []key
	if cfg.XboxEnabled && allDead([]string{config.DnsServiceIPv4Primary, config.DnsServiceIPv4Secondary}) {
		dead = append(dead, key{"xbox", ""})
	}
	for _, p := range config.Providers() {
		if cfg.Providers[p.ID] && allDead(p.IPv4) {
			dead = append(dead, key{"provider", p.ID})
		}
	}
	for _, c := range cfg.Custom {
		if c.Enabled && allDead([]string{c.Primary, c.Secondary}) {
			dead = append(dead, key{"custom", c.ID})
		}
	}

	a.dnsDeadMu.Lock()
	defer a.dnsDeadMu.Unlock()
	if len(dead) == 0 {
		a.dnsDeadStreak = nil
		return
	}
	if a.dnsDeadStreak == nil {
		a.dnsDeadStreak = map[string]int{}
	}
	var due []key
	for _, k := range dead {
		id := k.kind + "|" + k.id
		a.dnsDeadStreak[id]++
		if a.dnsDeadStreak[id] >= 2 {
			due = append(due, k)
		}
	}
	if len(due) == 0 {
		return
	}
	a.dnsDeadStreak = nil

	prev := cfg
	var names []string
	for _, k := range due {
		switch k.kind {
		case "xbox":
			cfg.XboxEnabled = false
			names = append(names, "Xbox DNS")
		case "provider":
			delete(cfg.Providers, k.id)
			if p := config.ProviderByID(k.id); p != nil && p.Name != "" {
				names = append(names, p.Name)
			} else {
				names = append(names, k.id)
			}
		case "custom":
			for i := range cfg.Custom {
				if cfg.Custom[i].ID == k.id {
					cfg.Custom[i].Enabled = false
					names = append(names, cfg.Custom[i].Name)
				}
			}
		}
	}
	if len(names) == 0 {
		return
	}
	a.log.Warn("dns", "источники не отвечают 2 проверки подряд, авто-выключение: "+strings.Join(names, ", "))
	// Применение в общем слоте (как у тумблеров): фронт получает dns:apply
	// и видит признак applying.
	a.dnsApplyBegin("auto_off")
	a.dnsApplyRun("auto_off", func() error {
		a.syncDnsDoH(prev, cfg)
		if err := a.applyDnsSources(cfg, prev); err != nil {
			return err
		}
		a.ensureDnsStateHonest()
		return nil
	})
	lang := a.cfg.GetLanguage()
	if a.cfg.GetNotificationsEnabled() {
		_ = notify.Show("ZPUI · DNS", tr(lang, "dns_source_off", strings.Join(names, ", ")))
	}
	a.emitSafe("xboxdns:source_auto_off", map[string]interface{}{"sources": names})
}

// enableXboxDNSForAutoselect — хук автоподбора (verifyYouTubeAfterBest):
// включает пресет Xbox при обнаруженной DNS-подмене YouTube и сбрасывает
// DNS-кеш, чтобы повторная проверка не увидела старые адреса.
func (a *App) enableXboxDNSForAutoselect() {
	cfg := a.cfg.GetXboxDnsConfig()
	if cfg.Enabled {
		return
	}
	a.log.Info("autoselect", "enabling DNS (youtube DNS poisoning)")
	prev := cfg
	cfg.XboxEnabled = true
	if err := a.applyDnsSources(cfg, prev); err != nil {
		a.log.Warn("dns", "autoselect enable: "+err.Error())
		return
	}
	executil.HiddenCmd("ipconfig", "/flushdns").Run()
}
