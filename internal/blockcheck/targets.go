package blockcheck

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ReadTargets читает домены из lists/list-general.txt и возвращает цели для проверки.
func ReadTargets(path string) ([]BulkTarget, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseTargets(string(body)), nil
}

// ParseTargets парсит список доменов (по одному на строку) в BulkTarget.
// Комментарии (#) и пустые строки пропускаются. Префикс ^ — синтаксис
// hostlist zapret («только сам домен, без поддоменов»); для проверки
// он снимается, иначе URL получается невалидным.
func ParseTargets(content string) []BulkTarget {
	var targets []BulkTarget
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "PING:") {
			continue
		}
		line = strings.TrimPrefix(line, "^")
		rawURL := line
		if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
			rawURL = "https://" + rawURL
		}
		targets = append(targets, BulkTarget{
			Name: line,
			URL:  rawURL,
		})
	}
	return targets
}

func DefaultTargetsPath(zapretDir string) string {
	return filepath.Join(zapretDir, "lists", "list-general.txt")
}

func UserTargetsPath(listsDir string) string {
	return filepath.Join(listsDir, "list-general-user.txt")
}

// fallbackTargets — ключевые цели, когда основной список отсутствует или пуст
// (битая установка): замер остаётся осмысленным и одинаковым у дашборда и
// автоподбора.
var fallbackTargets = []BulkTarget{
	{Name: "DiscordMain", URL: "https://discord.com"},
	{Name: "YouTubeWeb", URL: "https://www.youtube.com"},
	{Name: "GoogleMain", URL: "https://www.google.com"},
	{Name: "CloudflareWeb", URL: "https://www.cloudflare.com"},
}

// ReadCheckTargets — единый источник целей проверки доступности (дашборд и
// автоподбор): основной список (list-general.txt) + пользовательский
// (list-general-user.txt). Пустой основной список заменяется фолбэком.
func ReadCheckTargets(zapretDir, listsDir string) (def, user []BulkTarget) {
	def, _ = ReadTargets(DefaultTargetsPath(zapretDir))
	if len(def) == 0 {
		def = append([]BulkTarget(nil), fallbackTargets...)
	}
	if body, err := os.ReadFile(UserTargetsPath(listsDir)); err == nil {
		user = ParseTargets(string(body))
	}
	return def, user
}

// FilterCheckTargets — единый фильтр исключений: цель убирается, если гейт
// исключил её имя или хост URL (skip-файлы хранят домены, имя цели тоже
// участвует).
func FilterCheckTargets(targets []BulkTarget, excluded func(host string) bool) []BulkTarget {
	out := make([]BulkTarget, 0, len(targets))
	for _, t := range targets {
		if excluded(t.Name) {
			continue
		}
		host := ""
		if u, err := url.Parse(t.URL); err == nil {
			host = u.Hostname()
		}
		if host != "" && excluded(host) {
			continue
		}
		out = append(out, t)
	}
	return out
}

