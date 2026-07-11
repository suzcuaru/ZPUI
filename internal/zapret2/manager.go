package zapret2

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
	"zpui/internal/executil"
	"zpui/internal/logger"
)

type Status string

const (
	StatusRunning Status = "running"
	StatusStopped Status = "stopped"
	StatusUnknown Status = "unknown"
)

const ServiceName = "zapret2"

type Manager struct {
	cfg    *config.Config
	log    *logger.Logger
	mu     sync.RWMutex
	cmd    *exec.Cmd
	stdout io.ReadCloser
	stderr io.ReadCloser
	status Status
	stopCh chan struct{}
	version string

	stopRequested bool
	OnCrash       func()
}

func NewManager(cfg *config.Config, log *logger.Logger) *Manager {
	m := &Manager{
		cfg:     cfg,
		log:     log,
		status:  StatusStopped,
		version: detectVersion(cfg),
	}

	if m.isServiceRunning() {
		m.status = StatusRunning
		m.log.Info("zapret2", "Zapret2 service is installed and running")
	}

	return m
}

func (m *Manager) GetStatus() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.isProcessRunning() {
		return StatusRunning
	}

	if m.isServiceRunning() {
		return StatusRunning
	}

	return StatusStopped
}

func (m *Manager) isProcessRunning() bool {
	return m.cmd != nil && m.cmd.Process != nil &&
		(m.cmd.ProcessState == nil || !m.cmd.ProcessState.Exited())
}

func (m *Manager) GetVersion() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.version
}

func (m *Manager) RefreshVersion() {
	ver := detectVersion(m.cfg)
	m.mu.Lock()
	m.version = ver
	m.mu.Unlock()
	m.log.Info("zapret2", "Version refreshed: "+ver)
}

func detectVersion(cfg *config.Config) string {
	binary := filepath.Join(cfg.GetZapret2Path(), "binaries", "winws2.exe")
	if _, err := os.Stat(binary); err != nil {
		return "not installed"
	}
	return "installed"
}

func (m *Manager) Start() error {
	if m.isServiceRunning() {
		m.log.Info("zapret2", "Service already running")
		return nil
	}

	if serviceExists(ServiceName) {
		m.log.Info("zapret2", "Starting service...")
		startCmd := executil.HiddenCmd("net", "start", ServiceName)
		out, err := startCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("net start %s: %w: %s", ServiceName, err, strings.TrimSpace(string(out)))
		}
		return nil
	}

	return m.StartWithStrategy(m.cfg.GetCurrentStrategyV2())
}

