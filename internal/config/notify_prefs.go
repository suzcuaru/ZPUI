package config

import (
)


func (c *Config) GetSounds() SoundsConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Sounds
}

func (c *Config) SetSounds(s SoundsConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Sounds = s
	return c.save()
}

// forceNotificationsOn включает все уведомления (продуктовое решение v1.8.0).
func (c *Config) forceNotificationsOn() {
	c.NotificationsEnabled = true
	c.NotifyZPUIUpdates = true
	c.NotifyZapretUpdates = true
	c.NotifyMissingFiles = true
	c.NotifyServiceCrash = true
	c.NotifyResourceDrop = true
	c.NotifyErrors = true
	c.NotifyStrategyTest = true
}

func (c *Config) GetNotificationsEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.NotificationsEnabled
}

func (c *Config) SetNotificationsEnabled(enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.NotificationsEnabled = enabled
	return c.save()
}

func (c *Config) ShouldNotify(event string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.NotificationsEnabled {
		return false
	}
	switch event {
	case "zpui_update", "zapret_update":
		// Уведомления об обновлениях управляются единым тумблером «проверка обновлений».
		return c.AutoUpdateCheck
	case "service_crash":
		return c.NotifyServiceCrash
	case "resource_drop":
		return c.NotifyResourceDrop
	case "missing_files":
		return c.NotifyMissingFiles
	case "errors":
		return c.NotifyErrors
	default:
		return false
	}
}

func (c *Config) SetNotifyFlags(flags map[string]bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v, ok := flags["notify_zpui_updates"]; ok {
		c.NotifyZPUIUpdates = v
	}
	if v, ok := flags["notify_zapret_updates"]; ok {
		c.NotifyZapretUpdates = v
	}
	if v, ok := flags["notify_missing_files"]; ok {
		c.NotifyMissingFiles = v
	}
	if v, ok := flags["notify_service_crash"]; ok {
		c.NotifyServiceCrash = v
	}
	if v, ok := flags["notify_errors"]; ok {
		c.NotifyErrors = v
	}
	if v, ok := flags["notify_resource_drop"]; ok {
		c.NotifyResourceDrop = v
	}
	return c.save()
}

// GetLastNotifiedVersion возвращает последнюю версию компонента, о которой
// уже было отправлено уведомление (или "" если уведомлений не было).
func (c *Config) GetLastNotifiedVersion(component string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	switch component {
	case "ZPUI":
		return c.LastNotifiedZPUIVersion
	case "zapret":
		return c.LastNotifiedZapretVersion
	}
	return ""
}

// SetLastNotifiedVersion сохраняет версию, о которой отправлено уведомление.
func (c *Config) SetLastNotifiedVersion(component, version string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch component {
	case "ZPUI":
		c.LastNotifiedZPUIVersion = version
	case "zapret":
		c.LastNotifiedZapretVersion = version
	default:
		return nil
	}
	return c.save()
}
