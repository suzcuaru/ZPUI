package zapret2

import (
	"fmt"
	"os"
	"zpui/internal/executil"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type ServiceStatus struct {
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	Strategy  string `json:"strategy"`
	PID       int    `json:"pid"`
}

func (m *Manager) GetServiceStatus() ServiceStatus {
	s := ServiceStatus{}

	cmd := executil.HiddenCmd("sc", "query", ServiceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return s
	}

	s.Installed = true
	outStr := string(output)
	s.Running = strings.Contains(outStr, "RUNNING") || strings.Contains(outStr, "\xd0\xa0\xd0\x90\xd0\x91\xd0\x9e\xd0\xa2\xd0\x90\xd0\x95\xd0\xa2")

	if s.Running {
		svcStrategy := m.GetInstalledServiceStrategy()
		if svcStrategy != "" {
			s.Strategy = svcStrategy
			m.cfg.SetCurrentStrategyV2(svcStrategy + ".cmd")
		} else {
			s.Strategy = m.cfg.GetCurrentStrategyV2()
		}

		taskOut, _ := executil.HiddenCmd("tasklist", "/FI", "IMAGENAME eq winws2.exe", "/FO", "CSV", "/NH").CombinedOutput()
		for _, line := range strings.Split(string(taskOut), "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "winws2.exe") {
				parts := strings.Split(line, ",")
				if len(parts) >= 2 {
					pidStr := strings.Trim(parts[1], "\" ")
					if pid, err := strconv.Atoi(pidStr); err == nil {
						s.PID = pid
						break
					}
				}
			}
		}
	} else {
		s.Strategy = m.cfg.GetCurrentStrategyV2()
	}

	return s
}

func (m *Manager) GetInstalledServiceStrategy() string {
	cmd := executil.HiddenCmd("reg", "query",
		`HKLM\System\CurrentControlSet\Services\`+ServiceName,
		"/v", "zapret2-strategy")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "REG_") {
			continue
		}
		val := line
		if idx := strings.Index(val, "REG_SZ"); idx >= 0 {
			val = strings.TrimSpace(val[idx+6:])
		} else if idx := strings.Index(val, "REG_EXPAND_SZ"); idx >= 0 {
			val = strings.TrimSpace(val[idx+13:])
		} else {
			parts := strings.SplitN(val, "REG_", 2)
			if len(parts) >= 2 {
				val = parts[1]
				if sp := strings.Index(val, " "); sp >= 0 {
					val = val[sp+1:]
				}
			}
		}
		val = strings.Trim(val, `" `)
		if val != "" {
			return val
		}
	}
	return ""
}

func (m *Manager) InstallService(strategyFile string) error {
	m.log.Info("service", fmt.Sprintf("Installing zapret2 service with strategy: %s", strategyFile))

	if strategyFile == "" {
		strategyFile = m.cfg.GetCurrentStrategyV2()
	}
	if strategyFile == "" {
		strategyFile = "general2.cmd"
	}

	m.Stop()
	m.RemoveService()

	if err := m.serviceCreate(strategyFile); err != nil {
		return fmt.Errorf("service create: %w", err)
	}

	m.cfg.SetCurrentStrategyV2(strategyFile)
	m.log.Info("service", "Zapret2 service installed successfully")
	return nil
}

func (m *Manager) RemoveService() error {
	m.log.Info("service", "Removing zapret2 service...")

	m.Stop()
	m.serviceRemove()

	m.log.Info("service", "Zapret2 service removed")
	return nil
}

type InstallResult struct {
	Success  bool     `json:"success"`
	Version  string   `json:"version"`
	Strategy string   `json:"strategy"`
	Running  bool     `json:"running"`
	Errors   []string `json:"errors,omitempty"`
}

func (m *Manager) InstallServiceLogged(strategyFile string) (*InstallResult, error) {
	if strategyFile == "" {
		strategyFile = m.cfg.GetCurrentStrategyV2()
	}
	if strategyFile == "" {
		strategyFile = m.DefaultStrategyName()
	}

	m.log.Info("install", fmt.Sprintf("Installing zapret2 service, strategy: %s", strategyFile))

	strategyPath := m.cfg.Zapret2StrategyPath(strategyFile)
	if _, err := os.Stat(strategyPath); err != nil {
		return &InstallResult{Errors: []string{"strategy not found: " + strategyFile}}, nil
	}
	winws2 := filepath.Join(m.cfg.Zapret2BinDir(), "winws2.exe")
	if _, err := os.Stat(winws2); err != nil {
		return &InstallResult{Errors: []string{"winws2.exe not found at: " + winws2}}, nil
	}

	m.log.Info("install", "Pre-flight: testing winws2.exe as process before installing service...")
	if err := m.StartWithStrategy(strategyFile); err != nil {
		m.log.Error("install", fmt.Sprintf("Pre-flight test FAILED: %v", err))
		return &InstallResult{Errors: []string{"winws2.exe process test failed: " + err.Error() + " — check ZPUI log for winws2 output"}}, nil
	}
	m.log.Info("install", "Pre-flight test passed — winws2.exe runs successfully as process")

	m.Stop()
	time.Sleep(1 * time.Second)

	m.serviceRemove()

	if err := m.serviceCreate(strategyFile); err != nil {
		return &InstallResult{Errors: []string{err.Error()}}, nil
	}

	time.Sleep(1500 * time.Millisecond)
	running := m.isServiceRunning()


	m.cfg.SetCurrentStrategyV2(strategyFile)

	return &InstallResult{
		Success:  true,
		Version:  m.GetVersion(),
		Strategy: strategyFile,
		Running:  running,
	}, nil
}