func (m *Manager) StartWithStrategy(strategyFile string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stopRequested = false

	if m.isProcessRunning() {
		m.stopLocked()
	}

	strategyPath := m.cfg.Zapret2StrategyPath(strategyFile)
	if _, err := os.Stat(strategyPath); os.IsNotExist(err) {
		return fmt.Errorf("strategy file not found: %s", strategyPath)
	}

	m.log.Info("zapret2", fmt.Sprintf("Starting with strategy: %s", strategyFile))

	args, err := parseStrategyArgs(strategyPath, m.cfg.Zapret2BinDir(), m.cfg.Zapret2LuaDir(), m.cfg.GetZapret2Path())
	if err != nil {
		return fmt.Errorf("parse strategy args: %w", err)
	}

	binDir := strings.TrimSuffix(m.cfg.Zapret2BinDir(), `\`)
	winws2 := filepath.Join(binDir, "winws2.exe")
	if _, err := os.Stat(winws2); os.IsNotExist(err) {
		return fmt.Errorf("winws2.exe not found: %s", winws2)
	}

	m.log.Info("zapret2", fmt.Sprintf("winws2.exe args: %s", args))

	argTokens := splitArgs(args)
	for i := range argTokens {
		argTokens[i] = strings.Trim(argTokens[i], `"`)
	}
	m.cmd = executil.HiddenCmd(winws2, argTokens...)
	m.cmd.Dir = filepath.Join(m.cfg.GetZapret2Path())

	var err2 error
	m.stdout, err2 = m.cmd.StdoutPipe()
	if err2 != nil {
		return fmt.Errorf("stdout pipe: %w", err2)
	}
	m.stderr, err2 = m.cmd.StderrPipe()
	if err2 != nil {
		return fmt.Errorf("stderr pipe: %w", err2)
	}

	m.stopCh = make(chan struct{})

	if err2 := m.cmd.Start(); err2 != nil {
		return fmt.Errorf("start process: %w", err2)
	}

	m.status = StatusRunning

	go m.streamOutput(m.stdout)
	go m.streamOutput(m.stderr)

	m.log.Info("zapret2", fmt.Sprintf("Started (PID: %d)", m.cmd.Process.Pid))

	go func() {
		err := m.cmd.Wait()
		m.mu.Lock()
		wasStopRequested := m.stopRequested
		m.status = StatusStopped
		m.mu.Unlock()
		if err != nil {
			m.log.Error("zapret2", fmt.Sprintf("Process exited with error: %v", err))
		} else {
			m.log.Info("zapret2", "Process exited normally")
		}
		if !wasStopRequested && err != nil && m.OnCrash != nil {
			go m.OnCrash()
		}
	}()

	time.Sleep(2 * time.Second)
	m.mu.Lock()
	stillRunning := m.status == StatusRunning
	m.mu.Unlock()
	if !stillRunning {
		return fmt.Errorf("winws2.exe exited immediately after start")
	}

	return nil
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	m.stopRequested = true
	m.mu.Unlock()

	if m.isServiceRunning() {
		m.log.Info("zapret2", "Stopping service...")
		stopService(ServiceName)
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopLocked()
}

func (m *Manager) stopLocked() error {
	if m.cmd != nil && m.cmd.Process != nil {
		m.log.Info("zapret2", "Stopping process...")
		if m.stopCh != nil {
			close(m.stopCh)
		}
		if err := m.cmd.Process.Kill(); err != nil {
			m.log.Error("zapret2", fmt.Sprintf("Kill process error: %v", err))
		}
		killWinws2()
		m.status = StatusStopped
		m.log.Info("zapret2", "Process stopped")
	}
	return nil
}

func (m *Manager) Restart() error {
	strategy := m.cfg.GetCurrentStrategyV2()
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

func (m *Manager) isServiceRunning() bool {
	cmd := executil.HiddenCmd("sc", "query", ServiceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	outStr := string(output)
	if strings.Contains(outStr, "RUNNING") {
		return true
	}
	if strings.Contains(outStr, "\xd0\xa0\xd0\x90\xd0\x91\xd0\x9e\xd0\xa2\xd0\x90\xd0\x95\xd0\xa2") {
		return true
	}
	for _, line := range strings.Split(outStr, "\n") {
		line = strings.TrimSpace(line)
		if (strings.HasPrefix(line, "STATE") || strings.HasPrefix(line, "\xd0\xa1\xd0\x9e\xd0\xa1\xd0\xa2")) && strings.Contains(line, ": 4 ") {
			return true
		}
	}
	return false
}

func killWinws2() {
	kill := executil.HiddenCmd("taskkill", "/IM", "winws2.exe", "/F")
	kill.Run()
}

func (m *Manager) Teardown() {
	m.log.Info("zapret2", "Teardown: stopping service and WinDivert drivers")
	stopService(ServiceName)
	killWinws2()
	time.Sleep(1 * time.Second)
	deleteService(ServiceName)
}

func (m *Manager) VerifyFiles() *VerifyResult {
	dir := m.cfg.GetZapret2Path()
	result := &VerifyResult{
		Version:    m.GetVersion(),
		AllPresent: true,
	}
	for _, rel := range essentialFiles {
		full := filepath.Join(dir, rel)
		_, err := os.Stat(full)
		exists := err == nil
		if !exists {
			result.AllPresent = false
		}
		result.Files = append(result.Files, FileCheckResult{
			Exists: exists,
			Path:   rel,
		})
	}
	return result
}

var essentialFiles = []string{
	"binaries/winws2.exe",
}

type FileCheckResult struct {
	Exists bool   `json:"exists"`
	Path   string `json:"path"`
}

type VerifyResult struct {
	AllPresent bool              `json:"all_present"`
	Version    string            `json:"version"`
	Files      []FileCheckResult `json:"files"`
}

func (m *Manager) FindZapret2Dir(searchDir string) string {
	filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && info.Name() == "binaries" {
			winws2 := filepath.Join(path, "winws2.exe")
			if _, err := os.Stat(winws2); err == nil {
				m.cfg.SetZapret2Path(filepath.Dir(path))
				return io.EOF
			}
		}
		return nil
	})
	return m.cfg.GetZapret2Path()
}