package app

import (
	"os"
	"path/filepath"
	"strings"
	"time"
	"zpui/internal/blockcheck"

)


// ============================================================
// RESOURCE CHECKER (Block Checker)
// ============================================================

func (a *App) CheckResource(rawURL string) map[string]interface{} {
	if rawURL == "" {
		return map[string]interface{}{"error": "URL required"}
	}

	bc2 := a.cfg.GetBlockCheckConfig()
	checker := blockcheck.NewChecker(bc2.CheckTCP, bc2.CheckTLS, bc2.CheckHTTP, bc2.TimeoutSec)

	provider := checker.GetProviderInfo()
	direct := checker.Check(rawURL)

	blocked := direct.Verdict != blockcheck.VerdictOK

	inList := a.isHostInUserList(direct.Host)

	// Frontend ResourceChecker.jsx chitaet PascalCase-kljuchi
	// (report.Direct.Verdict, report.Direct.TCP.Ok, ...), poetomu sobiraem
	// map vruchnuju s PascalCase-kljuchami, a ne otdajom FullReport kak est'
	// (ego json-tegi lowercase slomali by frontend).
	return map[string]interface{}{
		"report": map[string]interface{}{
			"URL":    rawURL,
			"Host":   direct.Host,
			"Direct": directToMap(direct),
			"Provider": map[string]interface{}{
				"IP":      provider.IP,
				"ISP":     provider.ISP,
				"City":    provider.City,
				"Country": provider.Country,
				"Org":     provider.Org,
				"ASN":     provider.ASN,
			},
			"Blocked":    blocked,
			"BlockType":  direct.Verdict,
			"InUserList": inList,
			"CheckedAt":  time.Now().Format("2006-01-02 15:04:05"),
		},
	}
}

// CheckResourceDeep выполняет глубокий анализ ресурса: проверяет сам URL,
// скачивает страницу, извлекает все внешние хосты (CDN, API, скрипты) и
// проверяет каждый на блокировку. Возвращает отчёт со списком подресурсов.
func (a *App) CheckResourceDeep(rawURL string) map[string]interface{} {
	if rawURL == "" {
		return errResp("URL required")
	}

	bc2 := a.cfg.GetBlockCheckConfig()
	checker := blockcheck.NewChecker(bc2.CheckTCP, bc2.CheckTLS, bc2.CheckHTTP, bc2.TimeoutSec)
	report := checker.DeepCheck(rawURL)

	return map[string]interface{}{"report": report}
}

// directToMap preobrazuet CheckResult v map s PascalCase-kljuchami,
// chtoby frontend ResourceChecker.jsx mog chitat' report.Direct.TCP.Ok i t.d.
func directToMap(r blockcheck.CheckResult) map[string]interface{} {
	return map[string]interface{}{
		"URL":        r.URL,
		"Host":       r.Host,
		"TCP":        layerToMap(r.TCP),
		"TLS":        layerToMap(r.TLS),
		"HTTP":       layerToMap(r.HTTP),
		"Verdict":    r.Verdict,
		"Confidence": r.Confidence,
		"Notes":      r.Notes,
	}
}

func layerToMap(l blockcheck.LayerResult) map[string]interface{} {
	return map[string]interface{}{
		"Ok":       l.Ok,
		"TimeMs":   l.TimeMs,
		"Error":    l.Error,
		"Status":   l.Status,
		"StubPage": l.StubPage,
		"Header":   l.Header,
	}
}

func (a *App) AddHostToUserList(host string) map[string]interface{} {
	if host == "" {
		return map[string]interface{}{"error": "host required"}
	}

	listPath := filepath.Join(a.cfg.ListsDir(), "list-general-user.txt")
	data, _ := os.ReadFile(listPath)
	lines := strings.Split(string(data), "\n")

	for _, l := range lines {
		if strings.TrimSpace(l) == host {
			return map[string]interface{}{"status": "already_exists"}
		}
	}

	content := strings.Join(lines, "\n")
	if !strings.HasSuffix(content, "\n") && content != "" {
		content += "\n"
	}
	content += host + "\n"

	if err := os.WriteFile(listPath, []byte(content), 0644); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"status": "ok"}
}

// AddHostsToUserList добавляет несколько хостов в пользовательский список обхода
// (list-general-user.txt) за один вызов. Используется глубоким анализом для
// массового добавления заблокированных подресурсов.
func (a *App) AddHostsToUserList(hosts []interface{}) map[string]interface{} {
	listPath := filepath.Join(a.cfg.ListsDir(), "list-general-user.txt")
	data, _ := os.ReadFile(listPath)

	existing := map[string]bool{}
	var lines []string
	for _, l := range strings.Split(string(data), "\n") {
		lines = append(lines, l)
		if t := strings.TrimSpace(l); t != "" {
			existing[t] = true
		}
	}

	added := 0
	skipped := 0
	for _, h := range hosts {
		host, ok := h.(string)
		if !ok {
			continue
		}
		host = strings.TrimSpace(host)
		if host == "" || existing[host] {
			if host != "" {
				skipped++
			}
			continue
		}
		existing[host] = true
		lines = append(lines, host)
		added++
	}

	if added == 0 {
		return map[string]interface{}{"status": "ok", "added": 0, "skipped": skipped}
	}

	content := strings.Join(lines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if err := os.WriteFile(listPath, []byte(content), 0644); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"status": "ok", "added": added, "skipped": skipped}
}

func (a *App) isHostInUserList(host string) bool {
	listPath := filepath.Join(a.cfg.ListsDir(), "list-general-user.txt")
	data, err := os.ReadFile(listPath)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == host {
			return true
		}
	}
	return false
}
