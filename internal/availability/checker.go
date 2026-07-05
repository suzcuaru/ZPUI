package availability

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"zpui/internal/logger"
)

type Result struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	OK        bool   `json:"ok"`
	Reason    string `json:"reason,omitempty"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Method    string `json:"method,omitempty"`
}

type CheckSet struct {
	Default []Result `json:"default"`
	User    []Result `json:"user"`
}

type Checker struct {
	log *logger.Logger
}

func NewChecker(log *logger.Logger) *Checker {
	return &Checker{log: log}
}

type Target struct {
	Host string
	URL  string
}

func (c *Checker) Check(defaultTargets []Target, userHosts []string) *CheckSet {
	dialer := &net.Dialer{Timeout: 3 * time.Second}
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext:     dialer.DialContext,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	checkOne := func(t Target) Result {
		start := time.Now()
		resp, err := client.Get(t.URL)
		latency := time.Since(start).Milliseconds()

		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return Result{Name: t.Host, URL: t.URL, OK: true, LatencyMs: latency, Method: "HTTP " + resp.Status}
			}
			httpReason := fmt.Sprintf("HTTP %d", resp.StatusCode)
			if ok := tcpFallback(t.Host); ok {
				return Result{Name: t.Host, URL: t.URL, OK: true, LatencyMs: latency, Method: "TCP", Reason: httpReason + " → TCP ok"}
			}
			return Result{Name: t.Host, URL: t.URL, OK: false, Reason: httpReason, LatencyMs: latency, Method: "HTTP"}
		}

		errMsg := classifyError(err)

		if ok := tcpFallback(t.Host); ok {
			return Result{Name: t.Host, URL: t.URL, OK: true, LatencyMs: latency, Method: "TCP", Reason: errMsg + " → TCP ok"}
		}

		return Result{Name: t.Host, URL: t.URL, OK: false, Reason: errMsg, LatencyMs: latency, Method: "HTTP"}
	}

	runBatch := func(targets []Target) []Result {
		results := make([]Result, len(targets))
		var wg sync.WaitGroup
		for i, t := range targets {
			wg.Add(1)
			go func(idx int, tgt Target) {
				defer wg.Done()
				results[idx] = checkOne(tgt)
			}(i, t)
		}
		wg.Wait()
		return results
	}

	defaultResults := runBatch(defaultTargets)
	userResults := runBatch(userHostsToTargets(userHosts))

	cs := &CheckSet{Default: defaultResults, User: userResults}
	c.logResults(cs)
	return cs
}

func (c *Checker) logResults(cs *CheckSet) {
	var failed []Result
	for _, r := range cs.Default {
		if !r.OK {
			failed = append(failed, r)
		}
	}
	for _, r := range cs.User {
		if !r.OK {
			failed = append(failed, r)
		}
	}

	total := len(cs.Default) + len(cs.User)
	okCount := total - len(failed)

	if len(failed) > 0 {
		c.log.Warn("availability", fmt.Sprintf("Check: %d/%d ok, %d failed", okCount, total, len(failed)))
		for _, f := range failed {
			c.log.Warn("availability", fmt.Sprintf("  ✗ %s — %s (%s)", f.Name, f.Reason, f.Method))
		}
	} else {
		c.log.Info("availability", fmt.Sprintf("Check: %d/%d ok", okCount, total))
	}

	for _, r := range cs.Default {
		if r.OK && c.log.IsDebug("availability") {
			c.log.Debug("availability", fmt.Sprintf("  ✓ %s — %s (%dms)", r.Name, r.Method, r.LatencyMs))
		}
	}
	for _, r := range cs.User {
		if r.OK && c.log.IsDebug("availability") {
			c.log.Debug("availability", fmt.Sprintf("  ✓ %s — %s (%dms)", r.Name, r.Method, r.LatencyMs))
		}
	}
}

func tcpFallback(host string) bool {
	conn, err := net.DialTimeout("tcp", host+":443", 3*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func classifyError(err error) string {
	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline"):
		return "timeout"
	case strings.Contains(errStr, "no such host") || strings.Contains(errStr, "DNS"):
		return "DNS resolution failed"
	case strings.Contains(errStr, "connection refused"):
		return "connection refused"
	case strings.Contains(errStr, "connection reset"):
		return "connection reset"
	case strings.Contains(errStr, "tls:") || strings.Contains(errStr, "certificate"):
		return "TLS error"
	default:
		if len(errStr) > 80 {
			errStr = errStr[:80]
		}
		return errStr
	}
}

func userHostsToTargets(hosts []string) []Target {
	var targets []Target
	for _, h := range hosts {
		url := h
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			url = "https://" + url
		}
		host := strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://")
		if idx := strings.IndexAny(host, "/:"); idx > 0 {
			host = host[:idx]
		}
		targets = append(targets, Target{Host: host, URL: url})
	}
	return targets
}
