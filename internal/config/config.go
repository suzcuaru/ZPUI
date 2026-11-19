package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Актуальные адреса сервиса xbox-dns.ru (публикуются на сайте ресурса).
const (
	DnsServiceIPv4Primary   = "111.88.96.54"
	DnsServiceIPv4Secondary = "111.88.96.55"
	DnsServiceIPv6Primary   = "2a00:ab00:1233:26::50"
	DnsServiceIPv6Secondary = "2a00:ab00:1233:26::51"
	DnsServiceDoT           = "xbox-dns.ru"
	DnsServiceDoH           = "https://xbox-dns.ru/dns-query"
)

type WebConfig struct {
	Port int `json:"port"`
}

type LogConfig struct {
	MaxFiles int    `json:"max_files"`
	Level    string `json:"level"`
}

type BlockCheckConfig struct {
	CheckTCP   bool `json:"check_tcp"`
	CheckTLS   bool `json:"check_tls"`
	CheckHTTP  bool `json:"check_http"`
	TimeoutSec int  `json:"timeout_sec"`
}

type Config struct {
	mu              sync.RWMutex
	savingSince     atomic.Int64
	ZapretPath      string              `json:"zapret_path"`
	CurrentStrategy string              `json:"current_strategy"`
	ModVersion      string              `json:"mod_version"`
	TgProxy         TelegramProxyConfig `json:"tg_proxy"`
	Proxy           SocksProxyConfig    `json:"proxy"`
	// ProxyStaticIP — IP компьютера зафиксирован как статический (для
	// стабильной доступности прокси из ЛВС); снимается обратным переключением.
	ProxyStaticIP   bool          `json:"proxy_static_ip"`
	XboxDns         XboxDnsConfig `json:"xbox_dns"`
	Web             WebConfig     `json:"web"`
	AutoStart       bool          `json:"autostart"`
	Logs            LogConfig     `json:"logs"`
	AutoUpdateCheck bool          `json:"auto_update_check"`
	// AutoSkipDeadDomains — авто-добавление несуществующих доменов (устойчивый
	// NXDOMAIN) в skip-resources.txt. По умолчанию выключено: юзер решил, что
	// файл исключений должен наполняться только руками.
	AutoSkipDeadDomains bool   `json:"auto_skip_dead_domains"`
	ZapretRepoURL       string `json:"zapret_repo_url"`
	ModRepoURL          string `json:"mod_repo_url"`

	Theme          string       `json:"theme"`
	Language       string       `json:"language"`
	Sounds         SoundsConfig `json:"sounds"`
	FirstRunDone   bool         `json:"first_run_done"`
	ZapretSkipped  bool         `json:"zapret_skipped"`
	StartMinimized bool         `json:"start_minimized"`
	CloseToTray    bool         `json:"close_to_tray"`

	// VpnIgnore — VPN, которые пользователь «Пропустил» на стартовом экране:
	// скан окружения больше не поднимает по ним вопрос.
	VpnIgnore []string `json:"vpn_ignore,omitempty"`

	LastZapretState  bool `json:"last_zapret_state"`
	LastProxyState   bool `json:"last_proxy_state"`
	LastXboxDnsState bool `json:"last_xbox_dns_state"`

	// IpsetMode — режим ipset-фильтра: loaded (реальный список), none, any.
	// Раньше режим писался маркерами в сам файл ipset-all.txt, затирая список;
	// теперь файл меняется только обновлением, а режим живёт здесь.
	IpsetMode string `json:"ipset_mode,omitempty"`

	// StratTierToggles — видимость тиров стратегий в списке (ключи a, b, c,
	// d, untested); отсутствующий ключ = дефолт фронта. nil = выбор ещё не
	// сохранялся.
	StratTierToggles map[string]bool `json:"strat_tier_toggles,omitempty"`

	// GameFilter — пользовательские диапазоны портов игрового фильтра
	// (Flowseal 1.10.3): подставляются в %GameFilterTCP/UDP% стратегий.
	// Пустая строка = дефолт 1024-65535.
	GameFilter GameFilterConfig `json:"game_filter"`

	AutoStartZapret  bool `json:"auto_start_zapret"`
	AutoStartProxy   bool `json:"auto_start_proxy"`
	AutoStartXboxDns bool `json:"auto_start_xbox_dns"`

	// dns_default_migrated — одноразовый маркер миграции: с 1.8.0 автозапуск
	// Xbox DNS включён по умолчанию; для конфигов старых версий применяется
	// один раз при первом запуске новой версии.
	DNSDefaultMigrated bool `json:"dns_default_migrated"`

	// auto_start_proxy_migrated — одноразовый маркер: автозапуск Telegram-прокси
	// включён по умолчанию; старые конфиги с явным «выключено» включаются один раз.
	AutoStartProxyMigrated bool `json:"auto_start_proxy_migrated"`

	PowerSaving    bool `json:"power_saving"`
	PowerLevel     int  `json:"power_level"`
	PowerThreshold int  `json:"power_threshold"` // -1 = «от АКБ», 0 = «всегда», иначе 10..50 % (шаг 5)

	BlockCheck BlockCheckConfig `json:"block_check"`

	NotificationsEnabled bool `json:"notifications_enabled"`

	NotifyZPUIUpdates   bool `json:"notify_zpui_updates"`
	NotifyZapretUpdates bool `json:"notify_zapret_updates"`
	NotifyMissingFiles  bool `json:"notify_missing_files"`
	NotifyServiceCrash  bool `json:"notify_service_crash"`
	NotifyResourceDrop  bool `json:"notify_resource_drop"`
	NotifyErrors        bool `json:"notify_errors"`
	ResourceDropPct     int  `json:"resource_drop_pct"`

	ResourceCheckInterval int `json:"resource_check_interval"`

	// Последняя версия, о которой уже отправлено уведомление (дедупликация
	// между запусками, чтобы не спамить тостом при каждом старте).
	LastNotifiedZPUIVersion   string `json:"last_notified_zpui_version"`
	LastNotifiedZapretVersion string `json:"last_notified_zapret_version"`

	ShowStrategyModal  bool `json:"show_strategy_modal"`
	NotifyStrategyTest bool `json:"notify_strategy_test"`

	DisabledMods []string `json:"disabled_mods"`

	ServiceCrashCount int   `json:"service_crash_count"`
	ServiceLastCrash  int64 `json:"service_last_crash"`

	// LastFullZapretRestore — unix-время последней полной переустановки запрета
	// (снос папки + скачивание). Гард от петли «каждый запуск = переустановка».
	LastFullZapretRestore int64 `json:"last_full_zapret_restore"`

	// ZapretGeneration — поколение движка на машине («zapret1»). Смена
	// поколения один раз сбрасывает cooldown полной переустановки.
	ZapretGeneration string `json:"zapret_generation"`

	AutoSwitchEnabled      bool `json:"auto_switch_enabled"`
	AutoSwitchThresholdPct int  `json:"auto_switch_threshold_pct"`
	AutoSwitchIntervalMin  int  `json:"auto_switch_interval_min"`

	Sync SyncConfig `json:"sync"`

	// SyncData — канал ZPUI Sync (секции данных/диагностики на сервер):
	// мастер-тумблер и юзер-оверрайды частот по секциям.
	SyncData SyncDataConfig `json:"sync_data"`

	// Mcp — локальный MCP-сервер для управления ZPUI ИИ-агентами
	// (по умолчанию выключен; см. internal/mcpserver).
	Mcp McpConfig `json:"mcp"`

	// UpdatesSource — источник обновлений приложения и запретов:
	// auto | server | yandex | github | ghproxy (см. updates_source.go).
	UpdatesSource string `json:"updates_source"`

	// Zapret2 — экспертные флаги движка zapret2 (см. zapret2.go).
	Zapret2 Zapret2Options `json:"zapret2,omitempty"`

	// LuaDisabled — выключенные пользователем --lua-desync функции по пресетам:
	// имя файла стратегии → имена функций (без префикса и параметров).
	LuaDisabled map[string][]string `json:"lua_disabled,omitempty"`

	Server ServerConfig `json:"server"`

	// AuthorizationEnabled — сервер управления и личный кабинет. С 2.0.0
	// сервер реализован — включён по умолчанию (одноразовая миграция ниже
	// включает и в существующих конфигах).
	AuthorizationEnabled bool `json:"authorization_enabled"`

	// auth_default_migrated — одноразовый маркер: включение авторизации
	// при переходе на 2.0.0.
	AuthDefaultMigrated bool `json:"auth_default_migrated"`

	// dns_defaults_seeded — одноразовый маркер: дефолтная пользовательская
	// таблица DNS (Яндекс/Google/Cloudflare) сеется один раз; после удаления
	// записей пользователем заново не появляются.
	DNSDefaultsSeeded bool `json:"dns_defaults_seeded"`

	// sounds_default_migrated — одноразовый маркер: звук интерфейса включён
	// по умолчанию; в конфигах старых версий (где сохранено явное «выключено»
	// по старому дефолту) включается один раз при первом запуске новой версии.
	SoundsDefaultMigrated bool `json:"sounds_default_migrated"`

	// tgproxy_bind_migrated — одноразовый маркер: мост MTProto открыт для
	// домашней сети (bind 0.0.0.0 вместо старого дефолта 127.0.0.1, с которым
	// телефон подключиться не мог в принципе).
	TgproxyBindMigrated bool `json:"tgproxy_bind_migrated"`

	configPath string
}

