package app

import (
	"reflect"
	"testing"
)

func TestVpnAdaptersFromOutput(t *testing.T) {
	// Своего туннеля у приложения больше нет — «ZPUI VPN» в системе выглядит
	// как обычный сторонний WireGuard-адаптер и в findings попадает.
	out := "ZPUI VPN|WireGuard Tunnel\n\nEthernet 2|WireGuard Tunnel\nEthernet|Realtek PCIe GbE Family\nTailscale|Tailscale Tunnel\nmalformed line\nOpenVPN Wintun|OpenVPN TAP-Windows6"
	got := vpnAdaptersFromOutput(out)
	want := []string{"WireGuard Tunnel", "WireGuard Tunnel", "OpenVPN TAP-Windows6"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("vpnAdaptersFromOutput() = %v, want %v", got, want)
	}
}

func TestVpnFindingFilters(t *testing.T) {
	benign := []string{"Radmin VPN", "radminvpn.exe", "Hamachi Network Interface", "LogMeIn Hamachi", "ZeroTierOneService", "Tailscale Tunnel", "TeamViewer VPN", " Tunngle "}
	for _, name := range benign {
		if !isBenignVpn(name) {
			t.Errorf("isBenignVpn(%q) = false, want true (whitelist)", name)
		}
	}
	notBenign := []string{"", "WireGuard Tunnel", "NordLynx", "OpenVPN TAP-Windows6"}
	for _, name := range notBenign {
		if isBenignVpn(name) {
			t.Errorf("isBenignVpn(%q) = true, want false", name)
		}
	}

	ignore := []string{"WireGuard Tunnel"}
	ignored := []string{"wireguard tunnel", " WireGuard Tunnel "}
	for _, name := range ignored {
		if !isIgnoredVpn(name, ignore) {
			t.Errorf("isIgnoredVpn(%q) = false, want true (case-insensitive exact)", name)
		}
	}
	notIgnored := []string{"", "WireGuard", "OpenVPN TAP-Windows6"}
	for _, name := range notIgnored {
		if isIgnoredVpn(name, ignore) {
			t.Errorf("isIgnoredVpn(%q) = true, want false", name)
		}
	}
	if isIgnoredVpn("WireGuard Tunnel", nil) {
		t.Errorf("empty ignore list must not match anything")
	}
}
