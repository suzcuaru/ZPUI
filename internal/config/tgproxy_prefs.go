package config

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)


// TelegramProxyConfig — локальный MTProto-прокси для Telegram Desktop
// (мост трафика через WebSocket к Telegram DC).
type TelegramProxyConfig struct {
	Enabled   bool   `json:"enabled"`
	AutoStart bool   `json:"auto_start"`
	Port      int    `json:"port"`      // по умолчанию 1443
	BindHost  string `json:"bind_host"` // по умолчанию 0.0.0.0 (доступ из домашней сети)
	Secret    string `json:"secret"`    // 32 hex; генерируется при первом обращении

	// Фаза 2: резервные каналы и маскировка.
	FakeTLSDomain   string   `json:"fake_tls_domain,omitempty"`   // ee-secret маскировка под TLS к домену
	ProxyProtocol   bool     `json:"proxy_protocol,omitempty"`    // принимать PROXY protocol v1 (за nginx/haproxy)
	FallbackCF      bool     `json:"fallback_cf"`                 // CF-proxy fallback (default true)
	CFUserDomains   []string `json:"cf_user_domains,omitempty"`   // пользовательские CF-проксированные домены
	CFWorkerDomains []string `json:"cf_worker_domains,omitempty"` // Cloudflare Worker домены
	PoolSize        int      `json:"pool_size,omitempty"`         // размер пула WS на DC (default 4)
	NoSecure        bool     `json:"no_secure,omitempty"`         // CF-прокси и воркеры через :80 без TLS
	DCIP            []string `json:"dc_ip,omitempty"`             // переназначение IP датацентров "dc:ip"

	// Возможности компонента ZPUI-TgProxy.exe (headless tg-ws-proxy).
	NoH2        bool `json:"no_h2,omitempty"`         // выключить HTTP/2-мультиплексирование медиа через CF
	BufKB       int  `json:"buf_kb,omitempty"`        // буфер сокета KB (default 256)
	ForceTestDC bool `json:"force_test_dc,omitempty"` // гнать весь трафик на тестовые DC Telegram
	Verbose     bool `json:"verbose,omitempty"`       // DEBUG-лог компонента
}

// GetTgProxyConfig возвращает конфиг MTProto-прокси, при необходимости
// генерируя secret (32 hex) и сохраняя его.
func (c *Config) GetTgProxyConfig() TelegramProxyConfig {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.TgProxy.Secret == "" {
		c.TgProxy.Secret = generateHexSecret(16)
		_ = c.save()
	}
	if c.TgProxy.Port == 0 {
		c.TgProxy.Port = 1443
	}
	if c.TgProxy.BindHost == "" {
		c.TgProxy.BindHost = "0.0.0.0"
	}
	if c.TgProxy.PoolSize == 0 {
		c.TgProxy.PoolSize = 4
	}
	if c.TgProxy.BufKB == 0 {
		c.TgProxy.BufKB = 256
	}
	// Разовая миграция: старый дефолт 127.0.0.1 делал мост недоступным с
	// телефона — открываем его для домашней сети, если пользователь не выбрал
	// бинд осознанно (маркер ещё не ставился).
	if !c.TgproxyBindMigrated && c.TgProxy.BindHost == "127.0.0.1" {
		c.TgProxy.BindHost = "0.0.0.0"
		c.TgproxyBindMigrated = true
		_ = c.save()
	}
	return c.TgProxy
}

func (c *Config) SetTgProxyConfig(p TelegramProxyConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if p.Port == 0 {
		p.Port = 1443
	}
	if p.BindHost == "" {
		p.BindHost = "0.0.0.0"
	}
	if p.PoolSize <= 0 {
		p.PoolSize = 4
	}
	if p.PoolSize > 64 {
		p.PoolSize = 64
	}
	if p.BufKB <= 0 {
		p.BufKB = 256
	}
	if p.BufKB > 4096 {
		p.BufKB = 4096
	}
	p.CFUserDomains = normalizeDomainList(p.CFUserDomains)
	p.CFWorkerDomains = normalizeDomainList(p.CFWorkerDomains)
	p.FakeTLSDomain = strings.ToLower(strings.TrimSpace(p.FakeTLSDomain))
	c.TgProxy = p
	return c.save()
}

// generateHexSecret возвращает n случайных байт в hex-представлении (2n символов).
func generateHexSecret(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "00000000000000000000000000000000"
	}
	return hex.EncodeToString(b)
}
