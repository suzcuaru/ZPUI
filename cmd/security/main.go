package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"zpui/internal/executil"
	"zpui/internal/logger"
	"zpui/internal/winprogress"
)

var version = "1.0.0"

type CheckResult struct {
	Name    string
	Status  string
	Details string
}

func main() {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)

	logFile := filepath.Join(exeDir, "logs", "security.log")
	os.MkdirAll(filepath.Dir(logFile), 0755)
	logMgr, _ := logger.New(filepath.Dir(logFile), exeDir)
	if logMgr == nil {
		fmt.Fprintln(os.Stderr, "logger init failed")
		os.Exit(1)
	}
	defer logMgr.Close()

	logMgr.Info("security", fmt.Sprintf("Security module started (v%s)", version))

	pw := winprogress.New(fmt.Sprintf("ZPUI Security v%s", version))
	defer func() {
		pw.Close()
		pw.WaitClosed()
	}()

	pw.SetStatus("Проверка целостности файлов...")
	pw.SetProgress(15)
	time.Sleep(300 * time.Millisecond)

	var results []CheckResult

	results = append(results, verifyFileHashes(exeDir)...)
	pw.SetProgress(40)

	pw.SetStatus("Проверка драйверов...")
	results = append(results, checkDrivers(exeDir)...)
	pw.SetProgress(60)

	pw.SetStatus("Сканирование процессов...")
	results = append(results, scanProcesses()...)
	pw.SetProgress(80)

	pw.SetStatus("Проверка конфликтов...")
	results = append(results, checkConflicts()...)
	pw.SetProgress(95)

	report := buildReport(results)
	reportPath := filepath.Join(exeDir, "logs", "security_report.txt")
	if err := os.WriteFile(reportPath, []byte(report), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save report: %s\n", err)
	}

	exitCode := 0
	statusLine := "✓ Все проверки пройдены"
	for _, r := range results {
		if r.Status == "error" {
			exitCode = 2
			statusLine = "✗ Обнаружены критические проблемы"
			break
		}
		if r.Status == "warning" && exitCode == 0 {
			exitCode = 1
			statusLine = "⚠ Есть предупреждения"
		}
	}

	logMgr.Info("security", fmt.Sprintf("Check complete: %d results, exit %d", len(results), exitCode))

	pw.SetProgress(100)
	pw.SetStatus(statusLine)
	fmt.Print(report)

	time.Sleep(5 * time.Second)
	os.Exit(exitCode)
}

func verifyFileHashes(exeDir string) []CheckResult {
	var results []CheckResult

	checksumsPath := filepath.Join(exeDir, "checksums.sha256")
	data, err := os.ReadFile(checksumsPath)
	if err != nil {
		results = append(results, CheckResult{
			Name:    "Файл контрольных сумм",
			Status:  "warning",
			Details: "checksums.sha256 не найден — проверка хешей пропущена",
		})
		return results
	}

	expected := parseChecksums(string(data))
	if len(expected) == 0 {
		results = append(results, CheckResult{
			Name:    "Файл контрольных сумм",
			Status:  "warning",
			Details: "checksums.sha256 пуст или некорректен",
		})
		return results
	}

	criticalFiles := []string{"ZPUI.exe", "selfupdate.exe", "report.exe", "security.exe"}
	for _, fname := range criticalFiles {
		expHash, ok := expected[strings.ToLower(fname)]
		if !ok {
			continue
		}
		filePath := filepath.Join(exeDir, fname)
		actualHash, err := computeSHA256(filePath)
		if err != nil {
			results = append(results, CheckResult{
				Name:    "Файл: " + fname,
				Status:  "error",
				Details: "файл отсутствует или недоступен",
			})
			continue
		}
		if actualHash != expHash {
			results = append(results, CheckResult{
				Name:    "Файл: " + fname,
				Status:  "error",
				Details: fmt.Sprintf("хеш не совпадает (ожидался %s, получен %s)", expHash[:12], actualHash[:12]),
			})
		} else {
			results = append(results, CheckResult{
				Name:    "Файл: " + fname,
				Status:  "ok",
				Details: "хеш совпадает",
			})
		}
	}

	return results
}