// GameFilterConfig — диапазоны портов игрового фильтра (глобальные из
// настроек). Формат — список портов и диапазонов через запятую
// («1024-1934,1936-65535»); пусто = дефолт 1024-65535. Подставляется
// в %GameFilterTCP/%GameFilterUDP% стратегий; пер-стратегийные порты
// из игрового подбора хранятся в strategy_games и перекрывают эти.
type GameFilterConfig struct {
	TCPPorts string `json:"tcp_ports,omitempty"`
	UDPPorts string `json:"udp_ports,omitempty"`
}

// SoundsConfig — звуки интерфейса (движок на фронте, Web Audio; здесь только настройки).
type SoundsConfig struct {
	Enabled bool   `json:"enabled"`
	Volume  int    `json:"volume"` // 0..100
	Scheme  string `json:"scheme"` // standard | soft | quiet
}

// CachedSubscription — подписка из блока <subscription> (id, название, цена,
// срок действия, состав) для локального хранения: приложение работает на
// подписке без связи с сервером; кеш обновляется при каждом ответе с профилем.
type CachedSubscription struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Price          int    `json:"price"`
	Until          string `json:"until,omitempty"` // RFC3339, срок действия
	SmartSelection bool   `json:"smart_selection"`
	DevicesLimit   int    `json:"devices_limit"`
	TrafficLimitGB int    `json:"traffic_limit_gb"`
	SpeedTotalMbps int    `json:"speed_total_mbps"`
}

