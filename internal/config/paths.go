package config

import (
	"path/filepath"
	"strings"
)


// DataDir — каталог данных приложения (рядом с config.json).
func (c *Config) DataDir() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return filepath.Dir(c.configPath)
}

func (c *Config) ListsDir() string {
	return filepath.Join(c.GetZapretPath(), "lists")
}

// BackupsDir — каталог резервных копий рядом с config.json: персистентная
// копия пользовательских списков запрета и скачанный архив запрета
// (для переустановки без сети). Вне папки zapret — переживает её снос.
func (c *Config) BackupsDir() string {
	return filepath.Join(filepath.Dir(c.configPath), "backups")
}

func (c *Config) BinDir() string {
	return filepath.Join(c.GetZapretPath(), "bin")
}

func (c *Config) LogsDir() string {
	return filepath.Join(filepath.Dir(c.configPath), "logs")
}

func (c *Config) StrategyPath(name string) string {
	if !strings.HasSuffix(name, ".bat") {
		name += ".bat"
	}
	return filepath.Join(c.GetZapretPath(), name)
}
