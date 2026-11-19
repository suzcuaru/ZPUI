package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
)


// generateInstanceID создаёт случайный 16-символьный hex-идентификатор экземпляра.
func generateInstanceID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("zp-%x", os.Getpid())
	}
	return hex.EncodeToString(b)
}

// GetSyncInstanceID возвращает ID экземпляра, генерируя его при первом обращении.
// GetSyncInstanceID — уникальный идентификатор устройства. Источник истины —
// реестр (HKCU\Software\ZPUI\DeviceID): пишется один раз, переживает
// переустановку и не зависит от IP/железа. Config-копия — зеркало для
// бэкапов; расхождение чинится в пользу реестра.
func (c *Config) GetSyncInstanceID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if id := loadDeviceIDRegistry(); id != "" {
		if c.Sync.InstanceID != id {
			c.Sync.InstanceID = id
			_ = c.save()
		}
		return id
	}
	if c.Sync.InstanceID == "" {
		c.Sync.InstanceID = generateInstanceID()
	}
	persistDeviceIDRegistry(c.Sync.InstanceID)
	_ = c.save()
	return c.Sync.InstanceID
}

// GetSyncDisplayName возвращает имя устройства для обнаружения (fallback — hostname).
func (c *Config) GetSyncDisplayName() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Sync.DisplayName != "" {
		return c.Sync.DisplayName
	}
	if host, err := os.Hostname(); err == nil {
		return host
	}
	return "ZPUI"
}