// CachedProfile — локальный снимок профиля вместе с подпиской: пишется при
// каждом ответе сервера с <account>, читается, когда сервер недоступен
// (умер или нет связи); при возврате связи обновляется из свежего ответа.
type CachedProfile struct {
	Nick         string              `json:"nick"`
	NickDisplay  string              `json:"nick_display,omitempty"`
	Email        string              `json:"email"`
	CreatedAt    string              `json:"created_at,omitempty"`
	Subscription *CachedSubscription `json:"subscription,omitempty"`
}

// ServerConfig — подключение к серверу управления (аккаунт).
type ServerConfig struct {
	// URL сервера управления (меняется в настройках, если адрес сменится).
	URL string `json:"url"`
	// Сессия личного кабинета.
	AccountEmail string `json:"account_email"`
	AccountToken string `json:"account_token"`
	// «Больше не показывать» у предложения авторизоваться при запуске.
	AuthPromptDismissed bool `json:"auth_prompt_dismissed"`
	// Кеш профиля и подписки (офлайн-работа на подписке).
	Profile *CachedProfile `json:"profile,omitempty"`
}

// SyncDataConfig — настройки канала ZPUI Sync: мастер-тумблер (nil = включён
// для авторизованных) и оверрайды частот по секциям (множитель 1/2/5, выкл).
type SyncDataConfig struct {
	ChannelEnabled *bool                      `json:"channel_enabled,omitempty"`
	Overrides      map[string]SyncOverrideCfg `json:"overrides,omitempty"`
}

// SyncOverrideCfg — оверрайд одной секции.
type SyncOverrideCfg struct {
	Mult int  `json:"mult,omitempty"` // 1|2|5 — во сколько раз реже
	Off  bool `json:"off,omitempty"`  // секция выключена
}

// Update — мутация полей верхнего уровня под замком с сохранением. Прямые
// записи в экспортируемые поля гонятся с фоновыми читателями (marshal в
// save, tray, телеметрия) — все изменения только через него.
func (c *Config) Update(fn func(cfg *Config)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	fn(c)
	return c.save()
}

