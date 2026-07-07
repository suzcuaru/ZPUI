package blockcheck

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Checker struct {
	timeout    time.Duration
	dohClient  *http.Client
	httpClient *http.Client
	proxyAddr  string
}

func NewChecker(timeoutSec int, proxyAddr string) *Checker {
	if timeoutSec <= 0 {
		timeoutSec = 10
	}
	t := time.Duration(timeoutSec) * time.Second

	tr := &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		DialContext:       (&net.Dialer{Timeout: t}).DialContext,
		DisableKeepAlives: true,
	}

	c := &Checker{
		timeout:    t,
		proxyAddr:  proxyAddr,
		dohClient:  &http.Client{Timeout: t, Transport: tr},
		httpClient: &http.Client{
			Timeout:   t,
			Transport: tr,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
	}

	return c
}

// Check does a simple HTTP GET to the URL (following redirects).
// If HTTPS fails (TLS blocked by DPI), falls back to plain HTTP.
// A resource is considered OK if HTTP status < 400 and response is not a stub page.
func (c *Checker) Check(rawURL string) CheckResult {
	_, host, fullURL := parseURL(rawURL)
	result := CheckResult{
		URL:  rawURL,
		Host: host,
	}
	if fullURL == "" {
		result.Verdict = VerdictDown
		result.Confidence = ConfHigh
		result.Notes = []string{"неверный URL"}
		return result
	}

	c.checkHTTP(fullURL, host, &result)

	if !result.HTTP.Ok && isTLSFailure(result.HTTP.Error) && strings.HasPrefix(fullURL, "https://") {
		httpURL := "http://" + strings.TrimPrefix(fullURL, "https://")
		var httpResult CheckResult
		c.checkHTTP(httpURL, host, &httpResult)
		if httpResult.HTTP.Ok {
			result.HTTP = httpResult.HTTP
			result.Notes = append(result.Notes, "HTTPS заблокирован (TLS), но HTTP работает")
		}
	}

	c.classify(&result)
	return result
}

func isTLSFailure(err string) bool {
	if err == "" {
		return false
	}
	lower := strings.ToLower(err)
	return strings.Contains(lower, "tls") ||
		strings.Contains(lower, "handshake") ||
		strings.Contains(lower, "certificate") ||
		strings.Contains(lower, "ssl") ||
		strings.Contains(lower, "eof") ||
		strings.Contains(lower, "reset") ||
		strings.Contains(lower, "connection_reset")
}

