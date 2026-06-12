package zapret

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
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
	return m.cfg.SetCurrentStrategy(filename)
}

func (m *Manager) GetCurrentStrategy() string {
	return m.cfg.GetCurrentStrategy()
}

type AutoTestResult struct {
	Type        string   `json:"type"`
	Strategy    string   `json:"strategy,omitempty"`
	Current     int      `json:"current,omitempty"`
	Total       int      `json:"total,omitempty"`
	Phase       string   `json:"phase,omitempty"`
	Message     string   `json:"message,omitempty"`
	DiscordOK   bool     `json:"discord_ok,omitempty"`
	YouTubeOK   bool     `json:"youtube_ok,omitempty"`
	ResourcesOK int      `json:"resources_ok,omitempty"`
	ResourcesN  int      `json:"resources_n,omitempty"`
	ResponseMs  int64    `json:"response_ms,omitempty"`
	Resources   []string `json:"resources,omitempty"`
	Error       string   `json:"error,omitempty"`
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
	resources := m.loadTestResources()

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

	for i, s := range strategies {
		select {
		case <-testCtx.Done():
			results <- AutoTestResult{Type: "info", Message: "Тестирование отменено"}
			goto restore
		default:
		}

		results <- AutoTestResult{
			Type:     "progress",
			Current:  i + 1,
			Total:    len(strategies),
			Strategy: s.Filename,
			Phase:    "stop",
			Message:  fmt.Sprintf("[%d/%d] %s — остановка Zapret...", i+1, len(strategies), s.Name),
		}

		m.Stop()
		if !sleepCtx(testCtx, 2*time.Second) {
			goto restore
		}

		results <- AutoTestResult{
			Type:     "progress",
			Current:  i + 1,
			Total:    len(strategies),
			Strategy: s.Filename,
			Phase:    "start",
			Message:  fmt.Sprintf("[%d/%d] %s — запуск стратегии...", i+1, len(strategies), s.Name),
		}

		if err := m.StartWithStrategy(s.Filename); err != nil {
			results <- AutoTestResult{
				Type:     "result",
				Strategy: s.Filename,
				Error:    fmt.Sprintf("Не удалось запустить: %v", err),
			}
			continue
		}

		results <- AutoTestResult{
			Type:     "progress",
			Current:  i + 1,
			Total:    len(strategies),
			Strategy: s.Filename,
			Phase:    "wait",
			Message:  fmt.Sprintf("[%d/%d] %s — ожидание активации (5с)...", i+1, len(strategies), s.Name),
		}
		if !sleepCtx(testCtx, 5*time.Second) {
			goto restore
		}

		if m.GetStatus() != StatusRunning {
			results <- AutoTestResult{
				Type:     "result",
				Strategy: s.Filename,
				Error:    "Стратегия не запустилась",
			}
			continue
		}

		results <- AutoTestResult{
			Type:     "progress",
			Current:  i + 1,
			Total:    len(strategies),
			Strategy: s.Filename,
			Phase:    "test",
			Message:  fmt.Sprintf("[%d/%d] %s — тестирование доменов...", i+1, len(strategies), s.Name),
		}

		result := AutoTestResult{Type: "result", Strategy: s.Filename}

		discordOK, discordMs := testURL(httpClient, "https://discord.com/api/v10/gateway")
		youtubeOK, _ := testURL(httpClient, "https://www.youtube.com")
		result.DiscordOK = discordOK
		result.YouTubeOK = youtubeOK
		result.ResponseMs = discordMs

		okCount := 0
		var okResources []string
		for _, res := range resources {
			select {
			case <-testCtx.Done():
				goto restore
			default:
			}
			ok, _ := testURL(httpClient, "https://"+res)
			if ok {
				okCount++
				okResources = append(okResources, res)
			}
		}
		result.ResourcesOK = okCount
		result.ResourcesN = len(resources)
		result.Resources = okResources

		results <- AutoTestResult{
			Type:     "progress",
			Current:  i + 1,
			Total:    len(strategies),
			Strategy: s.Filename,
			Phase:    "save",
			Message:  fmt.Sprintf("[%d/%d] %s — результаты сохранены (доменов: %d/%d)", i+1, len(strategies), s.Name, okCount, len(resources)),
		}

		results <- result

		m.Stop()
		if !sleepCtx(testCtx, 2*time.Second) {
			goto restore
		}
	}

restore:
	results <- AutoTestResult{
		Type:    "info",
		Message: "Восстановление исходной стратегии...",
	}
	m.Stop()
	time.Sleep(1 * time.Second)
	if originalStrategy != "" {
		m.StartWithStrategy(originalStrategy)
	}
	m.log.Info("strategy", "Auto-test complete")
	results <- AutoTestResult{Type: "done"}
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

func (m *Manager) loadTestResources() []string {
	var resources []string
	seen := map[string]bool{}

	for _, file := range []string{"list-general.txt", "list-general-user.txt"} {
		data, err := os.ReadFile(filepath.Join(m.cfg.ListsDir(), file))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") && !seen[line] {
				seen[line] = true
				resources = append(resources, line)
			}
		}
	}

	if len(resources) > 20 {
		resources = resources[:20]
	}

	return resources
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
