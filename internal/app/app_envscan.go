package app

// Стартовая диагностика окружения: сторонние обходчики (служба/процессы
// zapret вне нашей папки) и активные VPN (службы, процессы, адаптеры,
// RAS-подключения). Вызывается со стартового экрана до проверки службы.

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/sys/windows/svc/mgr"

	"zpui/internal/executil"
)

// Известные клиентские VPN-процессы (без .exe). Совпадение по подстроке.
var vpnProcessHints = []string{
	"openvpn", "wireguard", "nordvpn", "expressvpn", "protonvpn", "surfshark",
	"cyberghost", "mullvad", "windscribe", "ipvanish", "tunnelbear",
	"hotspotshield", "vyprvpn", "privadovpn", "atlasvpn",
}

// VPN, которые не перехватывают трафик Discord/YouTube и не мешают движку
// (игровые и сетевые инструменты): на них не ругаемся никогда.
var vpnBenignHints = []string{
	"radmin", "hamachi", "zerotier", "tailscale", "tunngle", "gameranger", "teamviewer",
}

// Описания активных адаптеров, похожие на VPN. Tailscale/ZeroTier не считаем
// VPN-конкурентом: приложение само использует Tailscale-DNS (Xbox DNS).
var vpnAdapterRe = regexp.MustCompile(`(?i)vpn|wireguard|openvpn|tap-windows|nordvpn|protonvpn|expressvpn|surfshark|mullvad|cyberghost|windscribe`)

// isBenignVpn — находка относится к безобидным VPN (белый список).
func isBenignVpn(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return false
	}
	for _, h := range vpnBenignHints {
		if strings.Contains(n, h) {
			return true
		}
	}
	return false
}

// isIgnoredVpn — пользователь уже «Пропустил» эту находку (точное совпадение
// имени без учёта регистра).
func isIgnoredVpn(name string, ignore []string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return false
	}
	for _, ig := range ignore {
		if n == strings.ToLower(strings.TrimSpace(ig)) {
			return true
		}
	}
	return false
}

type vpnFindings struct {
	Processes []string
	Services  []string
	Adapters  []string
	Ras       []string
}

func (f *vpnFindings) empty() bool {
	return len(f.Processes)+len(f.Services)+len(f.Adapters)+len(f.Ras) == 0
}

// names — человекочитаемый список находок для сообщения.
func (f *vpnFindings) names() []string {
	out := make([]string, 0, len(f.Processes)+len(f.Services)+len(f.Adapters)+len(f.Ras))
	out = append(out, f.Services...)
	out = append(out, f.Processes...)
	out = append(out, f.Ras...)
	out = append(out, f.Adapters...)
	return out
}

// ScanEnvironment — снимок окружения для стартовой диагностики.
func (a *App) ScanEnvironment() map[string]interface{} {
	zapretSources := a.scanForeignZapret()
	vpn := a.scanVpn()
	return map[string]interface{}{
		"zapret": map[string]interface{}{
			"found":   len(zapretSources) > 0,
			"sources": zapretSources,
		},
		"vpn": map[string]interface{}{
			"found": !vpn.empty(),
			"names": vpn.names(),
		},
	}
}

// psOutput упразднён (07.10): PowerShell-дети подвешивали стартовый экран —
// перечисления процессов/адаптеров заменены внутренними вызовами Windows
// (envscan_native.go), внешние утилиты идут только через RunWithTimeout.

// scanForeignZapret: служба zapret, чей бинарник вне нашей папки, и процессы
// winws.exe не из нашей папки. Наша локальная копия может отсутствовать —
// тогда любая находка считается сторонней.
func (a *App) scanForeignZapret() []string {
	var found []string
	ourDir := ""
	if p := a.cfg.GetZapretPath(); p != "" {
		ourDir = strings.ToLower(filepath.Clean(p)) + string(filepath.Separator)
	}

	if bin, ok := serviceBinaryPath("zapret"); ok {
		binLower := cleanServiceBin(bin)
		if binLower != "" && (ourDir == "" || !strings.HasPrefix(binLower, ourDir)) {
			found = append(found, bin)
		}
	}
	for _, p := range runningWinwsPaths() {
		pLower := strings.ToLower(filepath.Clean(p))
		if ourDir == "" || !strings.HasPrefix(pLower, ourDir) {
			found = append(found, p)
		}
	}
	return found
}

