package config

import "strings"

// Источники обновлений (updates_source). Выбор в панели «Обновления»,
// один на приложение и запреты (решение владельца 12.11.2026). auto —
// приоритет сервера ZPUI Live, недоступен — остальные каналы.
const (
	UpdatesSourceAuto    = "auto"
	UpdatesSourceServer  = "server"
	UpdatesSourceYandex  = "yandex"
	UpdatesSourceGitHub  = "github"
	UpdatesSourceGHProxy = "ghproxy"
)

func ValidUpdatesSource(s string) bool {
	switch s {
	case UpdatesSourceAuto, UpdatesSourceServer, UpdatesSourceYandex, UpdatesSourceGitHub, UpdatesSourceGHProxy:
		return true
	}
	return false
}

// GetUpdatesSource — нормализованный источник; пустое/незнакомое значение
// трактуется как auto (конфиг мог писаться руками).
func (c *Config) GetUpdatesSource() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s := strings.ToLower(strings.TrimSpace(c.UpdatesSource))
	if !ValidUpdatesSource(s) {
		return UpdatesSourceAuto
	}
	return s
}

func (c *Config) SetUpdatesSource(s string) error {
	s = strings.ToLower(strings.TrimSpace(s))
	if !ValidUpdatesSource(s) {
		s = UpdatesSourceAuto
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.UpdatesSource = s
	return c.save()
}
