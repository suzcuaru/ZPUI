package config

import (
)


// GetSyncChannelEnabled — мастер-тумблер канала (по умолчанию включён).
func (c *Config) GetSyncChannelEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.SyncData.ChannelEnabled == nil || *c.SyncData.ChannelEnabled
}

// GetSyncOverrides — оверрайды секций (имя секции без префикса "data.").
func (c *Config) GetSyncOverrides() map[string]SyncOverrideCfg {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]SyncOverrideCfg, len(c.SyncData.Overrides))
	for k, v := range c.SyncData.Overrides {
		out[k] = v
	}
	return out
}

// SetSyncOverrides — сохранить мастер-тумблер и оверрайды секций.
func (c *Config) SetSyncOverrides(enabled bool, overrides map[string]SyncOverrideCfg) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SyncData.ChannelEnabled = &enabled
	c.SyncData.Overrides = overrides
	return c.save()
}

func (c *Config) GetServerConfig() ServerConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Server
}

func (c *Config) SetServerConfig(s ServerConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Server = s
	return c.save()
}

// UpdateServerConfig — атомарный read-modify-write серверного конфига:
// все изменения полей server.* должны идти через него, иначе параллельные
// писатели (heartbeat, очистка токена по 401) затирают друг друга.
func (c *Config) UpdateServerConfig(fn func(s *ServerConfig)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	fn(&c.Server)
	return c.save()
}

func (c *Config) GetAuthorizationEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.AuthorizationEnabled
}

func (c *Config) SetAuthorizationEnabled(enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.AuthorizationEnabled = enabled
	return c.save()
}
