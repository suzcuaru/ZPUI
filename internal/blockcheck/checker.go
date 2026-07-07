package blockcheck

import (
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
	checkTCP   bool
	checkTLS   bool
	checkHTTP  bool
	timeout    time.Duration
	httpClient *http.Client
}

func NewChecker(checkTCP, checkTLS, checkHTTP bool, timeoutSec int) *Checker {
	if timeoutSec <= 0 {
		timeoutSec = 8
	}
	t := time.Duration(timeoutSec) * time.Second

	tr := &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		DialContext:       (&net.Dialer{Timeout: t}).DialContext,
		DisableKeepAlives: true,
	}

	return &Checker{
		checkTCP:  checkTCP,
		checkTLS:  checkTLS,
		checkHTTP: checkHTTP,
		timeout:   t,
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
}

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

	port := 443
	if strings.HasPrefix(fullURL, "http://") {
		port = 80
	}

	if c.checkTCP {
		result.TCP = c.checkTCPConn(host, port)
		if !result.TCP.Ok {
			c.classify(&result)
			return result
		}
	}

	if c.checkTLS && port == 443 {
		result.TLS = c.checkTLSConn(host)
		if !result.TLS.Ok {
			c.classify(&result)
			return result
		}
	}

	if c.checkHTTP {
		c.checkHTTPGet(fullURL, &result)
	}

	c.classify(&result)
	return result
}

func (c *Checker) checkTCPConn(host string, port int) LayerResult {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, c.timeout)
	elapsed := time.Since(start).Seconds() * 1000
	if err != nil {
		return LayerResult{Ok: false, TimeMs: elapsed, Error: classifyErr(err)}
	}
	conn.Close()
	return LayerResult{Ok: true, TimeMs: elapsed}
}

func (c *Checker) checkTLSConn(host string) LayerResult {
	addr := net.JoinHostPort(host, "443")
	start := time.Now()
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: c.timeout},
		"tcp",
		addr,
		&tls.Config{InsecureSkipVerify: true, ServerName: host},
	)
	elapsed := time.Since(start).Seconds() * 1000
	if err != nil {
		return LayerResult{Ok: false, TimeMs: elapsed, Error: classifyErr(err)}
	}
	conn.Close()
	return LayerResult{Ok: true, TimeMs: elapsed}
}

func (c *Checker) checkHTTPGet(fullURL string, result *CheckResult) {
	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		result.HTTP = LayerResult{Ok: false, Error: err.Error()}
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
		result.HTTP.Error = classifyErr(err)
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
	if c.checkTCP && !result.TCP.Ok {
		if result.TCP.Error == "dns_failure" {
			result.Verdict = VerdictDNSBlock
			result.Confidence = ConfHigh
			result.Notes = append(result.Notes, "DNS не резолвится")
			return
		}
		if isTimeout(result.TCP.Error) {
			result.Verdict = VerdictTimeout
			result.Confidence = ConfMedium
		} else {
			result.Verdict = VerdictTCPBlock
			result.Confidence = ConfHigh
		}
		result.Notes = append(result.Notes, "TCP: "+result.TCP.Error)
		return
	}

	if c.checkTLS && !result.TLS.Ok {
		if result.TLS.Error == "dns_failure" {
			result.Verdict = VerdictDNSBlock
			result.Confidence = ConfHigh
			result.Notes = append(result.Notes, "DNS не резолвится — блокировка по DNS или нет записи")
			return
		}
		if isTimeout(result.TLS.Error) {
			result.Verdict = VerdictTimeout
			result.Confidence = ConfMedium
			result.Notes = append(result.Notes, "TLS таймаут")
		} else {
			result.Verdict = VerdictTLSBlock
			result.Confidence = ConfHigh
			result.Notes = append(result.Notes, "TLS: "+result.TLS.Error+" — DPI сбрасывает handshake")
		}
		return
	}

	if c.checkHTTP {
		if result.HTTP.StubPage {
			result.Verdict = VerdictHTTPStub
			result.Confidence = ConfHigh
			if result.HTTP.Status == 451 {
				result.Notes = append(result.Notes, "HTTP 451 — юридически недоступен")
			} else {
				result.Notes = append(result.Notes, "заглушка РКН/TSPU")
			}
			return
		}
		if result.HTTP.Ok {
			result.Verdict = VerdictOK
			result.Confidence = ConfHigh
			result.Notes = append(result.Notes, "ресурс доступен")
			return
		}
		if isTimeout(result.HTTP.Error) {
			result.Verdict = VerdictTimeout
			result.Confidence = ConfMedium
			result.Notes = append(result.Notes, "HTTP таймаут")
			return
		}
	}

	result.Verdict = VerdictDown
	result.Confidence = ConfMedium
	if result.HTTP.Error != "" {
		result.Notes = append(result.Notes, result.HTTP.Error)
	} else {
		result.Notes = append(result.Notes, "недоступен")
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

func classifyErr(err error) string {
	if err == nil {
		return ""
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "timeout") || strings.Contains(s, "deadline"):
		return "timeout"
	case strings.Contains(s, "connection refused"):
		return "connection_refused"
	case strings.Contains(s, "reset") || strings.Contains(s, "broken pipe"):
		return "connection_reset"
	case strings.Contains(s, "no such host"):
		return "dns_failure"
	case strings.Contains(s, "eof"):
		return "eof"
	case strings.Contains(s, "tls") || strings.Contains(s, "handshake") || strings.Contains(s, "certificate"):
		return "tls_error"
	default:
		return s
	}
}

func isTimeout(errStr string) bool {
	return errStr == "timeout"
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
	for _, marker := range stubMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
