package zapret

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Strategy struct {
	Name     string `json:"name"`
	Filename string `json:"filename"`
	Current  bool   `json:"current"`
}

func (m *Manager) ListStrategies() []Strategy {
	zapretDir := m.cfg.GetZapretPath()
	entries, err := os.ReadDir(zapretDir)
	if err != nil {
		return nil
	}

	current := m.cfg.GetCurrentStrategy()
	var strategies []Strategy

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "general") || !strings.HasSuffix(name, ".bat") {
			continue
		}
		if strings.HasPrefix(name, "service") {
			continue
		}

		displayName := strings.TrimSuffix(name, ".bat")
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
	if _, err := os.Stat(m.cfg.StrategyPath(filename)); os.IsNotExist(err) {
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
	return m.cfg.GetCurrentStrategy()
}

type ResourceResult struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	OK    bool   `json:"ok"`
	Ms    int64  `json:"ms"`
}

type AutoTestResult struct {
	Type        string           `json:"type"`
	Strategy    string           `json:"strategy,omitempty"`
	Current     int              `json:"current,omitempty"`
	Total       int              `json:"total,omitempty"`
	Phase       string           `json:"phase,omitempty"`
	Message     string           `json:"message,omitempty"`
	DiscordOK   bool             `json:"discord_ok,omitempty"`
	YouTubeOK   bool             `json:"youtube_ok,omitempty"`
	ResourcesOK int              `json:"resources_ok,omitempty"`
	ResourcesN  int              `json:"resources_n,omitempty"`
	ResponseMs  int64            `json:"response_ms,omitempty"`
	Resources   []string         `json:"resources,omitempty"`
	ResourcesDetail []ResourceResult `json:"resources_detail,omitempty"`
	Error       string           `json:"error,omitempty"`
}

var (
	autoTestMu     sync.Mutex
	autoTestActive bool
	autoTestCancel context.CancelFunc
)

func (m *Manager) IsAutoTestRunning() bool {
	autoTestMu.Lock()
	defer autoTestMu.Unlock()
	return autoTestActive
}

func (m *Manager) CancelAutoTest() {
	autoTestMu.Lock()
	defer autoTestMu.Unlock()
	if autoTestCancel != nil {
		autoTestCancel()
		autoTestCancel = nil
	}
	autoTestActive = false
}

type testResultData struct {
	Strategy  string           `json:"strategy"`
	Resources []ResourceResult `json:"resources"`
	Ok        int              `json:"ok"`
	Total     int              `json:"total"`
	AvgMs     int64            `json:"avg_ms"`
}

