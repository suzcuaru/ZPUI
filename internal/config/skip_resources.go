package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)


// skipResourcesFile returns the path to skip-resources.txt next to the config file.
// This file is shipped with the application and contains pre-defined hosts
// that should never be checked (always-down CDN endpoints, dead subdomains, etc).
// User can edit this file manually to add/remove entries.
// One host per line, lines starting with # are comments, blank lines ignored.
func (c *Config) skipResourcesFile() string {
	return filepath.Join(filepath.Dir(c.configPath), "skip-resources.txt")
}

// GetSkipResources reads the skip-resources.txt file and returns the list of hosts.
// If the file does not exist, returns an empty slice (no exclusions).
func (c *Config) GetSkipResources() []string {
	c.mu.RLock()
	path := c.skipResourcesFile()
	c.mu.RUnlock()
	return readSkipHosts(path)
}

// skipResourcesSystemFile returns the path to skip-resources-system.txt next to
// the config file. The file is owned by the application: default is header-only,
// content is replaced wholesale on server exclusion set version change (state.sync).
func (c *Config) skipResourcesSystemFile() string {
	return filepath.Join(filepath.Dir(c.configPath), "skip-resources-system.txt")
}

// GetSkipResourcesSystem reads the system skip file (server-managed) and
// returns the list of hosts. Format is identical to skip-resources.txt.
func (c *Config) GetSkipResourcesSystem() []string {
	c.mu.RLock()
	path := c.skipResourcesSystemFile()
	c.mu.RUnlock()
	return readSkipHosts(path)
}

// readSkipHosts parses a skip file: one host per line, lines starting with #
// are comments, blank lines ignored, hosts lowercased.
func readSkipHosts(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{}
	}
	var result []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		result = append(result, strings.ToLower(line))
	}
	return result
}

// hostMatchesSkip checks host against a skip list: exact match, subdomain
// match ("google.com" matches "drive.google.com") and contains-match.
func hostMatchesSkip(host string, list []string) bool {
	for _, s := range list {
		if s == "" {
			continue
		}
		if host == s || strings.HasSuffix(host, "."+s) || strings.Contains(host, s) {
			return true
		}
	}
	return false
}

// IsSkippedResource returns true if host matches any entry in the user
// skip file (skip-resources.txt) or the server-managed system skip file
// (skip-resources-system.txt).
// Match is case-insensitive: "google.com" matches "drive.google.com" (subdomain).
func (c *Config) IsSkippedResource(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	return hostMatchesSkip(host, c.GetSkipResources()) ||
		hostMatchesSkip(host, c.GetSkipResourcesSystem())
}

// GetSystemExclusions читает DPI-базу list-exclude-system.txt из каталога
// списков; она же — источник гейта проверок доступности (IsExcludedHost).
func (c *Config) GetSystemExclusions() []string {
	c.mu.RLock()
	dir := c.ListsDir()
	c.mu.RUnlock()
	return readSkipHosts(filepath.Join(dir, "list-exclude-system.txt"))
}

// IsExcludedHost — единый гейт исключений проверок доступности (дашборд и
// автоподбор): skip-resources.txt + skip-resources-system.txt (точное имя,
// поддомен, вхождение) плюс list-exclude-user.txt и list-exclude-system.txt
// (точное имя и поддомен).
func (c *Config) IsExcludedHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	if hostMatchesSkip(host, c.GetSkipResources()) ||
		hostMatchesSkip(host, c.GetSkipResourcesSystem()) {
		return true
	}
	for _, list := range [][]string{c.GetUserExclusions(), c.GetSystemExclusions()} {
		for _, s := range list {
			if host == s || strings.HasSuffix(host, "."+s) {
				return true
			}
		}
	}
	return false
}

// GetSkipResourcesFilePath returns the absolute path to skip-resources.txt.
// Located next to config.json.
func (c *Config) GetSkipResourcesFilePath() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.skipResourcesFile()
}

// GetSkipResourcesSystemFilePath returns the absolute path to
// skip-resources-system.txt. Located next to config.json.
func (c *Config) GetSkipResourcesSystemFilePath() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.skipResourcesSystemFile()
}

