package app

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"

	"zpui/internal/executil"
)

// ============================================================
// DIAGNOSTICS
// ============================================================

// diagResult — результат одной проверки диагностики.
type diagResult struct {
	Status string `json:"status"`
	Label  string `json:"label"`
	Detail string `json:"detail"`
}

// RunDiagnostics — полная диагностика системы (замена GET /api/zapret/diagnostics).
func (a *App) RunDiagnostics() map[string]interface{} {
	results := map[string]interface{}{}

	results["bfe_service"] = checkService("BFE", "Base Filtering Engine")
	results["zapret_service"] = checkService("zapret", "Служба Zapret")
	results["windivert"] = checkWinDivert(a.cfg.GetZapretPath())
	results["winws_process"] = checkProcess("winws.exe", "winws.exe (Zapret)")
	results["tcp_timestamps"] = checkTCPTimestamps()
	results["firewall"] = checkFirewallRule()
	results["system_proxy"] = checkSystemProxy()
	results["conflicting"] = checkConflictingServices()
	results["killer"] = checkServiceList("Killer", "Killer Network Service")
	results["intel"] = checkIntelConnectivity()
	results["checkpoint"] = checkCheckPoint()
	results["smartbyte"] = checkServiceList("SmartByte", "SmartByte")
	results["adguard"] = checkProcess("AdguardSvc.exe", "Adguard")
	results["vpn"] = checkVPN()
	results["dns"] = checkDNS()
	results["hosts_file"] = checkHostsFile()
	results["proxy"] = checkProxy(a)

	return results
}

func checkService(name, label string) diagResult {
	cmd := executil.HiddenCmd("sc", "query", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return diagResult{Status: "warn", Label: label, Detail: "Не удалось проверить"}
	}
	out := string(output)
	if strings.Contains(out, "RUNNING") {
		return diagResult{Status: "ok", Label: label, Detail: "Работает"}
	}
	if strings.Contains(out, "STOPPED") {
		return diagResult{Status: "warn", Label: label, Detail: "Остановлен"}
	}
	if strings.Contains(out, "does not exist") {
		return diagResult{Status: "warn", Label: label, Detail: "Не найден"}
	}
	return diagResult{Status: "ok", Label: label, Detail: "Присутствует"}
}

func checkWinDivert(zapretDir string) diagResult {
	if zapretDir == "" {
		return diagResult{Status: "warn", Label: "WinDivert", Detail: "Путь не задан"}
	}
	paths := []string{
		zapretDir + "\\WinDivert.dll",
		zapretDir + "\\bin\\WinDivert.dll",
		zapretDir + "\\WinDivert\\WinDivert.dll",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return diagResult{Status: "ok", Label: "WinDivert", Detail: "Найден"}
		}
	}
	return diagResult{Status: "error", Label: "WinDivert", Detail: "Не найден"}
}

func checkTCPTimestamps() diagResult {
	cmd := executil.HiddenCmd("netsh", "interface", "tcp", "show", "global")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return diagResult{Status: "warn", Label: "TCP Timestamps", Detail: "Не удалось проверить"}
	}
	out := string(output)
	if strings.Contains(out, "disabled") || strings.Contains(out, "Отключено") {
		return diagResult{Status: "ok", Label: "TCP Timestamps", Detail: "Отключены (OK)"}
	}
	return diagResult{Status: "warn", Label: "TCP Timestamps", Detail: "Включены — могут мешать"}
}

func checkFirewallRule() diagResult {
	cmd := executil.HiddenCmd("netsh", "advfirewall", "firewall", "show", "rule", "name=ZPUI_SOCKS5")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return diagResult{Status: "warn", Label: "Firewall (SOCKS5)", Detail: "Правило не найдено"}
	}
	out := string(output)
	if strings.Contains(out, "ZPUI_SOCKS5") {
		return diagResult{Status: "ok", Label: "Firewall (SOCKS5)", Detail: "Правило активно"}
	}
	return diagResult{Status: "warn", Label: "Firewall (SOCKS5)", Detail: "Правило не найдено"}
}

