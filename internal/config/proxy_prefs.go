package config

import (
	"fmt"
)


// StaticIP — запомненная статическая конфигурация сети: адрес, маска и шлюз,
// которые применялись в этой подсети (ключ хранения — "a.b.c.d/len").
type StaticIP struct {
	IP   string `json:"ip"`
	Mask string `json:"mask"`
	GW   string `json:"gw"`
}

// SocksProxyConfig — локальный SOCKS5-прокси для устройств в домашней сети:
// слушает на 0.0.0.0; клиенту достаточно указать IP компьютера и порт.
// Enabled — мастер-тумблер: без включённой статики прокси бесполезен
// (адрес ПК меняется при каждой аренде DHCP), поэтому по умолчанию выключен.
// StaticSubnets — запомненная статика по подсетям: при возврате в знакомую
// сеть сторож применяет её автоматически.
type SocksProxyConfig struct {
	Port          int                 `json:"port"` // по умолчанию 1080
	Enabled       bool                `json:"enabled"`
	StaticSubnets map[string]StaticIP `json:"static_subnets,omitempty"`
}

func (c *Config) GetProxyEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Proxy.Enabled
}

func (c *Config) SetProxyEnabled(on bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Proxy.Enabled = on
	return c.save()
}

// GetStaticSubnets — копия карты запомненной статики (не nil).
func (c *Config) GetStaticSubnets() map[string]StaticIP {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]StaticIP, len(c.Proxy.StaticSubnets))
	for k, v := range c.Proxy.StaticSubnets {
		out[k] = v
	}
	return out
}

// SetStaticSubnet — запомнить (или обновить) статику подсети.
func (c *Config) SetStaticSubnet(subnet string, s StaticIP) error {
	if subnet == "" {
		return fmt.Errorf("пустая подсеть")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Proxy.StaticSubnets == nil {
		c.Proxy.StaticSubnets = map[string]StaticIP{}
	}
	c.Proxy.StaticSubnets[subnet] = s
	return c.save()
}

// GetSocksProxyPort — порт SOCKS5-прокси (1..65535, иначе 1080).
func (c *Config) GetSocksProxyPort() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Proxy.Port < 1 || c.Proxy.Port > 65535 {
		return 1080
	}
	return c.Proxy.Port
}

func (c *Config) SetSocksProxyPort(port int) error {
	if port < 1 || port > 65535 {
		port = 1080
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Proxy.Port = port
	return c.save()
}

func (c *Config) GetProxyStaticIP() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ProxyStaticIP
}

func (c *Config) SetProxyStaticIP(on bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ProxyStaticIP = on
	return c.save()
}