// RView выполняет fn под read-локом: для точечных чтений сырых полей
// конфига параллельно с Update. Геттеры, которые сами берут RLock, внутри
// вызывать нельзя (рекурсивный RLock может deadlock-нуть при ждущем writer'е).
func (c *Config) RView(fn func()) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	fn()
}

// SyncConfig — идентификация устройства (ID переживает переустановку через реестр).
type SyncConfig struct {
	InstanceID  string `json:"instance_id"`  // уникальный ID экземпляра
	DisplayName string `json:"display_name"` // имя устройства
}

func defaultConfig(zapretDir string) *Config {
	return &Config{
		ZapretPath:      zapretDir,
		CurrentStrategy: "general.bat",
		TgProxy: TelegramProxyConfig{
			Enabled:    false,
			AutoStart:  false,
			Port:       1443,
			BindHost:   "0.0.0.0",
			Secret:     "",
			FallbackCF: true,
			PoolSize:   4,
			BufKB:      256,
		},
		// SOCKS5-прокси включён по умолчанию: поднимается при запуске
		// приложения («Автозапуск SOCKS5-прокси» в настройках).
		Proxy: SocksProxyConfig{
			Port:    1080,
			Enabled: true,
		},
		XboxDns: XboxDnsConfig{
			Enabled:     false,
			Servers:     []DnsServer{},
			XboxEnabled: false,
		},
		Web: WebConfig{
			Port: 8080,
		},
		AutoStart: false,
		// Автосмена стратегий включена по умолчанию: при деградации доступности
		// движок переключается сам. Пользователь может выключить — значение
		// сохранится в config.json и дефолт его не перезапишет.
		AutoSwitchEnabled: true,
		Logs:              LogConfig{MaxFiles: 7, Level: "info"},
		AutoUpdateCheck:   true,
		ZapretRepoURL:     "https://github.com/flowseal/zapret-discord-youtube",
		ModRepoURL:        "https://github.com/bol-van/zapret",

		Theme:        "system",
		Language:     "ru",
		Sounds:       SoundsConfig{Enabled: true, Volume: 60, Scheme: "soft"},
		FirstRunDone: false,
		CloseToTray:  true,

		Mcp:           McpConfig{Enabled: false, Port: McpDefaultPort},
		UpdatesSource: UpdatesSourceAuto,

		NotificationsEnabled: true,

		NotifyStrategyTest: true,
		BlockCheck: BlockCheckConfig{
			CheckTCP:   false,
			CheckTLS:   true,
			CheckHTTP:  true,
			TimeoutSec: 8,
		},

		NotifyZPUIUpdates:     true,
		NotifyZapretUpdates:   true,
		NotifyMissingFiles:    true,
		NotifyServiceCrash:    true,
		NotifyResourceDrop:    true,
		ResourceDropPct:       70,
		ResourceCheckInterval: 10,

		PowerLevel:     1,
		PowerThreshold: -1, // «от АКБ»

		AutoStartProxy:   true,
		AutoStartXboxDns: true,

		AuthorizationEnabled: true,
	}
}

