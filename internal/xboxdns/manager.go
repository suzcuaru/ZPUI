//go:build windows

package xboxdns

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"zpui/internal/config"
	"zpui/internal/executil"
)

// Управление системным DNS Windows. Режим один — plain, только IPv4:
// адреса включённых серверов прописываются на ПЕРВИЧНЫЙ физический адаптер
// (лучший маршрут по умолчанию, не виртуальный); IPv6-стек при применении не
// трогается вовсе, остатки утечки старых сборок снимаются самолечением.
// Чтение состояния — реестр, применение — dnsapi с netsh-фолбэком,
// PowerShell не используется.

// AdapterDNS — текущие системные DNS одного адаптера (IPv4 и IPv6 вместе).
type AdapterDNS struct {
	Name string   `json:"name"`
	DNS  []string `json:"dns"`
}

// ServerHealth — результат проверки одного DNS-сервера.
type ServerHealth struct {
	Server string `json:"server"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
}

// Health — снимок проверки доступности серверов. OK = отвечает хотя бы один.
type Health struct {
	Servers   []ServerHealth `json:"servers"`
	OK        bool           `json:"ok"`
	CheckedAt time.Time      `json:"checked_at"`
}

// FailText — перечень несработавших серверов одной строкой (для причин).
func (h *Health) FailText() string {
	var parts []string
	for _, s := range h.Servers {
		if !s.OK {
			parts = append(parts, s.Server+": "+s.Error)
		}
	}
	return strings.Join(parts, "; ")
}

// Log — минимальный лог-интерфейс менеджера: приложение передаёт свой
// slog-логгер, служба ZPUI — адаптер над служебным журналом.
type Log interface {
	Info(category, msg string)
	Warn(category, msg string)
}

type Manager struct {
	mu         sync.RWMutex
	enabled    bool
	servers    []string        // IPv4 для применения (в порядке приоритета)
	original   []string        // снимок "adapter|family|значение" до первого применения
	knownOurs  map[string]bool // все адреса, которые продукт вообще может прописать
	lastHealth *Health
	log        Log
}

func NewManager(log Log) *Manager {
	return &Manager{log: log}
}

// Configure задаёт список адресов (IPv4).
func (m *Manager) Configure(servers []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.servers = append([]string(nil), servers...)
}

// SetKnownOurs задаёт полную вселенную «наших» адресов: пресет сервиса,
// легаси-адреса, все базовые провайдеры и пользовательские записи — даже
// выключенные. Снимок предыдущих адресов и чистка остатков сверяются с ней:
// адрес, попавший в снимок как «предыдущий», при выключении вернулся бы
// в систему и жил там вечно.
func (m *Manager) SetKnownOurs(ips []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.knownOurs = make(map[string]bool, len(ips))
	for _, ip := range ips {
		m.knownOurs[strings.ToLower(strings.TrimSpace(ip))] = true
	}
}

// isOursLocked — адрес принадлежит продукту (серверы, вселенная, loopback).
func (m *Manager) isOursLocked(v string) bool {
	if v == "127.0.0.1" {
		return true
	}
	if m.knownOurs[strings.ToLower(strings.TrimSpace(v))] {
		return true
	}
	for _, o := range m.servers {
		if strings.EqualFold(v, o) {
			return true
		}
	}
	return false
}

func (m *Manager) IsEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled
}

// SetPrevious загружает снимок DNS из config.json (при старте приложения).
func (m *Manager) SetPrevious(entries []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(entries) == 0 {
		m.original = nil
		return
	}
	m.original = append([]string(nil), entries...)
}

// Previous — текущий снимок для персистенции в config.json.
func (m *Manager) Previous() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]string(nil), m.original...)
}

// SystemDNS — текущие DNS поднятых адаптеров (IPv4+IPv6, без дублей).
// Читается из реестра, без внешних процессов.
func SystemDNS() []AdapterDNS {
	var out []AdapterDNS
	for _, a := range listAdapters() {
		if !a.Up {
			continue
		}
		ips := adapterDNSIPs(a.GUID)
		if ips == nil {
			ips = []string{}
		}
		out = append(out, AdapterDNS{Name: a.Name, DNS: ips})
	}
	return out
}

// SystemDNSFlat — плоские списки системных DNS всех поднятых адаптеров
// (IPv4 и IPv6 раздельно, без дублей) для панели «Сейчас в системе».
func SystemDNSFlat() (v4, v6 []string) {
	seen := map[string]bool{}
	for _, a := range listAdapters() {
		if !a.Up {
			continue
		}
		v4s, v4d := registryDNSValues(tcpipKeyFmt, a.GUID)
		v6s, v6d := registryDNSValues(tcpip6Fmt, a.GUID)
		all := make([]string, 0, len(v4s)+len(v4d)+len(v6s)+len(v6d))
		all = append(all, v4s...)
		all = append(all, v4d...)
		all = append(all, v6s...)
		all = append(all, v6d...)
		for _, ip := range all {
			if seen[ip] {
				continue
			}
			p := net.ParseIP(ip)
			if p == nil {
				continue
			}
			seen[ip] = true
			if p.To4() != nil {
				v4 = append(v4, ip)
			} else {
				v6 = append(v6, ip)
			}
		}
	}
	return v4, v6
}

// CheckAvailable — доступен ли сервис xbox-dns.ru (TCP:443 до его адреса).
func (m *Manager) CheckAvailable() bool {
	ips, err := net.LookupHost("xbox-dns.ru")
	if err != nil || len(ips) == 0 {
		return false
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ips[0], "443"), 4*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// CheckServers проверяет, что сконфигурированные адреса реально отвечают
// (тестовый резолв UDP:53 на каждый, 2 попытки). Жив — хотя бы один.
func (m *Manager) CheckServers() *Health {
	m.mu.RLock()
	servers := append([]string(nil), m.servers...)
	m.mu.RUnlock()
	return m.CheckServersOf(servers)
}

// CheckServersOf проверяет произвольный список адресов — не только
// сконфигурированные: вкладка DNS показывает живость каждого провайдера,
// включая выключенные. Снимок сохраняется в lastHealth.
func (m *Manager) CheckServersOf(servers []string) *Health {
	h := m.probeServers(servers)
	m.mu.Lock()
	m.lastHealth = h
	m.mu.Unlock()
	return h
}

// probeServers — сама проверка без записи в lastHealth. Служебные проверки
// (pre-flight включения, сторож) не должны затирать полный снимок доступности:
// он после них терял выключенных провайдеров, и индикаторы вкладки гасли.
func (m *Manager) probeServers(servers []string) *Health {
	h := &Health{CheckedAt: time.Now()}
	if len(servers) == 0 {
		h.Servers = append(h.Servers, ServerHealth{Server: "", Error: "no DNS servers configured"})
		return h
	}
	for _, server := range servers {
		if err := dnsProbe(server); err != nil {
			h.Servers = append(h.Servers, ServerHealth{Server: server, Error: err.Error()})
			continue
		}
		h.Servers = append(h.Servers, ServerHealth{Server: server, OK: true})
		h.OK = true
	}
	return h
}

// CachedHealth — последний снимок CheckServers (nil — ещё не проверяли).
func (m *Manager) CachedHealth() *Health {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastHealth
}

// VerifyServers — проверка «хотя бы один отвечает» с ошибкой-причиной.
// Результат в lastHealth не пишется: вызывается сторожем и при старте,
// полный снимок доступности ведёт только фоновый цикл.
func (m *Manager) VerifyServers() error {
	m.mu.RLock()
	servers := append([]string(nil), m.servers...)
	m.mu.RUnlock()
	if h := m.probeServers(servers); !h.OK {
		return fmt.Errorf("dns servers do not respond: %s", h.FailText())
	}
	return nil
}

// IsSetInSystem — фактическая активность: статический DNS первичного
// адаптера совпадает с одним из сконфигурированных адресов.
func (m *Manager) IsSetInSystem() bool {
	m.mu.RLock()
	servers := append([]string(nil), m.servers...)
	m.mu.RUnlock()

	name := PrimaryAdapterName()
	if name == "" {
		return false
	}
	guid := adapterGUID(name)
	if guid == "" {
		return false
	}
	for _, ip := range adapterStaticDNS(guid) {
		for _, s := range servers {
			if strings.EqualFold(ip, s) {
				return true
			}
		}
	}
	return false
}

// HasLoopbackDNS — на адаптере стоит 127.0.0.1/::1: артефакт удалённого
// encrypted-режима, наш локальный резолвер больше не существует никогда.
func HasLoopbackDNS(name string) bool {
	guid := adapterGUID(name)
	if guid == "" {
		return false
	}
	for _, ip := range adapterStaticDNS(guid) {
		if ip == "127.0.0.1" || ip == "::1" {
			return true
		}
	}
	return false
}

// Enable прописывает адреса серверов на первичный адаптер. Предварительно
// проверяет отклик серверов (pre-flight), сохраняет снимок предыдущих DNS
// для последующего восстановления и убирает наш пресет с виртуальных
// адаптеров, оставшийся от старых версий.
func (m *Manager) Enable() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.servers) == 0 {
		return fmt.Errorf("в таблице нет IP-адресов DNS-серверов (добавьте сервер или включите пресет)")
	}

	// Pre-flight: применять можно только живые серверы — иначе система
	// останется без работающего DNS.
	if h := m.probeServers(m.servers); !h.OK {
		return fmt.Errorf("DNS-серверы недоступны (%s)", h.FailText())
	}

	primary := PrimaryAdapterName()
	if primary == "" {
		return fmt.Errorf("не найден физический адаптер с маршрутом по умолчанию")
	}
	guid := adapterGUID(primary)
	if guid == "" {
		return fmt.Errorf("не найден GUID адаптера %q", primary)
	}

	// Снимок берётся только при первом применении: перерименение при
	// переключении провайдеров не должно записать наш же прошлый набор
	// (или адреса пользователя, живущие до нас) как «предыдущие».
	if len(m.original) == 0 {
		m.original = m.snapshotAdapter(primary, guid)
	}
	m.log.Info("xboxdns", fmt.Sprintf("Enabling DNS on %s: v4=%v (previous=%v)", primary, m.servers, m.original))

	// Только IPv4: IPv6 на адаптере не трогаем ни в каком направлении.
	if err := setStaticDNS(primary, guid, m.servers, nil); err != nil {
		return fmt.Errorf("system dns: %w", err)
	}
	m.healLeakedV6(primary, guid)

	m.cleanupForeign(primary, m.servers)
	m.enabled = true
	m.log.Info("xboxdns", "DNS enabled")
	return nil
}

// Repair — сверка/починка применённого набора: перепрописывает текущий список
// серверов на первичный адаптер БЕЗ pre-flight и нового снимка (DNS уже
// включён — snapshot делать нельзя, чтобы не записать наш набор как
// «предыдущий»). Полная перезапись списка добавляет недостающие и убирает
// лишние наши адреса за один вызов.
func (m *Manager) Repair() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.servers) == 0 {
		return nil
	}
	primary := PrimaryAdapterName()
	if primary == "" {
		return fmt.Errorf("не найден физический адаптер с маршрутом по умолчанию")
	}
	guid := adapterGUID(primary)
	if guid == "" {
		return fmt.Errorf("не найден GUID адаптера %q", primary)
	}
	if err := setStaticDNS(primary, guid, m.servers, nil); err != nil {
		return err
	}
	m.healLeakedV6(primary, guid)
	m.cleanupForeign(primary, m.servers)
	return nil
}

// snapshotAdapter — снимок DNS адаптера в формате "adapter|family|значение"
// (значение — IP или "dhcp"). Только IPv4: IPv6 продукт не трогает вообще
// (ни прописывает, ни восстанавливает, ни очищает). Статический адрес,
// принадлежащий продукту (включённые серверы, вселенная knownOurs, loopback),
// снимком не считается: восстанавливать его после выключения нельзя — он
// вернулся бы в систему и жил там вечно. Хранятся ВСЕ статические адреса
// адаптера: раньше брали только первый — остальные терялись при
// восстановлении.
func (m *Manager) snapshotAdapter(name, guid string) []string {
	v4s, _ := registryDNSValues(tcpipKeyFmt, guid)
	out := []string{}
	found := false
	for _, v := range v4s {
		if m.isOursLocked(v) {
			continue
		}
		out = append(out, name+"|4|"+v)
		found = true
	}
	if !found {
		out = append(out, name+"|4|dhcp")
	}
	return out
}

// healLeakedV6 — самолечение сборок до-fix: старая SetInterfaceDnsSettings
// писала наши IPv4-адреса и в IPv6-стек адаптера (Tcpip6\...\NameServer).
// Если там стоит ровно наш набор (каждый адрес — из вселенной продукта) —
// снимаем v6 в DHCP; если среди адресов есть чужие (свои v6-DNS пользователя)
// — не трогаем. Вызывается под m.mu.
func (m *Manager) healLeakedV6(name, guid string) {
	v6, _ := registryDNSValues(tcpip6Fmt, guid)
	if len(v6) == 0 {
		return
	}
	for _, a := range v6 {
		if !m.isOursLocked(a) {
			return
		}
	}
	if err := clearStaticDNS(name, guid, "6"); err != nil {
		m.log.Warn("xboxdns", "clean leaked IPv6 DNS on "+name+": "+err.Error())
		return
	}
	m.log.Info("xboxdns", "cleaned IPv6 DNS leaked by older builds on "+name)
}

// Disable — выключение: вернуть сохранённые адреса, а если снимка нет, но
// наш DNS ещё стоит в системе — сбросить первичный адаптер на DHCP. Значения
// снимка, принадлежащие продукту (остатки старых багов в снимке), не
// восстанавливаются; если после восстановления на адаптере всё ещё остались
// наши адреса — адаптер сбрасывается на DHCP.
func (m *Manager) Disable() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.log.Info("xboxdns", "Disabling DNS, restoring original addresses")

	if len(m.original) == 0 {
		if m.isSetInSystemLocked() {
			if err := m.resetPrimaryDHCP(); err != nil {
				m.log.Warn("xboxdns", "reset primary to DHCP: "+err.Error())
			}
		}
		m.enabled = false
		m.log.Info("xboxdns", "DNS disabled (no previous snapshot)")
		return nil
	}

	var errs []string
	for _, entry := range m.original {
		parts := strings.SplitN(entry, "|", 3)
		if len(parts) != 3 {
			continue
		}
		adapter, family, value := parts[0], parts[1], parts[2]
		if family != "4" {
			// IPv6 не трогаем (решение продукта); в снимках старых версий
			// могли остаться v6-записи — игнорируем их.
			continue
		}
		if value != "" && value != "dhcp" && m.isOursLocked(value) {
			m.log.Warn("xboxdns", fmt.Sprintf("snapshot value %q is ours, skipping (stale snapshot)", value))
			continue
		}
		guid := adapterGUID(adapter)
		if guid == "" {
			m.log.Warn("xboxdns", fmt.Sprintf("adapter %q not found for restore", adapter))
			continue
		}
		var err error
		if value == "" || value == "dhcp" {
			err = clearStaticDNS(adapter, guid, family)
			if err == nil {
				m.log.Info("xboxdns", fmt.Sprintf("Restored %s DNS (family %s) to DHCP", adapter, family))
			}
		} else {
			if family == "4" {
				err = setStaticDNS(adapter, guid, []string{value}, nil)
			} else {
				err = setStaticDNS(adapter, guid, nil, []string{value})
			}
			if err == nil {
				m.log.Info("xboxdns", fmt.Sprintf("Restored %s DNS (family %s) to %s", adapter, family, value))
			}
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", adapter, err))
		}
	}

	m.original = nil
	m.enabled = false

	// Контрольная зачистка: наши адреса не должны пережить выключение.
	// Сюда попадают снятые вручную пользователем адреса пары, мусор в
	// снимке и частично провалившееся восстановление.
	if name := PrimaryAdapterName(); name != "" {
		if guid := adapterGUID(name); guid != "" {
			left := false
			for _, ip := range adapterStaticDNS(guid) {
				if m.isOursLocked(ip) {
					left = true
					break
				}
			}
			if left {
				m.log.Warn("xboxdns", "our addresses survived restore on "+name+", resetting to DHCP")
				if err := resetDNSToDHCP(name, guid); err != nil {
					errs = append(errs, "leftover reset: "+err.Error())
				}
				m.healLeakedV6(name, guid)
			}
		}
	}

	m.log.Info("xboxdns", "DNS disabled")
	if len(errs) > 0 {
		return fmt.Errorf("partial restore errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// RestoreDHCP сбрасывает DNS первичного адаптера на DHCP и снимок предыдущих
// адресов (настройки персистентны, вызывается и при старте).
func (m *Manager) RestoreDHCP() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.resetPrimaryDHCP(); err != nil {
		return err
	}
	m.original = nil
	m.enabled = false
	return nil
}

func (m *Manager) resetPrimaryDHCP() error {
	primary := PrimaryAdapterName()
	if primary == "" {
		return fmt.Errorf("no primary adapter found")
	}
	guid := adapterGUID(primary)
	if guid == "" {
		return fmt.Errorf("no GUID for adapter %q", primary)
	}
	m.log.Info("xboxdns", "Restoring DHCP on "+primary)
	return resetDNSToDHCP(primary, guid)
}

// isSetInSystemLocked — IsSetInSystem без захвата mu.
func (m *Manager) isSetInSystemLocked() bool {
	name := PrimaryAdapterName()
	if name == "" {
		return false
	}
	guid := adapterGUID(name)
	if guid == "" {
		return false
	}
	for _, ip := range adapterStaticDNS(guid) {
		if m.isOursLocked(ip) {
			return true
		}
		if ip == "::1" {
			return true
		}
	}
	return false
}

// HasOurDNSInSystem — на первичном адаптере остались адреса продукта, хотя
// применение выключено (остаток сбоя/старой версии). Чистится при старте.
func (m *Manager) HasOurDNSInSystem() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	name := PrimaryAdapterName()
	if name == "" {
		return false
	}
	guid := adapterGUID(name)
	if guid == "" {
		return false
	}
	for _, ip := range adapterStaticDNS(guid) {
		if m.isOursLocked(ip) {
			return true
		}
	}
	return false
}

// CleanupForeignAdapters снимает наш пресет (и остатки encrypted-режима)
// с виртуальных адаптеров: keep — адаптер, который не трогаем.
func (m *Manager) CleanupForeignAdapters(keep string) {
	m.mu.RLock()
	servers := append([]string(nil), m.servers...)
	m.mu.RUnlock()
	m.cleanupForeign(keep, servers)
}

func (m *Manager) cleanupForeign(keep string, servers []string) {
	ours := append(append([]string(nil), servers...), config.LegacyDnsServiceIPv4...)
	ours = append(ours,
		config.DnsServiceIPv6Primary, config.DnsServiceIPv6Secondary, // v6-остатки прежних версий
		"127.0.0.1", "::1")
	for _, a := range listAdapters() {
		if !a.Up || a.Name == keep || !a.Virtual {
			continue
		}
		static := adapterStaticDNS(a.GUID)
		hit := false
		for _, ip := range static {
			for _, o := range ours {
				if strings.EqualFold(ip, o) {
					hit = true
					break
				}
			}
			if hit {
				break
			}
		}
		if !hit {
			continue
		}
		if err := resetDNSToDHCP(a.Name, a.GUID); err != nil {
			m.log.Warn("xboxdns", fmt.Sprintf("cleanup virtual %s: %v", a.Name, err))
			continue
		}
		m.log.Info("xboxdns", fmt.Sprintf("Cleaned our DNS preset from virtual adapter %s (reset to DHCP)", a.Name))
	}
}

func runNetsh(args ...string) error {
	_, err := executil.RunWithTimeout(15*time.Second, "netsh", args...)
	return err
}

// SetDoHServer регистрирует пару «IP + DoH-шаблон» в Windows
// (netsh dns add encryption). На системах без поддержки DoH (Windows 10 без
// соответствующих обновлений) netsh вернёт ошибку — вызывающий решает сам.
func SetDoHServer(server, template string) error {
	return runNetsh("dns", "add", "encryption",
		"server="+strings.TrimSpace(server),
		"dohtemplate="+strings.TrimSpace(template),
		"autoupgrade=yes", "udpfallback=no")
}

// DeleteDoHServer снимает DoH-регистрацию адреса (лучший результат — не ошибка).
func DeleteDoHServer(server string) {
	_ = runNetsh("dns", "delete", "encryption", "server="+strings.TrimSpace(server))
}
