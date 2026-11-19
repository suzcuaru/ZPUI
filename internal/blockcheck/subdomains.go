package blockcheck

import (
	"context"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// knownServiceSubdomains — субдомены известных сервисов, критичные для их
// работы, но не видимые в HTML главной страницы и не выводимые простым
// префиксом (другой регистрируемый домен: discord.media, cdn.discordapp.com).
// Ключ — метка домена; совпадение по любой метке проверяемого хоста
// (discord.com, discord.gg, ptb.discord.com → ключ "discord").
var knownServiceSubdomains = map[string][]string{
	"discord": {"discord.media", "gateway.discord.gg", "cdn.discordapp.com", "discordapp.net"},
	"youtube": {"i.ytimg.com", "yt3.ggpht.com"},
	"twitch":  {"static.twitchcdn.net", "static-cdn.jtvnw.net"},
}

// subdomainPrefixes — типовые префиксы для DNS-пробы субдоменов.
var subdomainPrefixes = []string{
	"www", "api", "cdn", "media", "static", "assets", "img", "images",
	"video", "files", "download", "gateway", "ws", "chat", "upload", "content",
}

// SubReport — итог проверки собственных субдоменов цели.
type SubReport struct {
	Host         string            `json:"host"`
	Candidates   int               `json:"candidates"`
	Existing     int               `json:"existing"`
	SubResources []DeepSubResource `json:"sub_resources"`
}

// SubdomainCandidates генерирует кандидатов на проверку: префиксные субдомены
// проверяемого хоста плюс известные субдомены сервисов из словаря.
// Сам хост и дубликаты исключаются.
func SubdomainCandidates(host string) []string {
	base := strings.ToLower(strings.TrimSpace(host))
	base = strings.TrimSuffix(base, ".")
	if i := strings.Index(base, "://"); i >= 0 {
		base = base[i+3:]
	}
	if i := strings.Index(base, "/"); i >= 0 {
		base = base[:i]
	}
	base = strings.TrimPrefix(base, "www.")
	if base == "" || !strings.Contains(base, ".") {
		return nil
	}

	seen := map[string]bool{}
	var out []string
	add := func(h string) {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" || h == base || h == host || seen[h] || !looksLikeHost(h) {
			return
		}
		seen[h] = true
		out = append(out, h)
	}

	for _, p := range subdomainPrefixes {
		add(p + "." + base)
	}
	for _, label := range strings.Split(base, ".") {
		for _, sd := range knownServiceSubdomains[label] {
			add(sd)
		}
	}

	sort.Strings(out)
	return out
}

// dnsExists проверяет, что хост резолвится. NXDOMAIN и прочие ошибки DNS
// трактуются как «субдомена нет» — проверять нечего.
func dnsExists(host string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := net.DefaultResolver.LookupHost(ctx, host)
	if err == nil {
		return true
	}
	if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
		return false
	}
	return false
}

// checkSubResource проверяет один хост и упаковывает результат в DeepSubResource.
func (c *Checker) checkSubResource(host string) DeepSubResource {
	start := time.Now()
	res := c.Check("https://" + host + "/")
	return DeepSubResource{
		Host:      host,
		Verdict:   res.Verdict,
		Blocked:   isBlockingVerdict(res.Verdict),
		OK:        res.Verdict == VerdictOK,
		LatencyMs: time.Since(start).Milliseconds(),
		Reason:    deepReason(res),
	}
}

// CheckSubdomains выявляет блокировку собственных субдоменов: кандидаты
// генерируются по префиксам и словарю известных сервисов, несуществующие
// отсеиваются DNS-пробой, существующие проверяются тем же Check по слоям.
func (c *Checker) CheckSubdomains(host string) *SubReport {
	report := &SubReport{Host: host, SubResources: []DeepSubResource{}}
	candidates := SubdomainCandidates(host)
	report.Candidates = len(candidates)
	if len(candidates) == 0 {
		return report
	}

	// DNS-проба: оставляем только реально существующие субдомены.
	sem := make(chan struct{}, 15)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var existing []string
	for _, cand := range candidates {
		wg.Add(1)
		go func(h string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if !dnsExists(h) {
				return
			}
			mu.Lock()
			existing = append(existing, h)
			mu.Unlock()
		}(cand)
	}
	wg.Wait()
	report.Existing = len(existing)
	if len(existing) == 0 {
		return report
	}
	sort.Strings(existing)

	results := make([]DeepSubResource, len(existing))
	for i, h := range existing {
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
			report.SubResources = append(report.SubResources, r)
		}
	}
	for _, r := range results {
		if !r.Blocked {
			report.SubResources = append(report.SubResources, r)
		}
	}
	return report
}