func Load(configPath, zapretDir string) *Config {
	cfg := defaultConfig(zapretDir)
	cfg.configPath = configPath

	data, err := os.ReadFile(configPath)
	if err != nil {
		// Свежий конфиг: модель провайдеров пуста, в системе ничего нашего нет.
		cfg.SoundsDefaultMigrated = true
		cfg.DNSDefaultsSeeded = true
		cfg.XboxDns.Providers = map[string]bool{}
		cfg.XboxDns.Custom = []CustomDns{}
		cfg.Save()
		return cfg
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		cfg.SoundsDefaultMigrated = true
		cfg.DNSDefaultsSeeded = true
		cfg.XboxDns.Providers = map[string]bool{}
		cfg.XboxDns.Custom = []CustomDns{}
		cfg.Save()
		return cfg
	}

	cfg.configPath = configPath

	// DNS: миграция легаси-модели (primary_dns/secondary_dns/…) в таблицу
	// servers. Форс-перезапись адресов сервиса убрана: пользовательские
	// значения больше не затираются при каждом старте. Адреса из легаси-полей
	// в таблицу не импортируются — пресет xbox-dns.ru покрывает их через
	// xbox_enabled (легаси-поля старых версий всегда содержали сервисные
	// адреса: дефолт или перетертые багом).
	if len(cfg.XboxDns.Servers) == 0 {
		if cfg.XboxDns.PrimaryDNS != "" && !isServicePresetAddr(cfg.XboxDns.PrimaryDNS) {
			cfg.XboxDns.Servers = append(cfg.XboxDns.Servers, DnsServer{Addr: cfg.XboxDns.PrimaryDNS, Type: "plain", Enabled: true})
		}
		if cfg.XboxDns.SecondaryDNS != "" && !isServicePresetAddr(cfg.XboxDns.SecondaryDNS) {
			cfg.XboxDns.Servers = append(cfg.XboxDns.Servers, DnsServer{Addr: cfg.XboxDns.SecondaryDNS, Type: "plain", Enabled: true})
		}
		cfg.XboxDns.PrimaryDNS = ""
		cfg.XboxDns.SecondaryDNS = ""
		cfg.Save()
	}
	if cfg.XboxDns.Servers == nil {
		cfg.XboxDns.Servers = []DnsServer{}
	}

	// Модель провайдеров: однократный перенос легаси-таблицы servers.
	// Точное множество включённых адресов, совпавшее с базовым провайдером,
	// становится его тумблером; остатки — пользовательскими записями
	// (включёнными, только если master в этот момент применён).
	if cfg.XboxDns.Providers == nil && cfg.XboxDns.Custom == nil {
		var enabled []string
		for _, s := range cfg.XboxDns.Servers {
			if s.Enabled && ValidDnsIP(s.Addr) {
				enabled = append(enabled, net.ParseIP(strings.TrimSpace(s.Addr)).String())
			}
		}
		prov := map[string]bool{}
		for _, p := range Providers() {
			if sameAddrSet(enabled, p.IPv4) {
				prov[p.ID] = true
				enabled = dropAddrs(enabled, p.IPv4)
			}
		}
		custom := make([]CustomDns, 0, len(enabled))
		for i, a := range enabled {
			custom = append(custom, CustomDns{
				ID:      NewCustomDnsID(),
				Name:    fmt.Sprintf("DNS %d", i+1),
				Primary: a,
				Enabled: cfg.XboxDns.Enabled,
			})
		}
		cfg.XboxDns.Providers = prov
		cfg.XboxDns.Custom = custom
		cfg.XboxDns.Servers = []DnsServer{}
		cfg.Save()
	}

	// Дефолтная таблица DNS больше не сеется: список серверов — это
	// базовые провайдеры и пользовательские записи (маркер сохраняется
	// ради совместимости старых конфигов).
	if !cfg.DNSDefaultsSeeded {
		cfg.DNSDefaultsSeeded = true
		cfg.Save()
	}

	// Экономный режим: кламп новых полей (старые power_saving_auto/deep
	// из прежних конфигов больше не читаются).
	if cfg.PowerLevel < 1 || cfg.PowerLevel > 3 {
		cfg.PowerLevel = 1
	}

	// Звуки интерфейса: клампы для конфигов без этих полей.
	if cfg.Sounds.Volume < 0 || cfg.Sounds.Volume > 100 {
		cfg.Sounds.Volume = 60
	}
	switch cfg.Sounds.Scheme {
	case "standard", "soft", "quiet":
	default:
		cfg.Sounds.Scheme = "soft"
	}
	switch {
	case cfg.PowerThreshold == 0 || cfg.PowerThreshold == -1:
	case cfg.PowerThreshold >= 10 && cfg.PowerThreshold <= 50:
	default:
		cfg.PowerThreshold = 0
	}

	// Одноразовая миграция автозапуска Xbox DNS (новый дефолт с 1.8.0):
	// в конфигах старых версий сохранено явное «выключено» — после обновления
	// включаем один раз, чтобы режим работал «из коробки». Дальше тумблер
	// полностью в распоряжении пользователя.
	if !cfg.DNSDefaultMigrated {
		cfg.AutoStartXboxDns = true
		cfg.DNSDefaultMigrated = true
		cfg.Save()
	}

	// Одноразовая миграция автозапуска Telegram-прокси: тумблера в настройках
	// долго не было и в конфигах лежит явное «выключено» — включаем один раз,
	// прокси должен работать «из коробки». Дальше тумблер в настройках.
	if !cfg.AutoStartProxyMigrated {
		cfg.AutoStartProxy = true
		cfg.AutoStartProxyMigrated = true
		cfg.Save()
	}

	// Одноразовая миграция звука интерфейса (новый дефолт): в конфигах старых
	// версий сохранено явное «выключено» — после обновления включаем один раз,
	// чтобы звук работал «из коробки». Дальше тумблер только за пользователем.
	if !cfg.SoundsDefaultMigrated {
		cfg.Sounds.Enabled = true
		cfg.SoundsDefaultMigrated = true
		cfg.Save()
	}

	// Одноразовая миграция авторизации: сервер управления реализован
	// (2.0.0) — включаем экран входа, вкладку «Пользователь» и телеметрию
	// в существующих конфигах один раз.
	if !cfg.AuthDefaultMigrated {
		cfg.AuthorizationEnabled = true
		cfg.AuthDefaultMigrated = true
		cfg.Save()
	}

	// DNS-файлы: dns-system.json (сид/сервер) и dns-user.json (миграция
	// пользовательских записей из config.json). См. dns_files.go.
	cfg.ensureDnsFiles()

	// Xbox DNS (xbox-dns.ru) выведен из базовых провайдеров: легаси-флаг
	// глушится при каждом старте — пресет больше не применяется и не
	// возвращается из config.json (UI-тумблера для него больше нет).
	if cfg.XboxDns.XboxEnabled {
		cfg.XboxDns.XboxEnabled = false
		cfg.Save()
	}

	// Уведомления всегда включены: настроек уведомлений больше нет,
	// сохранённые ранее «выключенные» флаги игнорируются.
	cfg.forceNotificationsOn()

	return cfg
}

