package config

import (
	"fmt"
	"strconv"
	"strings"
)


func (c *Config) GetZapretPath() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ZapretPath
}

func (c *Config) SetZapretPath(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ZapretPath = path
}

func (c *Config) GetCurrentStrategy() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.CurrentStrategy
}

func (c *Config) SetCurrentStrategy(strategy string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.CurrentStrategy = strategy
	return c.save()
}

// GetIpsetMode возвращает режим ipset-фильтра; пустое/неверное значение = loaded.
func (c *Config) GetIpsetMode() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	switch c.IpsetMode {
	case "none", "any":
		return c.IpsetMode
	default:
		return "loaded"
	}
}

// IpsetModeStored — true, если режим уже записан (миграция со старой
// маркерной схемы выполняется только при пустом поле).
func (c *Config) IpsetModeStored() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.IpsetMode != ""
}

func (c *Config) SetIpsetMode(mode string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.IpsetMode = mode
	return c.save()
}

func copyStratToggles(src map[string]bool) map[string]bool {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]bool, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// GetStratTierToggles — глубокая копия карты видимости тиров (nil, если
// выбор ещё не сохранялся); карта не отдаётся наружу под замком конфига.
func (c *Config) GetStratTierToggles() map[string]bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return copyStratToggles(c.StratTierToggles)
}

func (c *Config) SetStratTierToggles(toggles map[string]bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.StratTierToggles = copyStratToggles(toggles)
	return c.save()
}

// ValidPortSpec — проверка списка портов/диапазонов через запятую:
// «1024-65535», «1024-1934,1936-65535», «443,2053,8443». Диапазон с
// перепутанными границами (a>b) невалиден.
func ValidPortSpec(spec string) bool {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return true
	}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return false
		}
		lo, hi, found := strings.Cut(part, "-")
		lo = strings.TrimSpace(lo)
		hi = strings.TrimSpace(hi)
		if !isDigits(lo) {
			return false
		}
		if !found {
			if n, err := strconv.Atoi(lo); err != nil || n < 1 || n > 65535 {
				return false
			}
			continue
		}
		if !isDigits(hi) {
			return false
		}
		a, err1 := strconv.Atoi(lo)
		b, err2 := strconv.Atoi(hi)
		if err1 != nil || err2 != nil || a > b || a < 1 || b > 65535 {
			return false
		}
	}
	return true
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// GetGameFilterPorts — порты игрового фильтра из настроек; пустые/битые
// значения откатываются на дефолт 1024-65535.
func (c *Config) GetGameFilterPorts() (tcp, udp string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	tcp, udp = c.GameFilter.TCPPorts, c.GameFilter.UDPPorts
	if !ValidPortSpec(tcp) {
		tcp = ""
	}
	if !ValidPortSpec(udp) {
		udp = ""
	}
	if strings.TrimSpace(tcp) == "" {
		tcp = "1024-65535"
	}
	if strings.TrimSpace(udp) == "" {
		udp = "1024-65535"
	}
	return tcp, udp
}

func (c *Config) SetGameFilterPorts(tcp, udp string) error {
	if !ValidPortSpec(tcp) || !ValidPortSpec(udp) {
		return fmt.Errorf("invalid port spec")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.GameFilter.TCPPorts = strings.TrimSpace(tcp)
	c.GameFilter.UDPPorts = strings.TrimSpace(udp)
	return c.save()
}

func (c *Config) GetZapretSkipped() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ZapretSkipped
}

func (c *Config) SetZapretSkipped(skipped bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ZapretSkipped = skipped
	return c.save()
}

func (c *Config) GetVpnIgnore() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]string(nil), c.VpnIgnore...)
}

// SetVpnIgnore заменяет список игнора целиком (управление из настроек).
func (c *Config) SetVpnIgnore(names []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	c.VpnIgnore = out
	return c.save()
}

func (c *Config) GetResourceDropPct() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.ResourceDropPct <= 0 {
		return 70
	}
	return c.ResourceDropPct
}

func (c *Config) SetResourceDropPct(pct int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if pct < 10 {
		pct = 10
	}
	if pct > 100 {
		pct = 100
	}
	c.ResourceDropPct = pct
	return c.save()
}

func (c *Config) GetResourceCheckInterval() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.ResourceCheckInterval < 5 {
		return 10
	}
	if c.ResourceCheckInterval > 60 {
		return 60
	}
	return c.ResourceCheckInterval
}

func (c *Config) SetResourceCheckInterval(min int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if min < 5 {
		min = 5
	}
	if min > 60 {
		min = 60
	}
	c.ResourceCheckInterval = min
	return c.save()
}

func (c *Config) GetBlockCheckConfig() BlockCheckConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.BlockCheck
}

func (c *Config) SetBlockCheckConfig(b BlockCheckConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if b.TimeoutSec <= 0 {
		b.TimeoutSec = 8
	}
	c.BlockCheck = b
	return c.save()
}

func (c *Config) GetServiceCrashCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ServiceCrashCount
}

func (c *Config) SetServiceCrashCount(count int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ServiceCrashCount = count
	return c.save()
}

func (c *Config) GetServiceLastCrash() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ServiceLastCrash
}

func (c *Config) GetLastFullZapretRestore() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.LastFullZapretRestore
}

func (c *Config) SetLastFullZapretRestore(t int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LastFullZapretRestore = t
	return c.save()
}

func (c *Config) GetZapretGeneration() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ZapretGeneration
}

func (c *Config) SetZapretGeneration(gen string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ZapretGeneration = gen
	return c.save()
}

func (c *Config) SetServiceLastCrash(t int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ServiceLastCrash = t
	return c.save()
}
