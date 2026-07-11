package zapret2

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Strategy struct {
	Name     string `json:"name"`
	Filename string `json:"filename"`
	Current  bool   `json:"current"`
}

func (m *Manager) ListStrategies() []Strategy {
	dir := m.cfg.GetZapret2Path()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	current := m.cfg.GetCurrentStrategyV2()
	var strategies []Strategy

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".cmd") {
			continue
		}
		if strings.HasPrefix(name, "service") {
			continue
		}

		displayName := strings.TrimSuffix(name, ".cmd")
		strategies = append(strategies, Strategy{
			Name:     displayName,
			Filename: name,
			Current:  name == current,
		})
	}

	sort.Slice(strategies, func(i, j int) bool {
		return strategies[i].Filename < strategies[j].Filename
	})

	return strategies
}

func (m *Manager) SetStrategy(filename string) error {
	if _, err := os.Stat(m.cfg.Zapret2StrategyPath(filename)); os.IsNotExist(err) {
		return err
	}

	if m.isServiceRunning() {
		if err := m.InstallService(filename); err != nil {
			return fmt.Errorf("service reinstall: %w", err)
		}
		return nil
	}

	if err := m.StartWithStrategy(filename); err != nil {
		return fmt.Errorf("start with strategy: %w", err)
	}
	return nil
}

func (m *Manager) GetCurrentStrategy() string {
	return m.cfg.GetCurrentStrategyV2()
}

func (m *Manager) DefaultStrategyName() string {
	strategies := m.ListStrategies()
	for _, s := range strategies {
		if s.Filename == "general2.cmd" {
			return s.Filename
		}
	}
	if len(strategies) > 0 {
		return strategies[0].Filename
	}
	return "general2.cmd"
}

func parseStrategyArgs(strategyPath, binDir, luaDir, rootDir string) (string, error) {
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

		if strings.Contains(trimmed, "winws2.exe") {
			idx := strings.Index(trimmed, "winws2.exe")
			rest := trimmed[idx+len("winws2.exe"):]
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
		return "", fmt.Errorf("no arguments found after winws2.exe")
	}

	raw := strings.Join(argLines, " ")
	raw = strings.ReplaceAll(raw, "^", "")
	raw = strings.ReplaceAll(raw, "\t", " ")

	binSlash := filepath.ToSlash(binDir) + "/"
	luaSlash := filepath.ToSlash(luaDir) + "/"
	rootSlash := filepath.ToSlash(rootDir) + "/"

	raw = strings.ReplaceAll(raw, `%~dp0binaries\`, binSlash)
	raw = strings.ReplaceAll(raw, `%~dp0lua\`, luaSlash)
	raw = strings.ReplaceAll(raw, `%~dp0files\`, rootSlash+"files/")
	raw = strings.ReplaceAll(raw, `%~dp0`, rootSlash)

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