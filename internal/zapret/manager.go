package zapret

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"zpui/internal/config"
	"zpui/internal/logger"
)

type Status string

const (
	StatusRunning Status = "running"
	StatusStopped Status = "stopped"
	StatusUnknown Status = "unknown"
)

type Manager struct {
	cfg     *config.Config
	log     *logger.Logger
	mu      sync.RWMutex
	cmd     *exec.Cmd
	stdout  io.ReadCloser
	stderr  io.ReadCloser
	status  Status
	stopCh  chan struct{}
	version string
}

func NewManager(cfg *config.Config, log *logger.Logger) *Manager {
	m := &Manager{
		cfg:     cfg,
		log:     log,
		status:  StatusStopped,
		version: detectZapretVersion(cfg),
	}

	if m.isServiceInstalled() {
		m.status = StatusRunning
		m.log.Info("zapret", "Zapret service is installed and running")
	}

	return m
}

func (m *Manager) GetStatus() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.isServiceInstalled() {
		return StatusRunning
	}

	if m.cmd != nil && m.cmd.Process != nil {
		if m.cmd.ProcessState == nil || !m.cmd.ProcessState.Exited() {
			return StatusRunning
		}
	}

	return StatusStopped
}

func (m *Manager) GetVersion() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.version
}

func (m *Manager) Start() error {
	return m.StartWithStrategy(m.cfg.GetCurrentStrategy())
}

func (m *Manager) StartWithStrategy(strategyFile string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.GetStatus() == StatusRunning {
		m.stopLocked()
	}

	strategyPath := m.cfg.StrategyPath(strategyFile)
	if _, err := os.Stat(strategyPath); os.IsNotExist(err) {
		return fmt.Errorf("strategy file not found: %s", strategyPath)
	}

	m.log.Info("zapret", fmt.Sprintf("Starting zapret with strategy: %s", strategyFile))

	m.cmd = exec.Command("cmd.exe", "/c", strategyPath)
	m.cmd.Dir = m.cfg.GetZapretPath()

	var err error
	m.stdout, err = m.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	m.stderr, err = m.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	m.stopCh = make(chan struct{})

	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("start process: %w", err)
	}

	m.status = StatusRunning

	go m.streamOutput(m.stdout)
	go m.streamOutput(m.stderr)

	m.log.Info("zapret", fmt.Sprintf("Zapret started (PID: %d)", m.cmd.Process.Pid))

	go func() {
		err := m.cmd.Wait()
		m.mu.Lock()
		defer m.mu.Unlock()
		m.status = StatusStopped
		if err != nil {
			m.log.Error("zapret", fmt.Sprintf("Process exited with error: %v", err))
		} else {
			m.log.Info("zapret", "Process exited normally")
		}
	}()

	return nil
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopLocked()
}

func (m *Manager) stopLocked() error {
	if m.cmd != nil && m.cmd.Process != nil {
		m.log.Info("zapret", "Stopping zapret process...")
		if m.stopCh != nil {
			close(m.stopCh)
		}
		if err := m.cmd.Process.Kill(); err != nil {
			m.log.Error("zapret", fmt.Sprintf("Kill process error: %v", err))
		}
		killWinws()
		m.status = StatusStopped
		m.log.Info("zapret", "Zapret process stopped")
	}
	return nil
}

func (m *Manager) Restart() error {
	strategy := m.cfg.GetCurrentStrategy()
	if err := m.Stop(); err != nil {
		return err
	}
	time.Sleep(2 * time.Second)
	return m.StartWithStrategy(strategy)
}

func (m *Manager) streamOutput(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		select {
		case <-m.stopCh:
			return
		default:
			line := scanner.Text()
			m.log.WriteZapretOutput(line)
		}
	}
}

func (m *Manager) isServiceInstalled() bool {
	cmd := exec.Command("sc", "query", "zapret")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), "RUNNING")
}

func detectZapretVersion(cfg *config.Config) string {
	serviceFile := filepath.Join(cfg.GetZapretPath(), "service.bat")
	data, err := os.ReadFile(serviceFile)
	if err != nil {
		return "unknown"
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "LOCAL_VERSION") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				v := strings.Trim(strings.TrimSpace(parts[1]), `"`)
				return v
			}
		}
	}
	return "unknown"
}

func killWinws() {
	kill := exec.Command("taskkill", "/IM", "winws.exe", "/F")
	kill.Run()
}

func (m *Manager) FindZapretDir(searchDir string) string {
	filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && info.Name() == "bin" {
			winws := filepath.Join(path, "winws.exe")
			if _, err := os.Stat(winws); err == nil {
				m.cfg.SetZapretPath(filepath.Dir(path))
				return io.EOF
			}
		}
		return nil
	})
	return m.cfg.GetZapretPath()
}
