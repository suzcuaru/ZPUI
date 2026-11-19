package ytcheck

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Проверка доступности YouTube на уровне DNS: zapret работает с уже
// установленным соединением (TLS/QUIC-фейки) и ничего не может сделать,
// если системный резолвер отдаёт отравленные или чёрно-холостые адреса
// youtube/googlevideo (поведение, замеченное в связке с zapret 1.10.x).
// Сравнение системного резолва с DoH и TCP/TLS-проба по полученным IP
// позволяет отличить «DNS-подмену» (лечится включением Xbox DNS) от
// других проблем.

// hosts — стабильные имена YouTube-инфраструктуры: точка входа и
// редиректор видеотрафика googlevideo.
var hosts = []string{"www.youtube.com", "redirector.googlevideo.com"}

type Result struct {
	OK          bool      `json:"ok"`
	DNSPoisoned bool      `json:"dns_poisoned"`
	Reason      string    `json:"reason,omitempty"`
	CheckedAt   time.Time `json:"checked_at"`
}

type dohResponse struct {
	Status int `json:"Status"`
	Answer []struct {
		Type int    `json:"type"`
		Data string `json:"data"`
	} `json:"Answer"`
}

var dohClient = &http.Client{Timeout: 6 * time.Second}

// Check выполняет полную проверку. Блокирующая, рассчитана на фоновый вызов.
func Check() Result {
	res := Result{CheckedAt: time.Now(), OK: true}

	poisoned := 0
	reachable := 0
	for _, host := range hosts {
		sysIPs := resolveSystem(host)
		dohIPs := resolveDoh(host)

		switch {
		case len(sysIPs) == 0 && len(dohIPs) > 0:
			// Системный резолвер молчит/отдаёт пусто, DoH отвечает — подмена.
			poisoned++
		case len(sysIPs) > 0 && len(dohIPs) > 0:
			sysOK := tlsReachable(sysIPs[0], host)
			if sysOK {
				reachable++
				continue
			}
			// Системный IP мёртв, а IP от DoH жив — резолв подменён.
			if tlsReachable(dohIPs[0], host) {
				poisoned++
			}
		case len(sysIPs) > 0:
			if tlsReachable(sysIPs[0], host) {
				reachable++
			}
		}
	}

	if poisoned == len(hosts) {
		res.OK = false
		res.DNSPoisoned = true
		res.Reason = "все YouTube-хосты резолвятся системным DNS в нерабочие адреса"
	} else if reachable == 0 && poisoned > 0 {
		res.OK = false
		res.DNSPoisoned = true
		res.Reason = "часть YouTube-хостов получает нерабочие адреса из системного DNS"
	} else if reachable == 0 {
		res.OK = false
		res.Reason = "YouTube-хосты недоступны по TLS"
	}
	return res
}

func resolveSystem(host string) []string {
	ips, err := net.LookupHost(host)
	if err != nil {
		return nil
	}
	return ips
}

// resolveDoh спрашивает DoH-резолверы (JSON API) напрямую, минуя системный DNS.
func resolveDoh(host string) []string {
	endpoints := []string{
		"https://dns.google/resolve?name=" + host + "&type=A",
		"https://cloudflare-dns.com/dns-query?name=" + host + "&type=A",
	}
	for _, ep := range endpoints {
		req, err := http.NewRequest("GET", ep, nil)
		if err != nil {
			continue
		}
		req.Header.Set("Accept", "application/dns-json")
		resp, err := dohClient.Do(req)
		if err != nil {
			continue
		}
		var dr dohResponse
		err = json.NewDecoder(resp.Body).Decode(&dr)
		resp.Body.Close()
		if err != nil || dr.Status != 0 {
			continue
		}
		var ips []string
		for _, a := range dr.Answer {
			if a.Type == 1 && a.Data != "" {
				ips = append(ips, a.Data)
			}
		}
		if len(ips) > 0 {
			return ips
		}
	}
	return nil
}

// tlsReachable — TCP+TLS-рукопожатие с заданным IP и SNI хоста.
func tlsReachable(ip, host string) bool {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(ip, "443"), &tls.Config{
		ServerName: host,
	})
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (r Result) String() string {
	if r.OK {
		return "ok"
	}
	return fmt.Sprintf("failed: %s", r.Reason)
}
