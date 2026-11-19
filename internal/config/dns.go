package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)


// LegacyDnsServiceIPv4 — прежние IPv4-адреса сервиса: остаются на виртуальных
// адаптерах от старых версий ZPUI и вычищаются при старте наравне с текущим
// пресетом (см. xboxdns.CleanupForeignAdapters).
var LegacyDnsServiceIPv4 = []string{"111.88.96.50", "111.88.96.51"}

// DnsServer — одна запись таблицы DNS-серверов.
type DnsServer struct {
	Addr    string `json:"addr"`    // IP, host[:port] или URL (для doh)
	Type    string `json:"type"`    // plain | dot | doh
	Enabled bool   `json:"enabled"` // запись участвует в работе
}

// XboxDnsConfig — настройки системного DNS: адреса включённых серверов
// применяются к системе как plain (без локального резолвера). Только IPv4:
// IPv6-DNS больше не поддерживается (v2.0.1) — при применении статический
// v6-DNS с адаптера снимается.
type XboxDnsConfig struct {
	Enabled     bool        `json:"enabled"`      // DNS применён к системе
	Servers     []DnsServer `json:"servers"`      // пользовательская таблица (сеется при первом старте)
	XboxEnabled bool        `json:"xbox_enabled"` // пресет xbox-dns.ru добавлен к таблице

	// Модель провайдеров: включённые базовые (ключи DnsProviders) и
	// пользовательские записи. Servers — легаси-таблица, переносится в
	// новую модель однократной миграцией при первом старте. Без omitempty:
	// пустые контейнеры обязаны попасть в JSON, иначе миграция станет
	// повторяться при каждом старте.
	Providers  map[string]bool `json:"providers"`
	Custom     []CustomDns     `json:"custom"`
	LastActive []string        `json:"last_active,omitempty"` // источники, включённые до полного выключения

	// ProvidersBeforeCustom — снимок включённых базовых провайдеров на время
	// статики: включение пользовательской записи глушит провайдеров, при
	// выключении статики они восстанавливаются по снимку. Пустая карта =
	// «снимка нет» (система при выключении честно уходит в DHCP/сохранённое).
	ProvidersBeforeCustom map[string]bool `json:"dns_providers_before_custom,omitempty"`

	// Previous — снимок DNS первичного адаптера до первого применения
	// (записи вида "adapter|4|значение": значение — IP или "dhcp").
	Previous []string `json:"previous,omitempty"`

	// Легаси-поля прежней модели: читаются однократно при миграции в Load()
	// и больше не заполняются.
	PrimaryDNS   string `json:"primary_dns,omitempty"`
	SecondaryDNS string `json:"secondary_dns,omitempty"`
}

// CustomDns — пользовательская запись DNS: имя, основной и запасной адрес
// (IPv4). DoH — необязательный шаблон шифрования: пара «IP + шаблон»
// регистрируется в Windows как DoH-сервер. System — предустановленная запись
// приложения: включается тумблером, но не редактируется и не удаляется.
type CustomDns struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Primary   string `json:"primary"`
	Secondary string `json:"secondary,omitempty"`
	DoH       string `json:"doh,omitempty"`
	Enabled   bool   `json:"enabled"`
	System    bool   `json:"system,omitempty"`
}

// DnsProvider — базовый провайдер: оба адреса включаются одним тумблером.
// DoH — необязательный шаблон шифрования: при включении провайдера пара
// «IP + шаблон» регистрируется в Windows как DoH-сервер (как у записей).
// Blocked/Reason — провайдер временно отключён сервером (не отвечает):
// включить его нельзя, UI показывает подсказку (DNS 2.0).
type DnsProvider struct {
	ID      string   `json:"id"`
	Name    string   `json:"name,omitempty"`
	IPv4    []string `json:"ipv4"`
	DoH     string   `json:"doh,omitempty"`
	Sort    int      `json:"sort,omitempty"`
	Blocked bool     `json:"blocked,omitempty"`
	Reason  string   `json:"reason,omitempty"`
}

// DnsProvidersSeed — вкомпилированный сид системного списка: из него
// создаётся dns-system.json при первом старте (см. dns_files.go). Xbox DNS
// (xbox-dns.ru) в таблице не состоит: его флаг — XboxEnabled (легаси-поле,
// источники зовут его "xbox"); UI показывает его первой строкой.
var DnsProvidersSeed = []DnsProvider{
	{ID: "yandex", Name: "Яндекс DNS", IPv4: []string{"77.88.8.8", "77.88.8.1"}, Sort: 100},
	{ID: "cloudflare", Name: "Cloudflare", IPv4: []string{"1.1.1.1", "1.0.0.1"}, Sort: 200},
	{ID: "google", Name: "Google DNS", IPv4: []string{"8.8.8.8", "8.8.4.4"}, Sort: 300},
	{ID: "adguard", Name: "AdGuard DNS", IPv4: []string{"94.140.14.14", "94.140.15.15"}, Sort: 400},
	{ID: "comss", Name: "Comss DNS", IPv4: []string{"83.220.169.155", "212.109.195.93"}, DoH: "https://dns.comss.one/dns-query", Sort: 500},
}