func (m *Manager) serviceCreate(strategyFile string) error {
	binPath := m.cfg.Zapret2BinDir()
	luaPath := m.cfg.Zapret2LuaDir()
	rootDir := m.cfg.GetZapret2Path()
	strategyPath := m.cfg.Zapret2StrategyPath(strategyFile)

	args, err := parseStrategyArgs(strategyPath, binPath, luaPath, rootDir)
	if err != nil {
		return fmt.Errorf("parse strategy: %w", err)
	}

	winws2 := filepath.Join(strings.TrimSuffix(binPath, `\`), "winws2.exe")
	fullCmd := fmt.Sprintf(`"%s" %s`, winws2, args)
	m.log.Info("service", fmt.Sprintf("sc create binPath: %s", fullCmd))

	stopService(ServiceName)
	deleteService(ServiceName)
	stopWinDivertDrivers()

	if err := runSc("create", ServiceName,
		"binPath=", fullCmd,
		"DisplayName=", "zapret2",
		"start=", "auto"); err != nil {
		return fmt.Errorf("sc create: %w", err)
	}

	runSc("description", ServiceName, "Zapret2 DPI bypass (Lua engine)")

	name := strings.TrimSuffix(strategyFile, ".cmd")
	runRegAdd(`HKLM\System\CurrentControlSet\Services\` + ServiceName,
		"/v", "zapret2-strategy",
		"/t", "REG_SZ",
		"/d", name,
		"/f")

	m.log.Info("service", fmt.Sprintf("Strategy saved to registry: %s", name))

	if err := runSc("start", ServiceName); err != nil {
		m.cfg.SetCurrentStrategyV2(strategyFile)
		return fmt.Errorf("sc start: %w", err)
	}

	m.log.Info("service", "Service start command issued, waiting 3s to verify...")

	time.Sleep(3 * time.Second)

	if !m.isServiceRunning() {
		state := getServiceState(ServiceName)
		m.log.Error("service", fmt.Sprintf("Service NOT running after 3s. State: %s", state))
		m.log.Info("service", "Attempting to read Windows Event Log for crash details...")
		m.logScErrors()

		deleteService(ServiceName)
		return fmt.Errorf("service started but crashed immediately (state: %s). Check that winws2.exe and all DLLs (WinDivert.dll, cygwin1.dll) are in binaries/windows-x86_64/ and that Lua scripts exist in lua/", state)
	}

	m.log.Info("service", "Service verified running after 3s")
	return nil
}

func (m *Manager) serviceRemove() {
	stopService(ServiceName)
	deleteService(ServiceName)

	out := runCmd("tasklist", "/FI", "IMAGENAME eq winws2.exe")
	if strings.Contains(out, "winws2.exe") {
		m.log.Info("service", "winws2.exe still running, killing")
		runCmd("taskkill", "/IM", "winws2.exe", "/F")
	}
}

func runSc(args ...string) error {
	cmd := executil.HiddenCmd("sc", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sc %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runCmd(name string, args ...string) string {
	cmd := executil.HiddenCmd(name, args...)
	out, _ := cmd.CombinedOutput()
	return string(out)
}

func runRegAdd(keyPath string, args ...string) {
	allArgs := append([]string{"add", keyPath}, args...)
	executil.HiddenCmd("reg", allArgs...).Run()
}

func stopService(name string) {
	executil.HiddenCmd("net", "stop", name).Run()
}

func deleteService(name string) {
	executil.HiddenCmd("sc", "delete", name).Run()
}

func serviceExists(name string) bool {
	return executil.HiddenCmd("sc", "query", name).Run() == nil
}

func stopWinDivertDrivers() {
	stopService("WinDivert")
	stopService("WinDivert14")
}

func removeWinDivertDrivers() {
	stopWinDivertDrivers()
	deleteService("WinDivert")
	deleteService("WinDivert14")
}

func getServiceState(name string) string {
	cmd := executil.HiddenCmd("sc", "query", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "NOT_FOUND"
	}
	outStr := string(output)
	for _, line := range strings.Split(outStr, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "STATE") {
			return line
		}
	}
	return "UNKNOWN"
}

func (m *Manager) logScErrors() {
	cmd := executil.HiddenCmd("sc", "query", ServiceName)
	if out, err := cmd.CombinedOutput(); err != nil {
		m.log.Error("service", fmt.Sprintf("sc query error: %v: %s", err, strings.TrimSpace(string(out))))
	} else {
		m.log.Info("service", fmt.Sprintf("sc query output:\n%s", strings.TrimSpace(string(out))))
	}

	regCmd := executil.HiddenCmd("reg", "query",
		`HKLM\System\CurrentControlSet\Services\`+ServiceName)
	if out, err := regCmd.CombinedOutput(); err == nil {
		m.log.Info("service", fmt.Sprintf("Registry params:\n%s", strings.TrimSpace(string(out))))
	}

	binDir := m.cfg.Zapret2BinDir()
	m.log.Info("service", fmt.Sprintf("BinDir: %s", binDir))
	for _, f := range []string{"winws2.exe", "WinDivert.dll", "WinDivert64.sys", "cygwin1.dll"} {
		p := filepath.Join(binDir, f)
		if _, err := os.Stat(p); err != nil {
			m.log.Error("service", fmt.Sprintf("MISSING: %s", p))
		} else {
			m.log.Info("service", fmt.Sprintf("OK: %s", p))
		}
	}

	luaDir := m.cfg.Zapret2LuaDir()
	entries, _ := os.ReadDir(luaDir)
	m.log.Info("service", fmt.Sprintf("LuaDir: %s (%d files)", luaDir, len(entries)))
}