package config

import (
)


func (c *Config) GetAutoSwitchEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.AutoSwitchEnabled
}

func (c *Config) SetAutoSwitchEnabled(enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.AutoSwitchEnabled = enabled
	return c.save()
}

func (c *Config) GetAutoSwitchThresholdPct() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	pct := c.AutoSwitchThresholdPct
	if pct < 50 || pct > 80 {
		return 50
	}
	return pct
}

func (c *Config) SetAutoSwitchThresholdPct(pct int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if pct < 50 {
		pct = 50
	}
	if pct > 80 {
		pct = 80
	}
	c.AutoSwitchThresholdPct = pct
	return c.save()
}

func (c *Config) GetAutoSwitchIntervalMin() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	min := c.AutoSwitchIntervalMin
	if min < 1 {
		return 10
	}
	if min > 60 {
		return 60
	}
	return min
}

func (c *Config) SetAutoSwitchIntervalMin(min int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if min < 1 {
		min = 1
	}
	if min > 60 {
		min = 60
	}
	c.AutoSwitchIntervalMin = min
	return c.save()
}
