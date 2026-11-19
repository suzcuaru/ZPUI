package config

import (
	"fmt"
	"strings"
)

// Zapret2Options — экспертные флаги движка zapret2, добавляемые к аргументам
// любой стратегии (как game filter). Нулевые значения = поведение движка
// по умолчанию.
type Zapret2Options struct {
	Debug             bool `json:"debug"`
	IPCacheLifetime   int  `json:"ipcache_lifetime"`
	IPCacheHostname   bool `json:"ipcache_hostname"`
	CtrackDisable     bool `json:"ctrack_disable"`
	InterceptEmptyAck bool `json:"intercept_empty_ack"`
}

func (c *Config) GetZapret2() Zapret2Options {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Zapret2
}

func (c *Config) SetZapret2(z Zapret2Options) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Zapret2 = z
	return c.save()
}

// GetLuaDisabled — копия карты выключенных lua-функций (nil-safe).
func (c *Config) GetLuaDisabled() map[string][]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string][]string, len(c.LuaDisabled))
	for k, v := range c.LuaDisabled {
		out[k] = append([]string(nil), v...)
	}
	return out
}

// SetLuaDisabledFor добавляет/убирает имя функции в списке выключенных для
// пресета; опустевший список удаляет ключ.
func (c *Config) SetLuaDisabledFor(strategyFilename, fn string, enabled bool) error {
	strategyFilename = strings.TrimSpace(strategyFilename)
	fn = strings.TrimSpace(fn)
	if strategyFilename == "" || fn == "" {
		return fmt.Errorf("пустое имя стратегии или функции")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if enabled {
		for _, ex := range c.LuaDisabled[strategyFilename] {
			if ex == fn {
				return nil
			}
		}
		if c.LuaDisabled == nil {
			c.LuaDisabled = map[string][]string{}
		}
		c.LuaDisabled[strategyFilename] = append(c.LuaDisabled[strategyFilename], fn)
		return c.save()
	}
	list := c.LuaDisabled[strategyFilename]
	out := list[:0]
	for _, ex := range list {
		if ex != fn {
			out = append(out, ex)
		}
	}
	if len(out) == 0 {
		delete(c.LuaDisabled, strategyFilename)
	} else {
		c.LuaDisabled[strategyFilename] = out
	}
	return c.save()
}
