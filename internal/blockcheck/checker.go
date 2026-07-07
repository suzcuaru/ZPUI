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
	"os"
	"strings"
	"time"
)

type Checker struct {
	timeout     time.Duration
	dohClient   *http.Client
	httpClient  *http.Client
	proxyAddr   string
}

func NewChecker(timeoutSec int, proxyAddr string) *Checker {
	if timeoutSec <= 0 {
		timeoutSec = 5
	}
	t := time.Duration(timeoutSec) * time.Second

	tr := &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		DialContext:       (&net.Dialer{Timeout: t}).DialContext,
		DisableKeepAlives: true,
	}

	c := &Checker{
		timeout:   t,
		proxyAddr: proxyAddr,
		dohClient: &http.Client{Timeout: t, Transport: tr},
	}

	c.httpClient = &http.Client{Timeout: t, Transport: tr, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return fmt.Errorf("redirects disabled")
	}}

	return c
}

func (c *Checker) Check(rawURL string) CheckResult {
	parsed, host, fullURL := parseURL(rawURL)
	_ = parsed
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

	c.checkDNS(host, &result)

	port := "443"
	if strings.HasPrefix(fullURL, "http://") {
		port = "80"
	}

	if result.DNS.Ok || result.DNSDoH.Ok {
		ip := pickIP(result.DNS.IPs, result.DNSDoH.IPs)
		if ip != "" {
			c.checkTCP(ip, port, &result)
			if result.TCP.Ok {
				if port == "443" {
					c.checkTLS(host, ip, &result)
					if result.TLS.Ok {
						c.checkHTTP(fullURL, host, &result, false)
					}
				} else {
					c.checkHTTP(fullURL, host, &result, false)
				}
			}
		}
	}

	c.classify(&result)
	return result
}

