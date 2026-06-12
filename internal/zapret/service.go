package zapret

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type ServiceStatus struct {
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	Strategy  string `json:"strategy"`
	PID       int    `json:"pid"`
}

func (m *Manager) GetServiceStatus() ServiceStatus {
	s := ServiceStatus{}

	cmd := exec.Command("sc", "query", "zapret")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return s
	}

	s.Installed = true
	s.Running = strings.Contains(string(output), "RUNNING")

	if s.Running {
		svcStrategy := m.getServiceStrategyFromBinPath()
		if svcStrategy != "" {
			s.Strategy = svcStrategy
			m.cfg.SetCurrentStrategy(svcStrategy + ".bat")
		} else {
			s.Strategy = m.cfg.GetCurrentStrategy()
		}

		taskOut, _ := exec.Command("tasklist", "/FI", "IMAGENAME eq winws.exe", "/FO", "CSV", "/NH").CombinedOutput()
		for _, line := range strings.Split(string(taskOut), "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "winws.exe") {
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
		s.Strategy = m.cfg.GetCurrentStrategy()
	}

	return s
}

func (m *Manager) getServiceStrategyFromBinPath() string {
	cmd := exec.Command("reg", "query",
		`HKLM\System\CurrentControlSet\Services\zapret`,
		"/v", "zapret-discord-youtube")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "REG_") {
			continue
		}
		parts := strings.SplitN(line, "REG_", 2)
		if len(parts) < 2 {
			continue
		}
		val := parts[1]
		idx := strings.Index(val, " ")
		if idx >= 0 {
			val = val[idx+1:]
		}
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"`)
		if val != "" {
			return val
		}
	}
	return ""
}

func (m *Manager) InstallService(strategyFile string) error {
	m.log.Info("service", fmt.Sprintf("Installing service with strategy: %s", strategyFile))

	m.Stop()
	m.RemoveService()

	binPath := m.cfg.BinDir()
	listsPath := m.cfg.ListsDir()
	strategyPath := m.cfg.StrategyPath(strategyFile)

	args, err := m.parseStrategyArgs(strategyPath, binPath, listsPath)
	if err != nil {
		return fmt.Errorf("parse strategy: %w", err)
	}
	m.log.Info("service", fmt.Sprintf("Parsed args length: %d", len(args)))

	exePath := filepath.Join(strings.TrimSuffix(binPath, `\`), "winws.exe")
	strategyName := strings.TrimSuffix(filepath.Base(strategyFile), filepath.Ext(strategyFile))

	m.log.Info("service", "Requesting admin elevation...")
	err = elevatedSCCreate(exePath, args, strategyName)
	if err != nil {
		return fmt.Errorf("elevated service create: %w", err)
	}

	m.cfg.SetCurrentStrategy(strategyFile)
	m.log.Info("service", "Service installed successfully")
	return nil
}

func (m *Manager) RemoveService() error {
	m.log.Info("service", "Removing zapret service...")

	m.Stop()

	err := elevatedSCRemove()
	if err != nil {
		m.log.Warn("service", fmt.Sprintf("elevated remove warning: %v", err))
	}

	m.log.Info("service", "Service removed")
	return nil
}

func (m *Manager) parseStrategyArgs(strategyPath, binPath, listsPath string) (string, error) {
	data, err := os.ReadFile(strategyPath)
	if err != nil {
		return "", err
	}

	content := string(data)
	content = strings.ReplaceAll(content, "\r", "")

	lines := strings.Split(content, "\n")
	var argsLines []string
	capturing := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(line, "@") || strings.HasPrefix(line, "::") || strings.HasPrefix(line, "echo") {
			if capturing {
				continue
			}
			continue
		}

		if strings.Contains(line, "winws.exe") {
			capturing = true
			idx := strings.Index(line, "winws.exe") + len("winws.exe")
			line = strings.TrimSpace(line[idx:])
		}

		if capturing {
			line = strings.TrimSuffix(line, "^")
			line = strings.TrimSuffix(line, "^ ")
			line = strings.TrimSpace(line)

			if line == "" {
				continue
			}

			if !strings.HasPrefix(line, "--") && !strings.HasPrefix(line, "-") {
				line = strings.TrimPrefix(line, "\"")
			}

			line = strings.ReplaceAll(line, `%BIN%`, binPath)
			line = strings.ReplaceAll(line, `%LISTS%`, listsPath)
			line = strings.ReplaceAll(line, `%GameFilterTCP%`, "12")
			line = strings.ReplaceAll(line, `%GameFilterUDP%`, "12")
			line = strings.ReplaceAll(line, `"%~dp0bin\`, fmt.Sprintf(`"%s`, binPath))
			line = strings.ReplaceAll(line, `"%~dp0lists\`, fmt.Sprintf(`"%s`, listsPath))

			argsLines = append(argsLines, line)
		}
	}

	return strings.Join(argsLines, " "), nil
}
