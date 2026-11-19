package app

// Мониторинг доступности интернета: несколько независимых целей (Cloudflare,
// Google, Яндекс DNS и наш сервер) — TCP-дозвон. Онлайн = ответила хотя бы
// одна цель: если один сервис лёг, доступ проверяется по остальным.
// Переходы офлайн↔онлайн — системные уведомления Windows.

import (
	"net"
	"time"

	"zpui/internal/notify"
	"zpui/internal/power"
)

type netTarget struct {
	addr string
}

var netWatchTargets = []string{
	"1.1.1.1:443",      // Cloudflare
	"8.8.8.8:443",      // Google
	"77.88.8.8:443",    // Яндекс (DNS)
	"zpui-life.ru:443", // наш сервер
	"94.140.14.14:443", // AdGuard
}

// netOnline — доступ в интернет есть, если отвечает хотя бы одна цель.
func netOnline() bool {
	for _, addr := range netWatchTargets {
		c, err := net.DialTimeout("tcp", addr, 3*time.Second)
		if err != nil {
			continue
		}
		c.Close()
		return true
	}
	return false
}

// StartNetWatch — фоновый контроль сети: переходы доступности сопровождаются
// системными уведомлениями. Стартовое состояние определяется без уведомления.
func (a *App) StartNetWatch() {
	a.supervisedGo("netwatch", func() {
		online := netOnline()
		if !online {
			a.log.Warn("netwatch", "start state: offline")
		}
		for {
			select {
			case <-a.stopCh:
				return
			case <-time.After(power.Stretch(30 * time.Second)):
			}
			// Экономный режим 3 ур. глушит фоновую активность — тик пропускаем.
			if power.ServicesPaused() {
				continue
			}
			now := netOnline()
			if now == online {
				continue
			}
			online = now
			if now {
				notify.Show("ZPUI · Сеть", "Доступ к интернету восстановлен")
				a.log.Info("netwatch", "network restored")
				a.dnsReapplyOnOnline()
			} else {
				notify.Show("ZPUI · Сеть", "Сеть недоступна — проверьте подключение")
				a.log.Warn("netwatch", "network unreachable")
			}
		}
	})
}
