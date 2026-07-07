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
	Name      string `json:"name"`
	URL       string `json:"url"`
	OK        bool   `json:"ok"`
	Blocked   bool   `json:"blocked"`
	Verdict   string `json:"verdict"`
	LatencyMs int64  `json:"latency_ms"`
	Reason    string `json:"reason,omitempty"`
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
		var wg sync.WaitGroup
		for i, t := range targets {
			wg.Add(1)
			go func(idx int, bt BulkTarget) {
				defer wg.Done()
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

func (c *Checker) checkOne(t BulkTarget) BulkResult {
	start := time.Now()

	direct := c.Check(t.URL)
	latency := time.Since(start).Milliseconds()

	ok := direct.Verdict == VerdictOK

	if !ok && c.proxyAddr != "" {
		proxyResult := c.CheckViaProxy(t.URL)
		if proxyResult != nil && proxyResult.Verdict == VerdictOK {
			ok = true
			direct.Verdict = VerdictOK
			direct.Notes = append(direct.Notes, "доступен через прокси")
		}
	}

	reason := ""
	if !ok {
		reason = direct.Verdict
		if len(direct.Notes) > 0 {
			reason = direct.Notes[0]
		}
	}

	return BulkResult{
		Name:      t.Name,
		URL:       t.URL,
		OK:        ok,
		Blocked:   !ok,
		Verdict:   direct.Verdict,
		LatencyMs: latency,
		Reason:    reason,
		TCP:       direct.TCP,
		TLS:       direct.TLS,
		HTTP:      direct.HTTP,
	}
}
