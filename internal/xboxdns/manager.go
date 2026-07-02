package xboxdns

import (
	"fmt"
	"strings"
	"sync"

	"zpui/internal/executil"
	"zpui/internal/logger"
)

type Manager struct {
	mu       sync.RWMutex
	enabled  bool
	primary  string
	secondary string
	log      *logger.Logger
	originalDNS []string
}

func NewManager(log *logger.Logger) *Manager {
	return &Manager{
		log: log,
	}
}

func (m *Manager) Configure(primary, secondary string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.primary = primary
	m.secondary = secondary
}

func (m *Manager) IsEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled
}

func (m *Manager) Enable() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.primary == "" {
		return fmt.Errorf("primary DNS not configured")
	}

	m.log.Info("xboxdns", fmt.Sprintf("Enabling Xbox DNS: %s / %s", m.primary, m.secondary))

	adapters := getActiveAdapters()
	if len(adapters) == 0 {
		return fmt.Errorf("no active network adapters found")
	}

	m.originalDNS = make([]string, 0)
	for _, adapter := range adapters {
		orig := getCurrentDNS(adapter)
		m.originalDNS = append(m.originalDNS, adapter+"|"+orig)

		executil.HiddenCmd("netsh", "interface", "ip", "set", "dns",
			adapter, "static", m.primary).Run()
		m.log.Info("xboxdns", fmt.Sprintf("Set %s primary DNS: %s", adapter, m.primary))

		if m.secondary != "" {
			executil.HiddenCmd("netsh", "interface", "ip", "add", "dns",
				adapter, m.secondary, "index=2").Run()
			m.log.Info("xboxdns", fmt.Sprintf("Set %s secondary DNS: %s", adapter, m.secondary))
		}
	}

	m.enabled = true
	m.log.Info("xboxdns", "Xbox DNS enabled")
	return nil
}

func (m *Manager) Disable() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.log.Info("xboxdns", "Disabling Xbox DNS, restoring original DNS")

	for _, entry := range m.originalDNS {
		parts := strings.SplitN(entry, "|", 2)
		if len(parts) != 2 {
			continue
		}
		adapter := parts[0]
		origDNS := parts[1]

		if origDNS == "" || origDNS == "dhcp" {
			executil.HiddenCmd("netsh", "interface", "ip", "set", "dns",
				adapter, "source=dhcp").Run()
			m.log.Info("xboxdns", fmt.Sprintf("Restored %s DNS to DHCP", adapter))
		} else {
			executil.HiddenCmd("netsh", "interface", "ip", "set", "dns",
				adapter, "static", origDNS).Run()
			m.log.Info("xboxdns", fmt.Sprintf("Restored %s DNS to %s", adapter, origDNS))
		}
	}

	m.originalDNS = nil
	m.enabled = false
	m.log.Info("xboxdns", "Xbox DNS disabled")
	return nil
}

func getActiveAdapters() []string {
	cmd := executil.HiddenCmd("netsh", "interface", "show", "interface")
	output, err := cmd.Output()
	if err != nil {
		return getDefaultAdapter()
	}

	var adapters []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "Admin State") || strings.Contains(line, "---") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			state := fields[1]
			name := strings.Join(fields[3:], " ")
			if state == "Connected" || state == "Подключено" {
				adapters = append(adapters, name)
			}
		}
	}

	if len(adapters) == 0 {
		return getDefaultAdapter()
	}
	return adapters
}

func getDefaultAdapter() []string {
	cmd := executil.HiddenCmd("powershell", "-NoProfile", "-Command",
		"(Get-NetAdapter | Where-Object Status -eq 'Up' | Select-Object -First 1).Name")
	output, err := cmd.Output()
	if err != nil {
		return []string{"Ethernet"}
	}
	name := strings.TrimSpace(string(output))
	if name == "" {
		return []string{"Ethernet"}
	}
	return []string{name}
}

func getCurrentDNS(adapter string) string {
	cmd := executil.HiddenCmd("netsh", "interface", "ip", "show", "dns", adapter)
	output, err := cmd.Output()
	if err != nil {
		return "dhcp"
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "Configuration") || strings.Contains(line, "---") {
			continue
		}
		fields := strings.Fields(line)
		for _, f := range fields {
			if isIP(f) {
				return f
			}
		}
	}
	return "dhcp"
}

func isIP(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if p == "" || len(p) > 3 {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}
