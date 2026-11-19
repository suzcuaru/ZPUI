package app

import (
	"strings"
	"sync"
	"time"


	"zpui/internal/database"
	"zpui/internal/power"
)


// ============================================================
// DEVICE TRACKER (background workers)
// ============================================================

// startDeviceTracker — периодическое обновление устройств из ARP-таблицы.
// Интервал 60с (в экономном режиме 300с): каждый тик — процесс arp -a и
// DNS-запросы, десятисекундный цикл не давал сети и диску покоя.
func (a *App) startDeviceTracker() {
	a.updateDevicesFromNetwork()

	for {
		interval := power.Stretch(60 * time.Second)
		select {
		case <-time.After(interval):
			// Экономный режим уровня 3: ARP/DNS-сканирование приостановлено.
			if power.ServicesPaused() {
				continue
			}
			a.updateDevicesFromNetwork()
		case <-a.stopCh:
			return
		}
	}
}

// ptrCache — кэш обратного DNS (IP → имя): у большинства LAN-хостов PTR
// отсутствует, и повторять медленные таймаутящие запросы каждый тик незачем.
var ptrCache sync.Map

func resolveHostnameCached(ip string) string {
	if v, ok := ptrCache.Load(ip); ok {
		return v.(string)
	}
	name := resolveHostname(ip)
	ptrCache.Store(ip, name)
	return name
}

func (a *App) updateDevicesFromNetwork() {
	arp := getARPTable()

	for ip, mac := range arp {
		mac = strings.ReplaceAll(strings.ToLower(mac), "-", ":")
		if mac == "" || len(mac) < 17 {
			continue
		}

		hostname := resolveHostnameCached(ip)

		now := time.Now()
		device := &database.SessionDevice{
			MAC:      mac,
			IP:       ip,
			Hostname: hostname,
			LastSeen: now,
			IsOnline: true,
		}

		existing, _ := database.GetDeviceByMAC(mac)
		if existing != nil {
			device.ID = existing.ID
			device.FirstSeen = existing.FirstSeen
			device.TotalDL = existing.TotalDL
			device.TotalUL = existing.TotalUL
		} else {
			device.FirstSeen = now
		}

		database.UpsertDevice(device)
	}

	// Помечаем устройства не в ARP как офлайн
	devices, _ := database.GetAllDevices()
	for _, d := range devices {
		found := false
		for _, mac := range arp {
			normalMac := strings.ReplaceAll(strings.ToLower(mac), "-", ":")
			if normalMac == d.MAC {
				found = true
				break
			}
		}
		if !found && d.IsOnline {
			database.UpsertDevice(&database.SessionDevice{
				ID:        d.ID,
				MAC:       d.MAC,
				IP:        d.IP,
				Hostname:  d.Hostname,
				FirstSeen: d.FirstSeen,
				LastSeen:  d.LastSeen,
				TotalDL:   d.TotalDL,
				TotalUL:   d.TotalUL,
				IsOnline:  false,
			})
		}
	}
}
