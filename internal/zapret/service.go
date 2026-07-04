package zapret

import (
	"fmt"
	"os"
	"zpui/internal/executil"
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

	cmd := executil.HiddenCmd("sc", "query", "zapret")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return s
	}

	s.Installed = true
	outStr := string(output)
	s.Running = strings.Contains(outStr, "RUNNING") || strings.Contains(outStr, "РАБОТАЕТ")

	if s.Running {
		svcStrategy := m.GetInstalledServiceStrategy()
		if svcStrategy != "" {
			s.Strategy = svcStrategy
			m.cfg.SetCurrentStrategy(svcStrategy + ".bat")
		} else {
			s.Strategy = m.cfg.GetCurrentStrategy()
		}

		taskOut, _ := executil.HiddenCmd("tasklist", "/FI", "IMAGENAME eq winws.exe", "/FO", "CSV", "/NH").CombinedOutput()
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

func (m *Manager) GetInstalledServiceStrategy() string {
	cmd := executil.HiddenCmd("reg", "query",
		`HKLM\System\CurrentControlSet\Services\zapret`,
		"/v", "zapret-discord-youtube")
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
	m.log.Info("service", fmt.Sprintf("Installing service with strategy: %s", strategyFile))

	if strategyFile == "" {
		strategyFile = m.cfg.GetCurrentStrategy()
	}
	if strategyFile == "" {
		strategyFile = "general.bat"
	}

	m.Stop()
	m.RemoveService()

	if err := m.serviceCreate(strategyFile); err != nil {
		return fmt.Errorf("service create: %w", err)
	}

	m.cfg.SetCurrentStrategy(strategyFile)
	m.log.Info("service", "Service installed successfully")
	return nil
}

func (m *Manager) RemoveService() error {
	m.log.Info("service", "Removing zapret service...")

	m.Stop()
	m.serviceRemove()

	m.log.Info("service", "Service removed")
	return nil
}

func parseStrategyArgs(strategyPath, binPath, listsPath, gfTCP, gfUDP string) (string, error) {
	data, err := os.ReadFile(strategyPath)
	if err != nil {
		return "", err
	}

	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(content, "\n")

	var argLines []string
	inArgs := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.Contains(trimmed, "winws.exe") {
			idx := strings.Index(trimmed, "winws.exe")
			rest := trimmed[idx+len("winws.exe"):]
			rest = strings.TrimLeft(rest, ` "`)
			rest = strings.TrimSpace(rest)
			if rest != "" {
				argLines = append(argLines, rest)
				inArgs = true
			}
			continue
		}

		if inArgs {
			if trimmed == "" || strings.HasPrefix(trimmed, "@echo") || strings.HasPrefix(trimmed, "set ") || strings.HasPrefix(trimmed, "cd ") || strings.HasPrefix(trimmed, "call ") || strings.HasPrefix(trimmed, "start ") || strings.HasPrefix(trimmed, "chcp ") || strings.HasPrefix(trimmed, "::") || strings.HasPrefix(trimmed, "rem ") {
				break
			}
			argLines = append(argLines, trimmed)
		}
	}

	if len(argLines) == 0 {
		return "", fmt.Errorf("no arguments found after winws.exe")
	}

	raw := strings.Join(argLines, " ")
	raw = strings.ReplaceAll(raw, "^", "")
	raw = strings.ReplaceAll(raw, "\t", " ")

	raw = strings.ReplaceAll(raw, `%BIN%`, filepath.ToSlash(binPath)+"/")
	raw = strings.ReplaceAll(raw, `%LISTS%`, filepath.ToSlash(listsPath)+"/")
	raw = strings.ReplaceAll(raw, `%GameFilterTCP%`, gfTCP)
	raw = strings.ReplaceAll(raw, `%GameFilterUDP%`, gfUDP)
	raw = strings.ReplaceAll(raw, `%GameFilter%`, gfTCP)

	raw = strings.ReplaceAll(raw, `"%~dp0bin\`, fmt.Sprintf(`"%s`, filepath.ToSlash(binPath)+"/"))
	raw = strings.ReplaceAll(raw, `"%~dp0lists\`, fmt.Sprintf(`"%s`, filepath.ToSlash(listsPath)+"/"))

	raw = strings.ReplaceAll(raw, `/`, `\`)

	tokens := splitArgs(raw)
	var result []string
	for _, t := range tokens {
		if strings.HasPrefix(t, "--") && strings.Contains(t, "=") {
			parts := strings.SplitN(t, "=", 2)
			result = append(result, parts[0])
			if len(parts) > 1 {
				result = append(result, parts[1])
			}
		} else {
			result = append(result, t)
		}
	}

	return strings.Join(result, " "), nil
}

func splitArgs(s string) []string {
	var tokens []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '"' {
			inQuote = !inQuote
			cur.WriteByte(ch)
			continue
		}
		if (ch == ' ' || ch == '\t') && !inQuote {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
			continue
		}
		cur.WriteByte(ch)
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}