func checkConflictingServices() diagResult {
	conflicts := []string{}
	services := map[string]string{
		"AdguardService": "Adguard",
		"SmartByte":      "SmartByte",
		"SAB":            "McAfee",
		"ekrn":           "ESET",
		"ksvlasst":       "Kaspersky",
	}
	for svc, name := range services {
		cmd := executil.HiddenCmd("sc", "query", svc)
		output, _ := cmd.CombinedOutput()
		if strings.Contains(string(output), "RUNNING") {
			conflicts = append(conflicts, name)
		}
	}
	if len(conflicts) > 0 {
		return diagResult{Status: "warn", Label: "Конфликты", Detail: strings.Join(conflicts, ", ")}
	}
	return diagResult{Status: "ok", Label: "Конфликты", Detail: "Не обнаружены"}
}

func checkDNS() diagResult {
	cmd := executil.HiddenCmd("powershell", "-Command", "Get-DnsClientServerAddress -AddressFamily IPv4 | Select-Object -First 10")
	output, _ := cmd.CombinedOutput()
	out := string(output)
	if strings.Contains(out, "127.0.0.1") || strings.Contains(out, "localhost") {
		return diagResult{Status: "warn", Label: "DNS", Detail: "Обнаружен локальный DNS"}
	}
	return diagResult{Status: "ok", Label: "DNS", Detail: "Стандартный"}
}

func checkProxy(a *App) diagResult {
	if a.proxy.IsRunning() {
		return diagResult{Status: "ok", Label: "SOCKS5 Прокси", Detail: "Работает"}
	}
	return diagResult{Status: "warn", Label: "SOCKS5 Прокси", Detail: "Остановлен"}
}

func checkProcess(name, label string) diagResult {
	cmd := executil.HiddenCmd("tasklist", "/FI", "IMAGENAME eq "+name)
	output, _ := cmd.CombinedOutput()
	if strings.Contains(string(output), name) {
		return diagResult{Status: "ok", Label: label, Detail: "Запущен"}
	}
	return diagResult{Status: "warn", Label: label, Detail: "Не запущен"}
}

