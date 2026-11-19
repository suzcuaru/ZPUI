package zsvc

// DNS-надзор службы: пока приложение закрыто, наш системный DNS не должен
// пропадать. Цикл (раз в 5 минут) читает config.json установки: если DNS
// включён, а на первичном адаптере его фактически нет (сняли вручную или
// сторонним ПО) — переприменяет адреса. Конфиг служба только ЧИТАЕТ: все
// решения (включение/выключение, авто-отключение мёртвых источников)
// остаются за приложением, чтобы не было двух писателей config.json.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"zpui/internal/config"
	"zpui/internal/xboxdns"
)

const (
	dnsSupervisorFirstDelay = 30 * time.Second
	dnsSupervisorInterval   = 5 * time.Minute
)

// dnsLogAdapter — журнал менеджера DNS в служебный лог службы.
type dnsLogAdapter struct{ l *SvcLog }

func (a dnsLogAdapter) Info(category, msg string) { a.l.Info("dns: " + msg) }
func (a dnsLogAdapter) Warn(category, msg string) { a.l.Info("dns warn: " + msg) }

// dnsSupervisor — цикл надзора за применённым DNS.
func (s *Service) dnsSupervisor(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(dnsSupervisorFirstDelay):
	}
	t := time.NewTicker(dnsSupervisorInterval)
	defer t.Stop()
	for {
		s.dnsCheckOnce()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// dnsCheckOnce — один проход надзора: сверить факт с конфигом и починить.
func (s *Service) dnsCheckOnce() {
	cfg := LoadConfig()
	if cfg.AppPath == "" {
		return
	}
	// Приложение живо — надзор приложения сам чинит набор (и владеет
	// config.json); служба не дублирует работу при живом приложении.
	if pid, _, active := s.wd.Snapshot(); active && appProcessAlive(pid) {
		return
	}
	dir := filepath.Dir(cfg.AppPath)
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		return
	}
	var ac struct {
		XboxDns config.XboxDnsConfig `json:"xbox_dns"`
	}
	if json.Unmarshal(data, &ac) != nil || !ac.XboxDns.Enabled {
		return
	}
	ips := ac.XboxDns.PlainIPs()
	if len(ips) == 0 {
		return
	}
	m := xboxdns.NewManager(dnsLogAdapter{s.log})
	m.Configure(ips)
	if m.IsSetInSystem() {
		return
	}
	s.log.Info("dns: адреса пропали с адаптера (приложение не запущено) — восстанавливаем")
	if err := m.Repair(); err != nil {
		s.log.Error("dns repair: " + err.Error())
		return
	}
	s.log.Info("dns: адреса восстановлены службой")
}