// AddSkipResource appends a host to skip-resources.txt if not already present.
func (c *Config) AddSkipResource(host string) error {
	return c.addSkipResource(host, false)
}

// AddSkipResourceWithComment добавляет хост; комментарий-причина в
// нормализованном файле перед каждым доменом не пишется: комментарий с
// префиксом "auto:" означает секцию авто-исключений, остальное — ручную.
func (c *Config) AddSkipResourceWithComment(host, comment string) error {
	trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(comment), "#"))
	return c.addSkipResource(host, strings.HasPrefix(trimmed, "auto:"))
}

func (c *Config) addSkipResource(host string, auto bool) error {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return fmt.Errorf("empty host")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	path := c.skipResourcesFile()
	_, autoHosts, manualHosts := readSkipFileParts(path)
	for _, h := range autoHosts {
		if h == host {
			return nil
		}
	}
	for _, h := range manualHosts {
		if h == host {
			return nil
		}
	}
	if auto {
		autoHosts = append(autoHosts, host)
	} else {
		manualHosts = append(manualHosts, host)
	}
	return writeSkipFile(path, autoHosts, manualHosts)
}

// NormalizeSkipFile — привести skip-resources.txt к плоскому виду:
// ТОЛЬКО домены — без шапки, секций и служебных приписок (см. историю:
// раньше файл разрастался «# auto: …» / «# добавлено вручную:», шапка
// оставалась от старых версий). Вызывается при старте приложения.
func (c *Config) NormalizeSkipFile() {
	c.mu.Lock()
	defer c.mu.Unlock()
	path := c.skipResourcesFile()
	_, autoHosts, manualHosts := readSkipFileParts(path)
	autoHosts = dedupHosts(autoHosts)
	manualHosts = dedupHosts(manualHosts)
	_ = writeSkipFile(path, autoHosts, manualHosts)
}

func dedupHosts(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, h := range in {
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		out = append(out, h)
	}
	return out
}

// readSkipFileParts — разбор файла: шапка (комментарии от начала до первой
// пустой строки/домена; в файл больше не пишется, читается только ради
// совместимости), авто-хосты (под «# auto: …» — старый формат per-host
// заголовков тоже понимается) и ручные.
func readSkipFileParts(path string) (header, autoHosts, manualHosts []string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil
	}
	headerDone := false
	section := 0 // 0 = до секций, 1 = auto, 2 = manual
	lastCommentAuto := false
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if !headerDone {
			if trimmed == "" {
				if len(header) > 0 {
					headerDone = true
				}
				continue
			}
			if strings.HasPrefix(trimmed, "#") {
				header = append(header, trimmed)
				continue
			}
			headerDone = true
		}
		if trimmed == "" {
			continue
		}
		lowered := strings.ToLower(trimmed)
		if strings.HasPrefix(trimmed, "#") {
			switch {
			case strings.HasPrefix(lowered, "# auto:"):
				if section == 0 {
					lastCommentAuto = true // старый per-host заголовок
				}
			case strings.HasPrefix(lowered, SkipManualSection):
				section = 2
			case strings.HasPrefix(lowered, SkipAutoSection):
				section = 1
			}
			continue
		}
		host := lowered
		switch {
		case section == 1:
			autoHosts = append(autoHosts, host)
		case section == 2:
			manualHosts = append(manualHosts, host)
		default:
			if lastCommentAuto {
				autoHosts = append(autoHosts, host)
			} else {
				manualHosts = append(manualHosts, host)
			}
		}
		lastCommentAuto = false
	}
	return header, autoHosts, manualHosts
}

func writeSkipFile(path string, autoHosts, manualHosts []string) error {
	var b strings.Builder
	// Плоский список: только домены — без шапки, секций и служебных приписок.
	// Авто- и ручные хосты сливаются и дедуплицируются — файл читается как обычный хост-лист.
	for _, h := range dedupHosts(append(append([]string{}, autoHosts...), manualHosts...)) {
		b.WriteString(h + "\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}
