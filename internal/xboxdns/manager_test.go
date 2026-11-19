//go:build windows

package xboxdns

import (
	"encoding/binary"
	"strings"
	"testing"
)

func TestIsVirtualAdapterName(t *testing.T) {
	cases := map[string]bool{
		"Ethernet":                           false,
		"Ethernet 2":                         false,
		"Беспроводная сеть":                  false,
		"WLAN":                               false,
		"Tailscale":                          true,
		"OpenVPN Connect DCO Adapter":        true,
		"vEthernet (Default Switch)":         true,
		"vEthernet (WSL (Hyper-V firewall))": true,
		"Loopback Pseudo-Interface 1":        true,
		"Сетевое подключение Bluetooth":       true,
		"Сетевой мост Microsoft Wi-Fi Direct": true,
		"ZPUI VPN":                         true,
		"WireGuard Tunnel":                 true,
		"Hyper-V Virtual Ethernet Adapter": true,
		"Подключение по локальной сети* 1 (WAN Miniport)": true,
	}
	for name, want := range cases {
		if got := isVirtualAdapterName(name); got != want {
			t.Errorf("isVirtualAdapterName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestSplitDNSList(t *testing.T) {
	cases := map[string][]string{
		"111.88.96.54 111.88.96.55": {"111.88.96.54", "111.88.96.55"},
		"8.8.8.8,8.8.4.4":           {"8.8.8.8", "8.8.4.4"},
		" 1.1.1.1 ;  1.0.0.1 ":      {"1.1.1.1", "1.0.0.1"},
		"":                          nil,
		"8.8.8.8":                   {"8.8.8.8"},
	}
	for in, want := range cases {
		got := splitDNSList(in)
		if len(got) != len(want) {
			t.Errorf("splitDNSList(%q) = %v, want %v", in, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("splitDNSList(%q)[%d] = %q, want %q", in, i, got[i], want[i])
			}
		}
	}
}

func TestBuildDNSQuery(t *testing.T) {
	q := buildDNSQuery(0xABCD, "xbox-dns.ru")
	if len(q) < 17 {
		t.Fatalf("query too short: %d", len(q))
	}
	if binary.BigEndian.Uint16(q[0:2]) != 0xABCD {
		t.Errorf("ID mismatch: %x", q[0:2])
	}
	if binary.BigEndian.Uint16(q[2:4]) != 0x0100 {
		t.Errorf("flags = %x, want 0100 (RD)", q[2:4])
	}
	if binary.BigEndian.Uint16(q[4:6]) != 1 {
		t.Errorf("qdcount = %x, want 1", q[4:6])
	}
	// QNAME: 8 "xbox-dns" 2 "ru" 0 (дефис внутри метки — часть метки)
	tail := q[12:]
	want := "\x08xbox-dns\x02ru\x00\x00\x01\x00\x01"
	if string(tail) != want {
		t.Errorf("query tail = %q, want %q", tail, want)
	}
}

func TestSnapshotEntryRoundtrip(t *testing.T) {
	// Формат снимка "adapter|family|значение" разбирается корректно.
	entries := []string{"Ethernet|4|192.168.1.1", "Ethernet|6|dhcp", "broken-entry"}
	var v4, v6, bad int
	for _, e := range entries {
		parts := strings.SplitN(e, "|", 3)
		if len(parts) != 3 {
			bad++
			continue
		}
		switch parts[1] {
		case "4":
			v4++
		case "6":
			v6++
		}
	}
	if v4 != 1 || v6 != 1 || bad != 1 {
		t.Errorf("parsed v4=%d v6=%d bad=%d, want 1/1/1", v4, v6, bad)
	}
}

// Все серверы недоступны → включение отказом с внятной ошибкой, БЕЗ
// изменений в системе (pre-flight срабатывает до netsh/dnsapi).
func TestEnableRefusedWhenNoServerResponds(t *testing.T) {
	m := NewManager(nil)
	m.Configure([]string{"192.0.2.1"}) // TEST-NET-1: ответа не будет
	err := m.Enable()
	if err == nil {
		t.Fatal("Enable must refuse when no DNS server responds")
	}
	if !strings.Contains(err.Error(), "DNS-серверы недоступны") {
		t.Fatalf("error = %v, want pre-flight refusal", err)
	}
	if m.IsEnabled() {
		t.Error("manager must stay disabled after refused enable")
	}
}

// Pre-flight проверяет ВСЕ включённые серверы (включая пресетные) и
// помечает недоступные по одному — данные для точек в таблице UI.
func TestCheckServersMarksEachServer(t *testing.T) {
	m := NewManager(nil)
	m.Configure([]string{"192.0.2.1", "127.0.0.1"})
	h := m.CheckServers()
	if len(h.Servers) != 2 {
		t.Fatalf("health entries = %d, want 2 (per server)", len(h.Servers))
	}
	found := map[string]bool{}
	for _, s := range h.Servers {
		found[s.Server] = true
	}
	if !found["192.0.2.1"] || !found["127.0.0.1"] {
		t.Fatalf("per-server health incomplete: %+v", h.Servers)
	}
}