func (c *Checker) CheckViaProxy(rawURL string) *CheckResult {
	if c.proxyAddr == "" {
		return nil
	}

	parsed, host, fullURL := parseURL(rawURL)
	_ = parsed
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
		DialContext: socksDialer,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	proxyClient := &http.Client{
		Timeout:   c.timeout,
		Transport: proxyTransport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
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

	result.HTTP.Ok = resp.StatusCode < 500
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

func (c *Checker) checkDNS(host string, result *CheckResult) {
	sysStart := time.Now()
	sysIPs, err := net.LookupIP(host)
	result.DNS.TimeMs = time.Since(sysStart).Seconds() * 1000
	if err != nil {
		result.DNS.Ok = false
		result.DNS.Error = err.Error()
	} else {
		result.DNS.IPs = ipStrings(sysIPs)
		result.DNS.Ok = len(result.DNS.IPs) > 0
	}

	dohIPs, err := c.dohResolve(host)
	if err != nil {
		result.DNSDoH.Ok = false
		result.DNSDoH.Error = err.Error()
	} else {
		result.DNSDoH.IPs = dohIPs
		result.DNSDoH.Ok = len(dohIPs) > 0
	}

	result.DNSMismatch = isDNSMismatch(result.DNS.IPs, result.DNSDoH.IPs)
}

func (c *Checker) dohResolve(host string) ([]string, error) {
	req, _ := http.NewRequest("GET", "https://cloudflare-dns.com/dns-query?name="+url.QueryEscape(host)+"&type=A", nil)
	req.Header.Set("Accept", "application/dns-json")
	resp, err := c.dohClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var dohResp struct {
		Answer []struct {
			Data string `json:"data"`
			Type int    `json:"type"`
		} `json:"Answer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&dohResp); err != nil {
		return nil, err
	}

	var ips []string
	for _, a := range dohResp.Answer {
		if a.Type == 1 && a.Data != "" {
			ips = append(ips, a.Data)
		}
	}
	return ips, nil
}

func (c *Checker) checkTCP(ip, port string, result *CheckResult) {
	addr := net.JoinHostPort(ip, port)
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, c.timeout)
	result.TCP.TimeMs = time.Since(start).Seconds() * 1000
	if err != nil {
		result.TCP.Ok = false
		errStr := err.Error()
		if strings.Contains(errStr, "reset") || strings.Contains(errStr, "refused") {
			result.TCP.Error = "RST"
		} else {
			result.TCP.Error = errStr
		}
		return
	}
	conn.Close()
	result.TCP.Ok = true
}

func (c *Checker) checkTLS(host, ip string, result *CheckResult) {
	addr := net.JoinHostPort(ip, "443")
	dialer := &net.Dialer{Timeout: c.timeout}
	start := time.Now()
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: true,
	})
	result.TLS.TimeMs = time.Since(start).Seconds() * 1000
	if err != nil {
		result.TLS.Ok = false
		errStr := err.Error()
		if strings.Contains(errStr, "reset") || strings.Contains(errStr, "EOF") || strings.Contains(errStr, "broken pipe") {
			result.TLS.Error = "reset_after_clienthello"
			result.TLS.Detail = "TLS handshake killed after ClientHello — SNI-based DPI"
		} else if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline") {
			result.TLS.Error = "timeout"
			result.TLS.Detail = "TLS handshake timed out — possible DPI silent drop"
		} else {
			result.TLS.Error = errStr
		}
		return
	}
	defer conn.Close()

	state := conn.ConnectionState()
	result.TLS.Ok = true
	if len(state.PeerCertificates) > 0 {
		result.TLS.CertCN = state.PeerCertificates[0].Subject.CommonName
	}
}

func (c *Checker) checkHTTP(fullURL, host string, result *CheckResult, viaProxy bool) {
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
		result.HTTP.Error = err.Error()
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	result.HTTP.Status = resp.StatusCode
	result.HTTP.Ok = resp.StatusCode < 500
	result.HTTP.StubPage = detectStubPage(body)

	if resp.StatusCode == 451 {
		result.HTTP.Ok = false
		result.HTTP.StubPage = true
	}
}

func (c *Checker) classify(result *CheckResult) {
	if !result.DNS.Ok && result.DNSDoH.Ok {
		result.Verdict = VerdictDNSBlock
		result.Confidence = ConfHigh
		result.Notes = append(result.Notes, "системный DNS не резолвится, DoH резолвится — отравление DNS")
		return
	}

	if result.DNSMismatch {
		result.Verdict = VerdictDNSBlock
		result.Confidence = ConfHigh
		result.Notes = append(result.Notes, "адреса системного DNS и DoH полностью не совпадают — подмена DNS")
		return
	}

	if result.TCP.Error == "RST" {
		result.Verdict = VerdictTCPReset
		result.Confidence = ConfHigh
		result.Notes = append(result.Notes, "TCP handshake отклонён RST — блэкхолинг на IP-уровне")
		return
	}

	if !result.TCP.Ok && result.DNS.Ok {
		if strings.Contains(result.TCP.Error, "timeout") {
			result.Verdict = VerdictTimeout
			result.Confidence = ConfLow
			result.Notes = append(result.Notes, "TCP handshake таймаут")
		} else {
			result.Verdict = VerdictDown
			result.Confidence = ConfMedium
		}
		return
	}

	if !result.TLS.Ok && result.TCP.Ok {
		result.Verdict = VerdictTLSBlock
		result.Confidence = ConfMedium
		if result.TLS.Detail != "" {
			result.Notes = append(result.Notes, result.TLS.Detail)
		} else {
			result.Notes = append(result.Notes, "TCP работает, TLS убит — DPI по SNI")
		}
		return
	}

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

	if !result.HTTP.Ok && result.TLS.Ok {
		result.Verdict = VerdictDown
		result.Confidence = ConfMedium
		result.Notes = append(result.Notes, fmt.Sprintf("HTTP статус %d", result.HTTP.Status))
		return
	}

	if result.HTTP.Ok && result.TLS.Ok {
		result.Verdict = VerdictOK
		result.Confidence = ConfHigh
		result.Notes = append(result.Notes, "ресурс доступен")
		return
	}

	result.Verdict = VerdictUnknown
	result.Confidence = ConfLow
	if len(result.Notes) == 0 {
		result.Notes = append(result.Notes, "не удалось классифицировать")
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

	// Try multiple provider APIs
	providers := []struct {
		url string
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
		Query    string `json:"query"`
		City     string `json:"city"`
		Country  string `json:"country"`
		Org      string `json:"org"`
		Isp      string `json:"isp"`
		As       string `json:"as"`
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

func ipStrings(ips []net.IP) []string {
	var result []string
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			result = append(result, v4.String())
		}
	}
	return result
}

func pickIP(a, b []string) string {
	if len(a) > 0 {
		return a[0]
	}
	if len(b) > 0 {
		return b[0]
	}
	return ""
}

func isDNSMismatch(sysIPs, dohIPs []string) bool {
	if len(sysIPs) == 0 || len(dohIPs) == 0 {
		return false
	}
	sysSet := make(map[string]bool)
	for _, ip := range sysIPs {
		sysSet[ip] = true
	}
	for _, ip := range dohIPs {
		if sysSet[ip] {
			return false
		}
	}
	return true
}

func detectStubPage(body []byte) bool {
	text := strings.ToLower(string(body))
	for _, marker := range stubMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
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

var _ = os.Getenv