func checkSystemProxy() diagResult {
	cmd := executil.HiddenCmd("reg", "query", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyEnable")
	output, _ := cmd.CombinedOutput()
	out := string(output)
	if strings.Contains(out, "0x1") {
		cmd2 := executil.HiddenCmd("reg", "query", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyServer")
		output2, _ := cmd2.CombinedOutput()
		proxy := strings.TrimSpace(strings.ReplaceAll(string(output2), "ProxyServer", ""))
		proxy = strings.TrimSpace(strings.ReplaceAll(proxy, "REG_SZ", ""))
		return diagResult{Status: "warn", Label: "Системный прокси", Detail: "Включен: " + proxy}
	}
	return diagResult{Status: "ok", Label: "Системный прокси", Detail: "Отключен"}
}

func checkServiceList(keyword, label string) diagResult {
	cmd := executil.HiddenCmd("sc", "query", "state=", "all")
	output, _ := cmd.CombinedOutput()
	if strings.Contains(string(output), keyword) {
		return diagResult{Status: "warn", Label: label, Detail: "Обнаружен — может конфликтовать"}
	}
	return diagResult{Status: "ok", Label: label, Detail: "Не обнаружен"}
}

func checkIntelConnectivity() diagResult {
	cmd := executil.HiddenCmd("sc", "query", "state=", "all")
	output, _ := cmd.CombinedOutput()
	out := string(output)
	if strings.Contains(out, "Intel") && strings.Contains(out, "Connectivity") && strings.Contains(out, "Network") {
		return diagResult{Status: "warn", Label: "Intel Connectivity", Detail: "Обнаружен — конфликтует с Zapret"}
	}
	return diagResult{Status: "ok", Label: "Intel Connectivity", Detail: "Не обнаружен"}
}

func checkCheckPoint() diagResult {
	cmd := executil.HiddenCmd("sc", "query", "state=", "all")
	output, _ := cmd.CombinedOutput()
	out := string(output)
	if strings.Contains(out, "TracSrvWrapper") || strings.Contains(out, "EPWD") {
		return diagResult{Status: "warn", Label: "Check Point", Detail: "Обнаружен — конфликтует с Zapret"}
	}
	return diagResult{Status: "ok", Label: "Check Point", Detail: "Не обнаружен"}
}

func checkVPN() diagResult {
	cmd := executil.HiddenCmd("sc", "query", "state=", "all")
	output, _ := cmd.CombinedOutput()
	out := string(output)
	var found []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(strings.ToUpper(line), "VPN") && strings.Contains(line, "SERVICE_NAME") {
			name := strings.TrimPrefix(line, "SERVICE_NAME: ")
			found = append(found, name)
		}
	}
	if len(found) > 0 {
		return diagResult{Status: "warn", Label: "VPN сервисы", Detail: strings.Join(found, ", ")}
	}
	return diagResult{Status: "ok", Label: "VPN сервисы", Detail: "Не обнаружены"}
}

func checkHostsFile() diagResult {
	hostsPath := os.Getenv("SystemRoot") + `\System32\drivers\etc\hosts`
	data, err := os.ReadFile(hostsPath)
	if err != nil {
		return diagResult{Status: "warn", Label: "Hosts файл", Detail: "Не удалось прочитать"}
	}
	content := strings.ToLower(string(data))
	blocked := []string{}
	for _, domain := range []string{"youtube.com", "youtu.be", "discord.com", "google.com"} {
		if strings.Contains(content, domain) && !strings.HasPrefix(strings.TrimSpace(content), "#") {
			blocked = append(blocked, domain)
		}
	}
	if len(blocked) > 0 {
		return diagResult{Status: "warn", Label: "Hosts файл", Detail: "Найдены записи: " + strings.Join(blocked, ", ")}
	}
	return diagResult{Status: "ok", Label: "Hosts файл", Detail: "Чистый"}
}

// ============================================================
// NETWORK HELPERS
// ============================================================

func getLocalIPs() []string {
	var ips []string
	interfaces, err := net.Interfaces()
	if err != nil {
		return ips
	}
	for _, iface := range interfaces {
		nameLower := strings.ToLower(iface.Name)
		if strings.Contains(nameLower, "loopback") ||
			strings.Contains(nameLower, "vethernet") {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.IsLoopback() || ipNet.IP.To4() == nil {
				continue
			}
			ip := ipNet.IP
			if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				continue
			}
			if bytes.HasPrefix(ip, []byte{172, 26}) ||
				bytes.HasPrefix(ip, []byte{172, 20}) ||
				bytes.HasPrefix(ip, []byte{172, 17}) ||
				bytes.HasPrefix(ip, []byte{172, 18}) ||
				bytes.HasPrefix(ip, []byte{172, 19}) {
				continue
			}
			ips = append(ips, ip.String())
		}
	}
	return ips
}

func getMACAddress() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range interfaces {
		if len(iface.HardwareAddr) > 0 && !strings.HasPrefix(iface.Name, "Loopback") {
			return iface.HardwareAddr.String()
		}
	}
	return ""
}

func getHostname() string {
	cmd := executil.HiddenCmd("hostname")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

func getARPTable() map[string]string {
	arp := make(map[string]string)
	cmd := executil.HiddenCmd("arp", "-a")
	output, err := cmd.Output()
	if err != nil {
		return arp
	}
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Interface") || strings.HasPrefix(line, "---") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		host := strings.Trim(fields[0], " ")
		ipRaw := fields[0]
		mac := ""
		if len(fields) >= 2 {
			ipRaw = strings.Trim(fields[0], "()")
			if strings.Contains(fields[0], "(") {
				ipRaw = strings.Trim(fields[1], "()")
				mac = fields[3]
			} else if len(fields) >= 4 {
				mac = fields[1]
				ipRaw = strings.Trim(host, "()")
			}
		}
		if mac != "" && mac != "FF-FF-FF-FF-FF-FF" && mac != "ff-ff-ff-ff-ff-ff" {
			arp[ipRaw] = mac
		}
	}
	return arp
}

func resolveHostname(ip string) string {
	names, err := net.LookupAddr(ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	name := names[0]
	name = strings.TrimSuffix(name, ".")
	return name
}

// ResolveExePath — возвращает путь к исполняемому файлу (для main.go).
func ResolveExePath() string {
	return getExePath()
}

// Suppress unused import warnings
var _ = filepath.Join