func (m *Manager) RunAutoTest(ctx context.Context, results chan<- AutoTestResult, done chan<- struct{}) {
	defer close(done)

	autoTestMu.Lock()
	if autoTestActive {
		autoTestMu.Unlock()
		results <- AutoTestResult{Type: "done", Error: "Автотест уже запущен"}
		return
	}
	autoTestActive = true
	testCtx, cancel := context.WithCancel(ctx)
	autoTestCancel = cancel
	autoTestMu.Unlock()

	defer func() {
		autoTestMu.Lock()
		autoTestActive = false
		autoTestCancel = nil
		autoTestMu.Unlock()
	}()

	strategies := m.ListStrategies()
	originalStrategy := m.cfg.GetCurrentStrategy()
	resources := m.loadTestTargets()

	m.log.Info("strategy", fmt.Sprintf("Auto-test started: %d strategies, %d resources", len(strategies), len(resources)))

	results <- AutoTestResult{
		Type:    "info",
		Total:   len(strategies),
		Message: fmt.Sprintf("Найдено стратегий: %d, ресурсов для проверки: %d", len(strategies), len(resources)),
	}

	httpClient := &http.Client{
		Timeout: 8 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
			DisableKeepAlives:   true,
			MaxIdleConns:        1,
			DialContext:         (&net.Dialer{Timeout: 4 * time.Second}).DialContext,
		},
	}

	var allResults []testResultData
	jsonPath := filepath.Join(m.cfg.LogsDir(), "auto_test_results.json")

	for i, s := range strategies {
		select {
		case <-testCtx.Done():
			results <- AutoTestResult{Type: "info", Message: "Тестирование отменено"}
			goto restore
		default:
		}

		m.log.Info("strategy", fmt.Sprintf("[%d/%d] Testing %s", i+1, len(strategies), s.Name))

		results <- AutoTestResult{
			Type:     "progress",
			Current:  i + 1,
			Total:    len(strategies),
			Strategy: s.Filename,
			Phase:    "start",
			Message:  fmt.Sprintf("[%d/%d] %s", i+1, len(strategies), s.Name),
		}

		proc, err := m.startWinws(s.Filename)
		if err != nil {
			results <- AutoTestResult{Type: "result", Strategy: s.Filename, Error: err.Error()}
			continue
		}

		if !sleepCtx(testCtx, 3*time.Second) {
				proc.Process.Kill()
				proc.Wait()
				goto restore
			}

		if proc.ProcessState != nil && proc.ProcessState.Exited() {
			results <- AutoTestResult{Type: "result", Strategy: s.Filename, Error: "winws exited immediately"}
			continue
		}

		results <- AutoTestResult{
			Type:     "progress",
			Current:  i + 1,
			Total:    len(strategies),
			Strategy: s.Filename,
			Phase:    "test",
			Message:  fmt.Sprintf("[%d/%d] %s — тестирование ресурсов", i+1, len(strategies), s.Name),
		}

		var detail []ResourceResult
		var totalMs int64
		okCount := 0
		for _, res := range resources {
			select {
			case <-testCtx.Done():
				proc.Process.Kill()
				proc.Wait()
				goto restore
			default:
			}
			ok, ms := testURL(httpClient, res.URL)
			detail = append(detail, ResourceResult{Name: res.Name, URL: res.URL, OK: ok, Ms: ms})
			if ok {
				okCount++
				totalMs += ms
			}
		}

		avgMs := int64(0)
		if okCount > 0 {
			avgMs = totalMs / int64(okCount)
		}

		proc.Process.Kill()
		proc.Wait()

		d := testResultData{
			Strategy:  s.Filename,
			Resources: detail,
			Ok:        okCount,
			Total:     len(resources),
			AvgMs:     avgMs,
		}
		allResults = append(allResults, d)

		results <- AutoTestResult{
			Type:        "result",
			Strategy:    s.Filename,
			ResourcesOK: okCount,
			ResourcesN:  len(resources),
			ResponseMs:  avgMs,
			ResourcesDetail: detail,
		}

		if !sleepCtx(testCtx, 1*time.Second) {
			goto restore
		}
	}

	sort.Slice(allResults, func(i, j int) bool {
		scoreI := float64(allResults[i].Ok) / float64(allResults[i].Total)
		scoreJ := float64(allResults[j].Ok) / float64(allResults[j].Total)
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		return allResults[i].AvgMs < allResults[j].AvgMs
	})

	if data, err := json.MarshalIndent(allResults, "", "  "); err == nil {
		os.WriteFile(jsonPath, data, 0644)
		m.log.Info("strategy", fmt.Sprintf("Results written to %s", jsonPath))
	}

	results <- AutoTestResult{Type: "info", Message: "Автотест завершён"}
	if len(allResults) > 0 {
		best := allResults[0]
		results <- AutoTestResult{
			Type:        "result",
			Strategy:    best.Strategy,
			ResourcesOK: best.Ok,
			ResourcesN:  best.Total,
			ResponseMs:  best.AvgMs,
			ResourcesDetail: best.Resources,
		}
		results <- AutoTestResult{
			Type:    "info",
			Message: fmt.Sprintf("Лучшая: %s (%d/%d ресурсов, %d мс)", best.Strategy, best.Ok, best.Total, best.AvgMs),
		}
	}