// DnsProviders — легаси-алиас сида: используется как фолбэк, пока файл
// dns-system.json не создан (см. dns_files.go). Новый код должен звать
// Providers().
var DnsProviders = DnsProvidersSeed

func NewCustomDnsID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("c%d", time.Now().UnixNano())
	}
	return "c" + hex.EncodeToString(b)
}

// ValidDnsIP — адрес является IP (семья значения не имеет).
func ValidDnsIP(addr string) bool {
	return net.ParseIP(strings.TrimSpace(addr)) != nil
}

// ValidDohTemplate — шаблон DoH: http(s)-URL с хостом.
func ValidDohTemplate(s string) bool {
	u, err := url.Parse(strings.TrimSpace(s))
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}

// EnabledSourceIDs — идентификаторы всех включённых источников
// ("xbox", id базового провайдера, "custom:<id>").
func (x XboxDnsConfig) EnabledSourceIDs() []string {
	var out []string
	if x.XboxEnabled {
		out = append(out, "xbox")
	}
	for _, p := range Providers() {
		if x.Providers[p.ID] {
			out = append(out, p.ID)
		}
	}
	for _, c := range x.Custom {
		if c.Enabled {
			out = append(out, "custom:"+c.ID)
		}
	}
	return out
}

// ValidServerType — тип записи относится к поддерживаемым.
func ValidServerType(t string) bool {
	switch t {
	case "plain", "dot", "doh":
		return true
	}
	return false
}

// ValidServerAddr — адрес синтаксически пригоден для своего типа:
// plain — IP; dot — host или host:port; doh — http(s)-URL либо host[:port]
// (разворачивается в https://host[:port]/dns-query при использовании).
func ValidServerAddr(typ, addr string) bool {
	switch typ {
	case "plain":
		return net.ParseIP(addr) != nil
	case "dot":
		return validHostPort(addr)
	case "doh":
		if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
			u, err := url.Parse(addr)
			return err == nil && u.Host != ""
		}
		return validHostPort(addr)
	}
	return false
}

func validHostPort(s string) bool {
	if s == "" {
		return false
	}
	if h, _, err := net.SplitHostPort(s); err == nil {
		return h != ""
	}
	for _, r := range s {
		if r <= ' ' || r == '/' || r == '\\' {
			return false
		}
	}
	return true
}

// EffectiveServers — включённые источники в порядке применения: пресет
// xbox-dns.ru, базовые провайдеры, пользовательские записи, легаси-таблица.
// Тип записи игнорируется: в систему применяется всё, что является IP-адресом
// (см. PlainIPs).
func (x XboxDnsConfig) EffectiveServers() []DnsServer {
	out := make([]DnsServer, 0, len(x.Servers)+8)
	if x.XboxEnabled {
		out = append(out,
			DnsServer{Addr: DnsServiceIPv4Primary, Type: "plain", Enabled: true},
			DnsServer{Addr: DnsServiceIPv4Secondary, Type: "plain", Enabled: true},
		)
	}
	for _, p := range Providers() {
		if !x.Providers[p.ID] {
			continue
		}
		for _, a := range p.IPv4 {
			out = append(out, DnsServer{Addr: a, Type: "plain", Enabled: true})
		}
	}
	for _, c := range x.Custom {
		if !c.Enabled {
			continue
		}
		for _, a := range []string{c.Primary, c.Secondary} {
			if a = strings.TrimSpace(a); a != "" {
				out = append(out, DnsServer{Addr: a, Type: "plain", Enabled: true})
			}
		}
	}
	for _, s := range x.Servers {
		if s.Enabled && ValidServerType(s.Type) && s.Addr != "" {
			out = append(out, s)
		}
	}
	return out
}

// PlainIPs — IPv4-адреса пресета и включённых записей (то, что можно
// прописать в netsh); записи с не-IP адресами (домены DoT/DoH) пропускаются.
func (x XboxDnsConfig) PlainIPs() []string {
	var out []string
	for _, s := range x.EffectiveServers() {
		if ip := net.ParseIP(strings.TrimSpace(s.Addr)); ip != nil && ip.To4() != nil {
			out = append(out, s.Addr)
		}
	}
	return out
}

// isServicePresetAddr — адрес относится к известным адресам сервиса
// xbox-dns.ru (легаси-поля всегда хранили только их).
func isServicePresetAddr(addr string) bool {
	switch addr {
	case DnsServiceIPv4Primary, DnsServiceIPv4Secondary,
		"111.88.96.50", "111.88.96.51", // устаревшие адреса, которые прописывал баг прежних версий
		DnsServiceIPv6Primary, DnsServiceIPv6Secondary:
		return true
	}
	return false
}

