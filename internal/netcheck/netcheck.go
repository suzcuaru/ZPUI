// Package netcheck — быстрая проверка наличия выхода в интернет.
// Зонды — TCP-подключения к anycast-DNS по IP (без DNS-резолва, независимо
// от запрета и доменных блокировок): сеть есть — первый же зонд отвечает
// за десятки миллисекунд, сети нет — все три не отвечают.
package netcheck

import (
	"net"
	"time"
)

var probes = []string{
	"77.88.8.8:53", // Яндекс DNS
	"8.8.8.8:53",   // Google DNS
	"1.1.1.1:53",   // Cloudflare DNS
}

const probeTimeout = 2 * time.Second

// Online отвечает «есть ли интернет»: хотя бы один зонд подключился.
// Все промахнулись — сети нет (пробуждение ноутбука, отвалившийся Wi-Fi,
// перезагружающийся роутер).
func Online() bool {
	results := make(chan bool, len(probes))
	for _, addr := range probes {
		go func(addr string) {
			c, err := net.DialTimeout("tcp", addr, probeTimeout)
			if err == nil {
				c.Close()
			}
			results <- err == nil
		}(addr)
	}
	for range probes {
		if <-results {
			return true
		}
	}
	return false
}