func (c *Checker) CheckViaProxy(rawURL string) *CheckResult {
	if c.proxyAddr == "" {
		return nil
	}

	_, host, fullURL := parseURL(rawURL)
	if fullURL == "" {
		return nil
	}

	result := &CheckResult{
		URL:  rawURL,
		Host: host,
	}

	socksDialer, err := socks5Dialer(c.proxyAddr, c.timeout)
	if err != nil {
		result.Verdict = VerdictUnknown
		result.Notes = []string{"прокси недоступен: " + err.Error()}
		return result
	}

	proxyTransport := &http.Transport{
		DialContext:     socksDialer,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	proxyClient := &http.Client{
		Timeout:   c.timeout,
		Transport: proxyTransport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	start := time.Now()
	resp, err := proxyClient.Get(fullURL)
	elapsed := time.Since(start).Seconds() * 1000

	if err != nil {
		result.HTTP.Ok = false
		result.HTTP.Error = err.Error()
		result.Verdict = VerdictTimeout
		result.Confidence = ConfLow
		result.Notes = []string{"через обход: " + err.Error()}
		return result
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	result.HTTP.Ok = resp.StatusCode < 400
	result.HTTP.Status = resp.StatusCode
	result.HTTP.TimeMs = elapsed
	result.HTTP.StubPage = detectStubPage(body)

	if result.HTTP.Ok && !result.HTTP.StubPage {
		result.Verdict = VerdictOK
		result.Confidence = ConfHigh
	} else if result.HTTP.StubPage {
		result.Verdict = VerdictHTTPStub
		result.Confidence = ConfHigh
	} else {
		result.Verdict = VerdictDown
	}

	return result
}

func (c *Checker) checkHTTP(fullURL, host string, result *CheckResult) {
	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		result.HTTP.Ok = false
		result.HTTP.Error = err.Error()
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7")

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	result.HTTP.TimeMs = time.Since(start).Seconds() * 1000
	if err != nil {
		result.HTTP.Ok = false
		errStr := err.Error()
		if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline") {
			result.HTTP.Error = "timeout"
		} else if strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "reset") {
			result.HTTP.Error = "connection_reset"
		} else {
			result.HTTP.Error = errStr
		}
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	result.HTTP.Status = resp.StatusCode
	result.HTTP.Ok = resp.StatusCode < 400
	result.HTTP.StubPage = detectStubPage(body)

	if resp.StatusCode == 451 {
		result.HTTP.Ok = false
		result.HTTP.StubPage = true
	}
}

func (c *Checker) classify(result *CheckResult) {
	if result.HTTP.StubPage {
		result.Verdict = VerdictHTTPStub
		result.Confidence = ConfHigh
		if result.HTTP.Status == 451 {
			result.Notes = append(result.Notes, "HTTP 451 — юридически недоступен")
		} else {
			result.Notes = append(result.Notes, "тело ответа содержит маркер заглушки РКН/TSPU")
		}
		return
	}

	if result.HTTP.Error == "timeout" {
		result.Verdict = VerdictTimeout
		result.Confidence = ConfMedium
		result.Notes = append(result.Notes, "таймаут соединения")
		return
	}

	if result.HTTP.Error == "connection_reset" {
		result.Verdict = VerdictTCPReset
		result.Confidence = ConfHigh
		result.Notes = append(result.Notes, "соединение сброшено — DPI или блокировка по IP")
		return
	}

	if result.HTTP.Ok {
		result.Verdict = VerdictOK
		result.Confidence = ConfHigh
		result.Notes = append(result.Notes, "ресурс доступен")
		return
	}

	if result.HTTP.Status > 0 {
		result.Verdict = VerdictDown
		result.Confidence = ConfMedium
		result.Notes = append(result.Notes, fmt.Sprintf("HTTP статус %d", result.HTTP.Status))
		return
	}

	result.Verdict = VerdictDown
	result.Confidence = ConfMedium
	if result.HTTP.Error != "" {
		result.Notes = append(result.Notes, result.HTTP.Error)
	} else {
		result.Notes = append(result.Notes, "соединение не установлено")
	}
}

func (c *Checker) GetProviderInfo() ProviderInfo {
	info := ProviderInfo{}
	client := &http.Client{
		Timeout: c.timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			DialContext:     (&net.Dialer{Timeout: c.timeout}).DialContext,
		},
	}

	providers := []struct {
		url   string
		parse func(body []byte, info *ProviderInfo) bool
	}{
		{"https://ipinfo.io/json", parseIPInfo},
		{"https://ip-api.com/json/", parseIPAPI},
		{"https://myip.dev/json", parseMyIP},
	}

	for _, p := range providers {
		resp, err := client.Get(p.url)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		if p.parse(body, &info) && info.IP != "" {
			break
		}
	}

	return info
}

func parseIPInfo(body []byte, info *ProviderInfo) bool {
	var d struct {
		IP      string `json:"ip"`
		City    string `json:"city"`
		Country string `json:"country"`
		Org     string `json:"org"`
	}
	if err := json.Unmarshal(body, &d); err != nil || d.IP == "" {
		return false
	}
	info.IP = d.IP
	info.City = d.City
	info.Country = d.Country
	info.Org = d.Org
	parts := strings.SplitN(d.Org, " ", 2)
	if len(parts) >= 2 {
		info.ASN = parts[0]
		info.ISP = parts[1]
	} else {
		info.ISP = d.Org
	}
	return true
}

func parseIPAPI(body []byte, info *ProviderInfo) bool {
	var d struct {
		Query   string `json:"query"`
		City    string `json:"city"`
		Country string `json:"country"`
		Org     string `json:"org"`
		Isp     string `json:"isp"`
		As      string `json:"as"`
	}
	if err := json.Unmarshal(body, &d); err != nil || d.Query == "" {
		return false
	}
	info.IP = d.Query
	info.City = d.City
	info.Country = d.Country
	info.Org = d.Org
	info.ISP = d.Isp
	info.ASN = d.As
	return true
}

func parseMyIP(body []byte, info *ProviderInfo) bool {
	var d struct {
		IP      string `json:"ip"`
		City    string `json:"city"`
		Country string `json:"country"`
		Org     string `json:"asn"`
	}
	if err := json.Unmarshal(body, &d); err != nil || d.IP == "" {
		return false
	}
	info.IP = d.IP
	info.City = d.City
	info.Country = d.Country
	info.Org = d.Org
	parts := strings.SplitN(d.Org, " ", 2)
	if len(parts) >= 2 {
		info.ASN = parts[0]
		info.ISP = parts[1]
	} else {
		info.ISP = d.Org
	}
	return true
}

func parseURL(raw string) (parsed *url.URL, host, full string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, "", ""
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return nil, "", ""
	}
	host = parsed.Hostname()
	return parsed, host, raw
}

func detectStubPage(body []byte) bool {
	if len(body) > 8192 {
		return false
	}
	text := strings.ToLower(string(body))
	matches := 0
	for _, marker := range stubMarkers {
		if strings.Contains(text, marker) {
			matches++
		}
	}
	return matches >= 1
}

func socks5Dialer(proxyAddr string, timeout time.Duration) (func(ctx context.Context, network, addr string) (net.Conn, error), error) {
	proxyURL, err := url.Parse("socks5://" + proxyAddr)
	if err != nil {
		return nil, err
	}
	dialer, err := makeSocks5Dialer(proxyURL, timeout)
	if err != nil {
		return nil, err
	}
	return dialer.DialContext, nil
}