func checkDrivers(exeDir string) []CheckResult {
	var results []CheckResult

	for _, drvName := range []string{"WinDivert", "WinDivert14"} {
		out, _ := executil.HiddenCmd("sc", "query", drvName).CombinedOutput()
		outStr := string(out)
		if strings.Contains(outStr, "RUNNING") || strings.Contains(outStr, "РАБОТАЕТ") {
			results = append(results, CheckResult{
				Name:    "Драйвер: " + drvName,
				Status:  "ok",
				Details: "запущен",
			})
		} else if strings.Contains(outStr, "not exist") || strings.Contains(outStr, "не существует") || strings.Contains(outStr, "FAILED") {
			results = append(results, CheckResult{
				Name:    "Драйвер: " + drvName,
				Status:  "warning",
				Details: "не установлен (нормально если zapret использует встроенный)",
			})
		}
	}

	zapretDir := findZapretDir(exeDir)
	if zapretDir != "" {
		sysPath := filepath.Join(zapretDir, "bin", "WinDivert64.sys")
		if _, err := os.Stat(sysPath); err != nil {
			altSysPath := filepath.Join(zapretDir, "bin", "WinDivert32.sys")
			if _, err2 := os.Stat(altSysPath); err2 != nil {
				results = append(results, CheckResult{
					Name:    "WinDivert .sys",
					Status:  "error",
					Details: "WinDivert64.sys/32.sys не найден в папке zapret/bin",
				})
			}
		}
	}

	return results
}

func scanProcesses() []CheckResult {
	var results []CheckResult

	out, _ := executil.HiddenCmd("tasklist", "/FO", "CSV", "/NH").Output()
	lines := strings.Split(string(out), "\n")

	conflictProcesses := map[string]string{
		"openvpn":   "OpenVPN — может конфликтовать с DPI-обходом",
		"vpn":       "VPN-клиент — может мешать работе zapret",
		"nordvpn":   "NordVPN — перехватывает трафик",
		"warp":      "Cloudflare WARP — конфликтует с WinDivert",
		"clash":     "Clash — прокси-клиент может мешать",
		"v2ray":     "V2Ray/Xray — прокси может конфликтовать",
		"shadowsocks": "Shadowsocks — может перехватывать трафик",
	}

	winwsCount := 0
	for _, line := range lines {
		line = strings.ToLower(strings.TrimSpace(line))
		if strings.Contains(line, "winws.exe") {
			winwsCount++
		}
		for proc, desc := range conflictProcesses {
			if strings.Contains(line, proc) {
				results = append(results, CheckResult{
					Name:    "Процесс: " + proc,
					Status:  "warning",
					Details: desc,
				})
			}
		}
	}

	if winwsCount > 1 {
		results = append(results, CheckResult{
			Name:    "Процесс: winws.exe",
			Status:  "warning",
			Details: fmt.Sprintf("обнаружено %d процессов winws.exe — возможен конфликт", winwsCount),
		})
	}

	return results
}

func checkConflicts() []CheckResult {
	var results []CheckResult

	for _, loc := range []string{
		`C:\zapret`,
		`C:\Program Files\zapret`,
		`C:\Program Files (x86)\zapret`,
	} {
		if info, err := os.Stat(loc); err == nil && info.IsDir() {
			entries, _ := os.ReadDir(loc)
			hasWinws := false
			for _, e := range entries {
				if strings.Contains(strings.ToLower(e.Name()), "winws") || strings.Contains(strings.ToLower(e.Name()), "general") {
					hasWinws = true
					break
				}
			}
			if hasWinws {
				results = append(results, CheckResult{
					Name:    "Сторонний zapret",
					Status:  "warning",
					Details: "обнаружен сторонний zapret: " + loc,
				})
			}
		}
	}

	out, _ := executil.HiddenCmd("reg", "query",
		`HKLM\SYSTEM\CurrentControlSet\Services\TapNhkSvc`,
		"/v", "ImagePath").Output()
	if len(out) > 0 && !strings.Contains(string(out), "ERROR") {
		results = append(results, CheckResult{
			Name:    "Системный сервис",
			Status:  "warning",
			Details: "обнаружен TAP/NHK сервис — возможный конфликт",
		})
	}

	return results
}

func buildReport(results []CheckResult) string {
	var b strings.Builder
	b.WriteString("=== ZPUI Security Report ===\n")
	b.WriteString(fmt.Sprintf("Date: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("Module Version: %s\n\n", version))

	okCount, warnCount, errCount := 0, 0, 0
	for _, r := range results {
		icon := "✓"
		switch r.Status {
		case "warning":
			icon = "⚠"
			warnCount++
		case "error":
			icon = "✗"
			errCount++
		default:
			okCount++
		}
		b.WriteString(fmt.Sprintf("[%s] %s — %s\n", icon, r.Name, r.Details))
	}

	b.WriteString(fmt.Sprintf("\nSummary: %d ok, %d warnings, %d errors\n", okCount, warnCount, errCount))
	return b.String()
}

func computeSHA256(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h), nil
}

func parseChecksums(data string) map[string]string {
	result := map[string]string{}
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		hash := parts[0]
		filename := strings.Trim(parts[1], "*()")
		result[strings.ToLower(filename)] = strings.ToLower(hash)
	}
	return result
}

func findZapretDir(exeDir string) string {
	candidates := []string{
		filepath.Join(exeDir, "zapret"),
		filepath.Join(exeDir, "Zapret"),
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	return ""
}