restore:
	results <- AutoTestResult{Type: "info", Message: "Восстановление исходной стратегии..."}
	if originalStrategy != "" {
		m.StartWithStrategy(originalStrategy)
	}
	m.log.Info("strategy", "Auto-test complete")
	results <- AutoTestResult{Type: "done"}
}

func (m *Manager) startWinws(strategyFile string) (*exec.Cmd, error) {
	strategyPath := m.cfg.StrategyPath(strategyFile)
	if _, err := os.Stat(strategyPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("strategy file not found: %s", strategyPath)
	}

	args, err := parseStrategyArgs(strategyPath, m.cfg.BinDir(), m.cfg.ListsDir())
	if err != nil {
		return nil, fmt.Errorf("parse strategy: %w", err)
	}

	binDir := strings.TrimSuffix(m.cfg.BinDir(), `\`)
	winws := filepath.Join(binDir, "winws.exe")
	if _, err := os.Stat(winws); os.IsNotExist(err) {
		return nil, fmt.Errorf("winws.exe not found: %s", winws)
	}

	argTokens := splitArgs(args)
	for i := range argTokens {
		argTokens[i] = strings.Trim(argTokens[i], `"`)
	}

	cmd := exec.Command(winws, argTokens...)
	cmd.Dir = binDir

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start winws: %w", err)
	}

	return cmd, nil
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}

func testURL(client *http.Client, url string) (bool, int64) {
	start := time.Now()
	resp, err := client.Get(url)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		return false, elapsed
	}
	resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 500, elapsed
}

type testTarget struct {
	Name string
	URL  string
}

func (m *Manager) loadTestTargets() []testTarget {
	path := filepath.Join(m.cfg.GetZapretPath(), "utils", "targets.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		m.log.Warn("strategy", fmt.Sprintf("Cannot read targets.txt (%s), fallback to defaults", path))
		return []testTarget{
			{Name: "DiscordMain", URL: "https://discord.com"},
			{Name: "YouTubeWeb", URL: "https://www.youtube.com"},
			{Name: "GoogleMain", URL: "https://www.google.com"},
			{Name: "CloudflareWeb", URL: "https://www.cloudflare.com"},
		}
	}

	var targets []testTarget
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") || strings.HasPrefix(line, ";") {
			continue
		}
		if !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		name := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, `" `)
		if name == "" || val == "" {
			continue
		}
		if strings.HasPrefix(val, "PING:") {
			continue
		}
		if !strings.HasPrefix(val, "http://") && !strings.HasPrefix(val, "https://") {
			continue
		}
		targets = append(targets, testTarget{Name: name, URL: val})
	}

	if len(targets) == 0 {
		targets = append(targets, testTarget{Name: "DiscordMain", URL: "https://discord.com"})
		targets = append(targets, testTarget{Name: "YouTubeWeb", URL: "https://www.youtube.com"})
	}

	return targets
}

func (m *Manager) LoadGameFilter() (mode string, tcp, udp string) {
	flagFile := filepath.Join(m.cfg.GetZapretPath(), "utils", "game_filter.enabled")
	tcp = "12"
	udp = "12"
	mode = "disabled"

	data, err := os.ReadFile(flagFile)
	if err != nil {
		return mode, tcp, udp
	}

	line := strings.TrimSpace(string(data))
	switch strings.ToLower(line) {
	case "all":
		mode = "all"
		tcp = "1024-65535"
		udp = "1024-65535"
	case "tcp":
		mode = "tcp"
		tcp = "1024-65535"
	case "udp":
		mode = "udp"
		udp = "1024-65535"
	}
	return mode, tcp, udp
}

func (m *Manager) SetGameFilter(mode string) error {
	flagFile := filepath.Join(m.cfg.GetZapretPath(), "utils", "game_filter.enabled")

	switch mode {
	case "disabled":
		return os.Remove(flagFile)
	case "all", "tcp", "udp":
		return os.WriteFile(flagFile, []byte(mode), 0644)
	default:
		return fmt.Errorf("invalid game filter mode: %s", mode)
	}
}
