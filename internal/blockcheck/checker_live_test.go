package blockcheck

import (
	"os"
	"testing"
)

// Живая проверка против реальных ресурсов — только при ZPUI_CHECKER_LIVE=1
// (по образцу netmetrics_live_test.go). Проверяет, что транспорт чекера
// договаривается до HTTP/2 с h2-only сервером (dns.quad9.net) и что
// CheckSubdomains находит известные субдомены Discord.
func TestLiveCheckerRealResources(t *testing.T) {
	if os.Getenv("ZPUI_CHECKER_LIVE") != "1" {
		t.Skip("живой режим: ZPUI_CHECKER_LIVE=1")
	}

	c := NewChecker(true, true, true, 8)

	r := c.Check("https://dns.quad9.net")
	if r.Verdict == VerdictDown {
		t.Errorf("dns.quad9.net = DOWN (HTTP %d): h2-артефакт вернулся?", r.HTTP.Status)
	}
	t.Logf("dns.quad9.net: verdict=%s http_status=%d", r.Verdict, r.HTTP.Status)

	sub := c.CheckSubdomains("discord.com")
	t.Logf("discord.com: candidates=%d existing=%d", sub.Candidates, sub.Existing)
	for _, s := range sub.SubResources {
		t.Logf("  %-24s %s", s.Host, s.Verdict)
	}
	if sub.Existing == 0 {
		t.Error("ожидается хотя бы один существующий субдомен discord.com")
	}
}
