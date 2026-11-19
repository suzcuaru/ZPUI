package blockcheck

import (
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// DeepSubResource — результат проверки одного обнаруженного подресурса
// (CDN, API, статика и т. п.), на который ссылается проверяемая страница.
type DeepSubResource struct {
	Host      string `json:"host"`
	Verdict   string `json:"verdict"`
	Blocked   bool   `json:"blocked"`
	OK        bool   `json:"ok"`
	LatencyMs int64  `json:"latency_ms"`
	Reason    string `json:"reason,omitempty"`
}

// DeepReport — итог глубокого исследования ресурса.
type DeepReport struct {
	URL          string            `json:"url"`
	Host         string            `json:"host"`
	MainVerdict  string            `json:"main_verdict"`
	MainBlocked  bool              `json:"main_blocked"`
	Fetched      bool              `json:"fetched"`
	FetchError   string            `json:"fetch_error,omitempty"`
	BodyBytes    int               `json:"body_bytes"`
	SubResources []DeepSubResource `json:"sub_resources"`
	Subdomains   *SubReport        `json:"subdomains,omitempty"`
	Total        int               `json:"total"`
	BlockedCount int               `json:"blocked_count"`
	OKCount      int               `json:"ok_count"`
	CheckedAt    string            `json:"checked_at"`
}

var (
	// absURLRe ловит абсолютные URL вида https://host... в любом месте тела
	// (html-атрибуты, inline-JS, CSS, json-конфиги).
	absURLRe = regexp.MustCompile(`(?i)https?://([a-z0-9][a-z0-9.\-]*[a-z0-9])`)
	// protoRelRe ловит протокольно-относительные ссылки: "//host..." после
	// кавычки/скобки/равно (src="//cdn...", url(//cdn...), ="//cdn...").
	protoRelRe = regexp.MustCompile(`(?i)["'\(=]//([a-z0-9][a-z0-9.\-]*[a-z0-9])`)
)

const deepMaxHosts = 50

// CollectHosts извлекает уникальные внешние именамена хостов, на которые
// ссылается тело страницы (скрипты, стили, картинки, API, CDN). Хост главной
// страницы (skipHost) исключается — он проверяется отдельно.
func CollectHosts(body []byte, skipHost string) []string {
	seen := map[string]bool{}
	skip := strings.ToLower(strings.TrimSpace(skipHost))
	skip = strings.TrimPrefix(skip, "www.")

	add := func(raw string) {
		h := strings.ToLower(strings.TrimSpace(raw))
		h = strings.TrimPrefix(h, "www.")
		if h == "" || h == skip || !looksLikeHost(h) {
			return
		}
		seen[h] = true
	}

	for _, m := range absURLRe.FindAllStringSubmatch(string(body), -1) {
		add(m[1])
	}
	for _, m := range protoRelRe.FindAllStringSubmatch(string(body), -1) {
		add(m[1])
	}

	hosts := make([]string, 0, len(seen))
	for h := range seen {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	return hosts
}

// looksLikeHost отсеивает мусор: требуется минимум одна точка и хотя бы одна
// буква (CDN/хосты всегда содержат буквы; чисто числовые «1.2.3» — это версии/мусор).
func looksLikeHost(h string) bool {
	if !strings.Contains(h, ".") {
		return false
	}
	for i := 0; i < len(h); i++ {
		if h[i] >= 'a' && h[i] <= 'z' {
			return true
		}
	}
	return false
}

// fetchBody скачивает тело страницы (до 1 МБ) для последующего разбора.
func (c *Checker) fetchBody(fullURL string) ([]byte, string) {
	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, "invalid request"
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, classifyErr(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return body, ""
}

// DeepCheck выполняет глубокий анализ ресурса: проверяет сам URL, скачивает
// страницу, извлекает все внешние хосты (CDN, API, скрипты, статику) и
// проверяет каждый на блокировку. Возвращает отчёт со списком подресурсов,
// отсортированным так, что заблокированные наверху.
func (c *Checker) DeepCheck(rawURL string) *DeepReport {
	report := &DeepReport{
		URL:       rawURL,
		CheckedAt: time.Now().Format("2006-01-02 15:04:05"),
	}
	_, host, fullURL := parseURL(rawURL)
	report.Host = host
	if fullURL == "" {
		report.FetchError = "неверный URL"
		return report
	}

	// 1. Проверяем сам ресурс (тот же вердикт, что в обычной проверке).
	main := c.Check(fullURL)
	report.MainVerdict = main.Verdict
	report.MainBlocked = main.Verdict != VerdictOK

	// 2. Проверяем собственные субдомены цели — независимо от того,
	// скачается ли тело страницы.
	report.Subdomains = c.CheckSubdomains(report.Host)

	// 3. Скачиваем тело страницы.
	body, ferr := c.fetchBody(fullURL)
	if len(body) == 0 {
		report.FetchError = "не удалось получить содержимое страницы"
		if ferr != "" {
			report.FetchError += " (" + ferr + ")"
		}
		return report
	}
	report.Fetched = true
	report.BodyBytes = len(body)

	// 4. Собираем внешние хосты и обрезаем до лимита.
	hosts := CollectHosts(body, strings.ToLower(host))
	if len(hosts) > deepMaxHosts {
		hosts = hosts[:deepMaxHosts]
	}
	report.Total = len(hosts)

	// 5. Проверяем каждый хост с ограничением параллелизма.
	results := make([]DeepSubResource, len(hosts))
	sem := make(chan struct{}, 15)
	var wg sync.WaitGroup
	for i, h := range hosts {
		wg.Add(1)
		go func(idx int, checkHost string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = c.checkSubResource(checkHost)
		}(i, h)
	}
	wg.Wait()

	for _, r := range results {
		if r.Blocked {
			report.BlockedCount++
		} else if r.OK {
			report.OKCount++
		}
	}

	// Заблокированные — наверх, затем недоступные, затем доступные, по алфавиту.
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Blocked != results[j].Blocked {
			return results[i].Blocked
		}
		if results[i].OK != results[j].OK {
			return !results[i].OK
		}
		return results[i].Host < results[j].Host
	})
	report.SubResources = results
	return report
}

func deepReason(r CheckResult) string {
	if len(r.Notes) > 0 {
		return r.Notes[0]
	}
	return r.Verdict
}
