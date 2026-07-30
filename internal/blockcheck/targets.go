package blockcheck

import (
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
// Комментарии (#) и пустые строки пропускаются.
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