func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.save()
}

func (c *Config) save() error {
	c.savingSince.Store(time.Now().UnixNano())
	defer c.savingSince.Store(0)
	dir := filepath.Dir(c.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// dns-user.json — источник истины для пользовательских DNS-записей:
	// держим в синхроне с config.json при каждом сохранении.
	if err := writeJSONFile(c.dnsUserPath(), c.XboxDns.Custom); err != nil {
		return fmt.Errorf("write dns-user.json: %w", err)
	}

	// Атомарная запись: обрыв питания между усечением и записью прямого
	// os.WriteFile оставил бы битый config.json, который Load молча заменяет
	// дефолтным.
	tmpPath := c.configPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write config tmp: %w", err)
	}
	if err := os.Rename(tmpPath, c.configPath); err != nil {
		// Запасной путь: редкие отказы rename-поверх на Windows (файл на
		// мгновение занят антивирусом) — окно без config.json минимально.
		_ = os.Remove(c.configPath)
		if err2 := os.Rename(tmpPath, c.configPath); err2 != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("replace config: %w", err2)
		}
	}
	return nil
}

// SaveStallAge returns how long the current save has been running.
// Zero means no save is in progress.
func (c *Config) SaveStallAge() time.Duration {
	ns := c.savingSince.Load()
	if ns == 0 {
		return 0
	}
	return time.Since(time.Unix(0, ns))
}

// normalizeDomainList чистит список доменов: трим, lowercase, дедупликация.
func normalizeDomainList(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(items))
	out := make([]string, 0, len(items))
	for _, it := range items {
		it = strings.ToLower(strings.TrimSpace(it))
		if it == "" || seen[it] {
			continue
		}
		seen[it] = true
		out = append(out, it)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (c *Config) GetTheme() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Theme
}

func (c *Config) GetLanguage() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Language == "" {
		return "ru"
	}
	return c.Language
}

func (c *Config) GetCloseToTray() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.CloseToTray
}

func (c *Config) GetAutoSkipDeadDomains() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.AutoSkipDeadDomains
}

func (c *Config) SetTheme(theme string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Theme = theme
	return c.save()
}

// Секции нормализованного skip-resources.txt: авто-исключения одной секцией
// (без повторяющегося заголовка перед каждым доменом — раньше после серии
// NXDOMAIN файл зарастал одинаковыми «# auto: …»), ручные — другой.
const (
	SkipAutoSection   = "# auto: исключены автоматически (NXDOMAIN, домен не существует):"
	SkipManualSection = "# добавлено вручную:"
)

// GetUserExclusions читает пользовательский список исключений доменов
// (zapret/lists/list-exclude-user.txt): одна строка — один домен, строки
// с # — комментарии. Файл может не существовать — тогда список пуст.
func (c *Config) GetUserExclusions() []string {
	c.mu.RLock()
	path := filepath.Join(c.ListsDir(), "list-exclude-user.txt")
	c.mu.RUnlock()
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{}
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.ToLower(strings.TrimSpace(line))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}