// cleanServiceBin выделяет путь бинарника из командной строки службы
// («"C:\path\winws.exe" --args…») и приводит к нижнему регистру.
func cleanServiceBin(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, `"`) {
		if end := strings.Index(raw[1:], `"`); end >= 0 {
			raw = raw[1 : 1+end]
		}
	} else if idx := strings.Index(raw, " -"); idx > 0 {
		raw = raw[:idx]
	}
	return strings.ToLower(filepath.Clean(raw))
}

// serviceBinaryPath — путь к бинарнику службы через SCM (не зависит от
// языка вывода sc.exe).
func serviceBinaryPath(name string) (string, bool) {
	m, err := mgr.Connect()
	if err != nil {
		return "", false
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return "", false
	}
	defer s.Close()
	cfg, err := s.Config()
	if err != nil {
		return "", false
	}
	if cfg.BinaryPathName == "" {
		return "", false
	}
	return cfg.BinaryPathName, true
}

// runningWinwsPaths — пути запущенных winws.exe (снимок процессов, без PowerShell).
func runningWinwsPaths() []string {
	var paths []string
	for _, p := range processSnapshot() {
		if p.name == "winws.exe" && p.path != "" {
			paths = append(paths, p.path)
		}
	}
	return paths
}

func (a *App) scanVpn() *vpnFindings {
	f := &vpnFindings{}
	ignore := a.cfg.GetVpnIgnore()

	// Процессы: снимок системы (Toolhelp), имена уже lowercase.
	for _, p := range processSnapshot() {
		for _, hint := range vpnProcessHints {
			if strings.Contains(p.name, hint) {
				if isBenignVpn(p.name) || isIgnoredVpn(p.name, ignore) {
					break
				}
				f.Processes = append(f.Processes, p.name)
				break
			}
		}
	}

	// Службы с vpn/wireguard/openvpn в имени (sc query, state=all).
	if out, err := executil.RunWithTimeout(10*time.Second, "sc", "query", "state=", "all"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if !strings.Contains(line, "SERVICE_NAME") && !strings.Contains(line, "ИМЯ_СЛУЖБЫ") {
				continue
			}
			name := line
			if idx := strings.Index(line, ":"); idx >= 0 {
				name = strings.TrimSpace(line[idx+1:])
			}
			nameLower := strings.ToLower(name)
			if strings.Contains(nameLower, "vpn") || strings.Contains(nameLower, "wireguard") || strings.Contains(nameLower, "openvpn") {
				if isBenignVpn(name) || isIgnoredVpn(name, ignore) {
					continue
				}
				f.Services = append(f.Services, name)
			}
		}
	}

	// Активные адаптеры с VPN-описанием (GetAdaptersAddresses, без PowerShell).
	for _, ad := range upAdapters() {
		if isBenignVpn(ad.Desc) || isBenignVpn(ad.Name) || isIgnoredVpn(ad.Desc, ignore) {
			continue
		}
		f.Adapters = append(f.Adapters, vpnAdaptersFromOutput(ad.Name+"|"+ad.Desc)...)
	}

	// RAS-подключения (встроенный Windows VPN): rasdial без аргументов
	// перечисляет активные. Вывод — в OEM-кодировке (у русской Windows cp866):
	// декодируем обязательно, иначе «Нет подключений» не находится и его
	// кракозябры попадают в список VPN как «имя подключения».
	if out, err := executil.RunWithTimeout(10*time.Second, "rasdial"); err == nil {
		out = oemToUTF8(out)
		outLower := strings.ToLower(strings.TrimSpace(out))
		// пустой ответ rasdial формулируют по-разному (Win10/Win11):
		// «No connections» / «Нет подключений» / «Отсутствуют подключения»
		if outLower != "" &&
			!strings.Contains(outLower, "no connections") &&
			!strings.Contains(outLower, "нет подключени") &&
			!strings.Contains(outLower, "отсутствуют подключени") {
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimSpace(line)
				if line != "" && !isBenignVpn(line) && !isIgnoredVpn(line, ignore) {
					f.Ras = append(f.Ras, line)
				}
			}
		}
	}

	return f
}

// vpnAdaptersFromOutput разбирает вывод «Name|Description» построчно и
// отбирает описания, похожие на VPN.
func vpnAdaptersFromOutput(output string) []string {
	var out []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		_, desc, ok := strings.Cut(line, "|")
		if !ok {
			continue
		}
		if vpnAdapterRe.MatchString(desc) {
			out = append(out, desc)
		}
	}
	return out
}