// sameAddrSet — множества адресов совпадают без учёта регистра и порядка.
func sameAddrSet(list, set []string) bool {
	if len(list) != len(set) {
		return false
	}
	for _, s := range set {
		hit := false
		for _, v := range list {
			if strings.EqualFold(v, s) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return true
}

func dropAddrs(list, set []string) []string {
	out := list[:0]
	for _, v := range list {
		hit := false
		for _, s := range set {
			if strings.EqualFold(v, s) {
				hit = true
				break
			}
		}
		if !hit {
			out = append(out, v)
		}
	}
	return out
}

func (c *Config) GetXboxDnsConfig() XboxDnsConfig {
	c.mu.RLock()
	x := c.XboxDns
	c.mu.RUnlock()
	// Глубокая копия ссылочных полей: вызывающие мутируют результат
	// (тумблеры вкладки DNS, гашение сирот из канала Sync) параллельно
	// друг другу — разделяемая map рано или поздно падает fatal'ом
	// concurrent map writes. Персистят копию через SetXboxDnsConfig.
	providers := make(map[string]bool, len(x.Providers))
	for k, v := range x.Providers {
		providers[k] = v
	}
	x.Providers = providers
	if len(x.ProvidersBeforeCustom) > 0 {
		x.ProvidersBeforeCustom = copyStratToggles(x.ProvidersBeforeCustom)
	}
	if len(x.Custom) > 0 {
		custom := make([]CustomDns, len(x.Custom))
		copy(custom, x.Custom)
		x.Custom = custom
	}
	if len(x.Servers) > 0 {
		servers := make([]DnsServer, len(x.Servers))
		copy(servers, x.Servers)
		x.Servers = servers
	}
	if len(x.LastActive) > 0 {
		last := make([]string, len(x.LastActive))
		copy(last, x.LastActive)
		x.LastActive = last
	}
	if len(x.Previous) > 0 {
		prev := make([]string, len(x.Previous))
		copy(prev, x.Previous)
		x.Previous = prev
	}
	return x
}

func (c *Config) SetXboxDnsConfig(x XboxDnsConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Битые/чужие записи не блокируют сохранение — просто выключаются.
	servers := make([]DnsServer, 0, len(x.Servers))
	for _, s := range x.Servers {
		s.Addr = strings.TrimSpace(s.Addr)
		if !ValidServerType(s.Type) || s.Addr == "" || !ValidServerAddr(s.Type, s.Addr) {
			continue
		}
		servers = append(servers, s)
	}
	x.Servers = servers
	custom := make([]CustomDns, 0, len(x.Custom))
	for _, e := range x.Custom {
		e.Name = strings.TrimSpace(e.Name)
		e.Primary = strings.TrimSpace(e.Primary)
		e.Secondary = strings.TrimSpace(e.Secondary)
		e.DoH = strings.TrimSpace(e.DoH)
		if e.ID == "" {
			e.ID = NewCustomDnsID()
		}
		if e.Name == "" || !ValidDnsIP(e.Primary) || (e.Secondary != "" && !ValidDnsIP(e.Secondary)) {
			continue
		}
		if e.DoH != "" && !ValidDohTemplate(e.DoH) {
			e.DoH = ""
		}
		custom = append(custom, e)
	}
	x.Custom = custom
	if x.Providers == nil {
		x.Providers = map[string]bool{}
	}
	x.ProvidersBeforeCustom = copyStratToggles(x.ProvidersBeforeCustom)
	c.XboxDns = x
	return c.save()
}

// SetXboxDnsEnabled переключает флаг применения DNS без изменения прочих полей.
func (c *Config) SetXboxDnsEnabled(enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.XboxDns.Enabled = enabled
	return c.save()
}

// GetDnsProvidersBeforeCustom — глубокая копия снимка включённых провайдеров
// на время статики (nil — снимка нет); карта не отдаётся наружу под замком.
func (c *Config) GetDnsProvidersBeforeCustom() map[string]bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return copyStratToggles(c.XboxDns.ProvidersBeforeCustom)
}

// SetDnsProvidersBeforeCustom — снимок включённых провайдеров на время
// статики; пустая/nil карта стирает снимок.
func (c *Config) SetDnsProvidersBeforeCustom(providers map[string]bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.XboxDns.ProvidersBeforeCustom = copyStratToggles(providers)
	return c.save()
}

func (c *Config) GetXboxDnsPrevious() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]string(nil), c.XboxDns.Previous...)
}

// SetXboxDnsPrevious сохраняет снимок DNS первичного адаптера (nil — сброс).
func (c *Config) SetXboxDnsPrevious(prev []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(prev) == 0 {
		c.XboxDns.Previous = nil
	} else {
		c.XboxDns.Previous = append([]string(nil), prev...)
	}
	return c.save()
}
