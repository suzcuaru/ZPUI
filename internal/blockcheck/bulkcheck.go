package blockcheck

import (
	"sync"
	"time"
)

type BulkTarget struct {
	Name string
	URL  string
}

type BulkResult struct {
	Name      string      `json:"name"`
	URL       string      `json:"url"`
	OK        bool        `json:"ok"`
	Blocked   bool        `json:"blocked"`
	Verdict   string      `json:"verdict"`
	LatencyMs int64       `json:"latency_ms"`
	Reason    string      `json:"reason,omitempty"`
	TCP       LayerResult `json:"tcp,omitempty"`
	TLS       LayerResult `json:"tls,omitempty"`
	HTTP      LayerResult `json:"http,omitempty"`
}

type BulkReport struct {
	Default []BulkResult `json:"default"`
	User    []BulkResult `json:"user"`
}

func (c *Checker) BulkCheck(defaultTargets []BulkTarget, userTargets []BulkTarget) *BulkReport {
	checkAll := func(targets []BulkTarget) []BulkResult {
		results := make([]BulkResult, len(targets))
		sem := make(chan struct{}, 50)
		var wg sync.WaitGroup
		for i, t := range targets {
			wg.Add(1)
			go func(idx int, bt BulkTarget) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				results[idx] = c.checkOne(bt)
			}(i, t)
		}
		wg.Wait()
		return results
	}

	return &BulkReport{
		Default: checkAll(defaultTargets),
		User:    checkAll(userTargets),
	}
}

// isBlockingVerdict возвращает true для вердиктов, означающих реальную
// блокировку (РКН/TSPU/DPI). 4xx от CDN и 5xx сюда НЕ входят —
// это не блокировка, это серверная проблема или норма для CDN.
func isBlockingVerdict(v string) bool {
	switch v {
	case VerdictTCPBlock, VerdictTLSBlock, VerdictDNSBlock, VerdictHTTPStub:
		return true
	}
	return false
}

func (c *Checker) checkOne(t BulkTarget) BulkResult {
	start := time.Now()

	res := c.Check(t.URL)
	latency := time.Since(start).Milliseconds()

	// OK = сервер доступен: либо отдал контент (2xx/3xx),
	// либо ответил 4xx от CDN (TLS прошла, сервер жив — не блокировка).
	ok := res.Verdict == VerdictOK

	// Blocked = реальная блокировка РКН/DPI (TLS reset, DNS, stub, и т.п.).
	// Timeout и DOWN сюда не входят — это не блокировка, это недоступность.
	blocked := isBlockingVerdict(res.Verdict)

	reason := ""
	if !ok {
		reason = res.Verdict
		if len(res.Notes) > 0 {
			reason = res.Notes[0]
		}
	}

	return BulkResult{
		Name:      t.Name,
		URL:       t.URL,
		OK:        ok,
		Blocked:   blocked,
		Verdict:   res.Verdict,
		LatencyMs: latency,
		Reason:    reason,
		TCP:       res.TCP,
		TLS:       res.TLS,
		HTTP:      res.HTTP,
	}
}
