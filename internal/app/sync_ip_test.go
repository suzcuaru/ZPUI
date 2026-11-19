package app

import (
	"net"
	"regexp"
	"testing"
)

// TestRouteInfo: адрес выбирается по маршруту по умолчанию и не является
// loopback; MAC — валидный формат, если получен.
func TestRouteInfo(t *testing.T) {
	ip, mac := routeInfo()
	if ip == "" {
		t.Skip("нет внешнего маршрута (оффлайн) — fallback путь")
	}
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.To4() == nil {
		t.Fatalf("routeInfo вернул не IPv4: %q", ip)
	}
	if parsed.IsLoopback() {
		t.Fatalf("routeInfo вернул loopback: %q", ip)
	}
	if mac != "" {
		if !regexp.MustCompile(`^([0-9a-f]{2}:){5}[0-9a-f]{2}$`).MatchString(mac) {
			t.Fatalf("неверный формат MAC: %q", mac)
		}
	}
}
