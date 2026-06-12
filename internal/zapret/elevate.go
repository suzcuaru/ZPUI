package zapret

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func (m *Manager) serviceCreate(strategyFile string) error {
	binPath := m.cfg.BinDir()
	listsPath := m.cfg.ListsDir()
	strategyPath := m.cfg.StrategyPath(strategyFile)

	args, err := parseStrategyArgs(strategyPath, binPath, listsPath)
	if err != nil {
		return fmt.Errorf("parse strategy: %w", err)
	}

	winws := filepath.Join(strings.TrimSuffix(binPath, `\`), "winws.exe")
	fullCmd := fmt.Sprintf(`"%s" %s`, winws, args)
	m.log.Info("service", fmt.Sprintf("sc create binPath: %s", fullCmd))

	stopService("zapret")
	deleteService("zapret")
	stopService("WinDivert")
	deleteService("WinDivert")
	stopService("WinDivert14")
	deleteService("WinDivert14")

	if err := runSc("create", "zapret",
		"binPath=", fullCmd,
		"DisplayName=", "zapret",
		"start=", "auto"); err != nil {
		return fmt.Errorf("sc create: %w", err)
	}

	runSc("description", "zapret", "Zapret DPI bypass software")

	name := strings.TrimSuffix(strategyFile, ".bat")
	runRegAdd("HKLM\\System\\CurrentControlSet\\Services\\zapret",
		"/v", "zapret-discord-youtube",
		"/t", "REG_SZ",
		"/d", name,
		"/f")

	m.log.Info("service", fmt.Sprintf("Strategy saved to registry: %s", name))

	if err := runSc("start", "zapret"); err != nil {
		m.cfg.SetCurrentStrategy(strategyFile)
		return fmt.Errorf("sc start: %w", err)
	}

	m.log.Info("service", "Service started successfully")
	return nil
}

func (m *Manager) serviceRemove() {
	stopService("zapret")
	deleteService("zapret")

	out := runCmd("tasklist", "/FI", "IMAGENAME eq winws.exe")
	if strings.Contains(out, "winws.exe") {
		m.log.Info("service", "winws.exe still running, killing")
		runCmd("taskkill", "/IM", "winws.exe", "/F")
	}

	if serviceExists("WinDivert") {
		stopService("WinDivert")
		deleteService("WinDivert")
	}
	if serviceExists("WinDivert14") {
		stopService("WinDivert14")
		deleteService("WinDivert14")
	}
}

func (m *Manager) serviceChangeStrategy(strategyFile string) error {
	m.log.Info("service", fmt.Sprintf("Changing strategy to: %s", strategyFile))

	if serviceExists("zapret") {
		m.log.Info("service", "Existing service found, removing first")
		stopService("zapret")
		deleteService("zapret")
		stopService("WinDivert")
		deleteService("WinDivert")
		stopService("WinDivert14")
		deleteService("WinDivert14")
	}

	return m.serviceCreate(strategyFile)
}

func runSc(args ...string) error {
	cmd := exec.Command("sc", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sc %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runCmd(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	out, _ := cmd.CombinedOutput()
	return string(out)
}

func runRegAdd(keyPath string, args ...string) {
	allArgs := append([]string{"add", keyPath}, args...)
	exec.Command("reg", allArgs...).Run()
}

func stopService(name string) {
	exec.Command("net", "stop", name).Run()
}

func deleteService(name string) {
	exec.Command("sc", "delete", name).Run()
}

func serviceExists(name string) bool {
	return exec.Command("sc", "query", name).Run() == nil
}


