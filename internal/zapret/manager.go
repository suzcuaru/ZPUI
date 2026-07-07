package zapret

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"zpui/internal/executil"
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
	cfg           *config.Config
	log           *logger.Logger
	mu            sync.RWMutex
	cmd           *exec.Cmd
	stdout        io.ReadCloser
	stderr        io.ReadCloser
	status        Status
	stopCh        chan struct{}
	version       string
	gameFilterTCP string
	gameFilterUDP string
}

func NewManager(cfg *config.Config, log *logger.Logger) *Manager {
	m := &Manager{
		cfg:     cfg,
		log:     log,
		status:  StatusStopped,
		version: detectZapretVersion(cfg),
	}

	_, m.gameFilterTCP, m.gameFilterUDP = m.LoadGameFilter()

	if m.isServiceRunning() {
		m.status = StatusRunning
		m.log.Info("zapret", "Zapret service is installed and running")
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
	ver := detectZapretVersion(m.cfg)
	m.mu.Lock()
	m.version = ver
	m.mu.Unlock()
	m.log.Info("zapret", "Version refreshed: "+ver)
}

func (m *Manager) Start() error {
	if m.isServiceRunning() {
		m.log.Info("zapret", "Service already running")
		return nil
	}

	if serviceExists("zapret") {
		m.log.Info("zapret", "Starting service...")
		startCmd := executil.HiddenCmd("net", "start", "zapret")
		out, err := startCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("net start zapret: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}

	return m.StartWithStrategy(m.cfg.GetCurrentStrategy())
}

func (m *Manager) StartWithStrategy(strategyFile string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isProcessRunning() {
		m.stopLocked()
	}

	strategyPath := m.cfg.StrategyPath(strategyFile)
	if _, err := os.Stat(strategyPath); os.IsNotExist(err) {
		return fmt.Errorf("strategy file not found: %s", strategyPath)
	}

	m.log.Info("zapret", fmt.Sprintf("Starting zapret with strategy: %s", strategyFile))

	args, err := parseStrategyArgs(strategyPath, m.cfg.BinDir(), m.cfg.ListsDir(), m.gameFilterTCP, m.gameFilterUDP)
	if err != nil {
		return fmt.Errorf("parse strategy args: %w", err)
	}

	binDir := strings.TrimSuffix(m.cfg.BinDir(), `\`)
	winws := filepath.Join(binDir, "winws.exe")
	if _, err := os.Stat(winws); os.IsNotExist(err) {
		return fmt.Errorf("winws.exe not found: %s", winws)
	}

	m.log.Info("zapret", fmt.Sprintf("winws.exe args: %s", args))

	argTokens := splitArgs(args)
	for i := range argTokens {
		argTokens[i] = strings.Trim(argTokens[i], `"`)
	}
	m.cmd = executil.HiddenCmd(winws, argTokens...)
	m.cmd.Dir = binDir

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
	if m.isServiceRunning() {
		m.log.Info("zapret", "Stopping service...")
		stopService("zapret")
		return nil
	}

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

func (m *Manager) isServiceRunning() bool {
	cmd := executil.HiddenCmd("sc", "query", "zapret")
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
		if line == "" {
			continue
		}
		// Пропускаем комментарии (.bat): REM, ::, #
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "REM ") || strings.HasPrefix(upper, "::") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(upper, "LOCAL_VERSION") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				v := strings.TrimSpace(parts[1])
				// Обрезаем возможный inline-комментарий
				if idx := strings.IndexAny(v, "&"); idx >= 0 {
					v = v[:idx]
				}
				v = strings.Trim(strings.TrimSpace(v), `"`)
				if v != "" {
					return v
				}
			}
		}
	}
	return "unknown"
}

func killWinws() {
	kill := executil.HiddenCmd("taskkill", "/IM", "winws.exe", "/F")
	kill.Run()
}

// Teardown полностью останавливает и удаляет службу zapret вместе с драйверами WinDivert.
// Используется при ручном удалении системной службы (RemoveSystemZapretService).
func (m *Manager) Teardown() {
	m.log.Info("zapret", "Teardown: stopping zapret service and WinDivert drivers")
	stopService("zapret")
	killWinws()
	time.Sleep(1 * time.Second)
	deleteService("zapret")
	removeWinDivertDrivers()
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

// InstallResult — результат установки службы запрета с логом.
type InstallResult struct {
	Success  bool     `json:"success"`
	Version  string   `json:"version"`
	Strategy string   `json:"strategy"`
	Running  bool     `json:"running"`
	Errors   []string `json:"errors,omitempty"`
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

var essentialFiles = []string{
	"bin/winws.exe",
	"bin/WinDivert.dll",
	"bin/WinDivert64.sys",
	"service.bat",
	"general.bat",
}

func (m *Manager) VerifyFiles() *VerifyResult {
	zapretDir := m.cfg.GetZapretPath()
	result := &VerifyResult{
		Version:    m.GetVersion(),
		AllPresent: true,
	}
	for _, rel := range essentialFiles {
		full := filepath.Join(zapretDir, rel)
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

func (m *Manager) verifyEssential() error {
	vr := m.VerifyFiles()
	if vr.AllPresent {
		return nil
	}
	var missing []string
	for _, fc := range vr.Files {
		if !fc.Exists {
			missing = append(missing, fc.Path)
		}
	}
	return fmt.Errorf("отсутствуют обязательные файлы: %s", strings.Join(missing, ", "))
}

// DefaultStrategyName возвращает стратегию по умолчанию (первый general* ALT).
func (m *Manager) DefaultStrategyName() string {
	strategies := m.ListStrategies()
	preferred := "general (ALT).bat"
	for _, s := range strategies {
		if s.Filename == preferred {
			return preferred
		}
	}
	if len(strategies) > 0 {
		return strategies[0].Filename
	}
	return "general.bat"
}

// EnsureUserLists создаёт пользовательские списки со значениями по умолчанию,
// если они отсутствуют (аналог load_user_lists в service.bat). Без этих файлов
// winws.exe падает с exit status 1, ссылаясь на отсутствующий ipset-exclude-user.
func (m *Manager) EnsureUserLists() error {
	listsDir := m.cfg.ListsDir()
	if err := os.MkdirAll(listsDir, 0755); err != nil {
		return fmt.Errorf("create lists dir: %w", err)
	}
	defaults := map[string]string{
		"ipset-exclude-user.txt": "203.0.113.113/32",
		"list-general-user.txt":  "domain.example.abc",
		"list-exclude-user.txt":  "domain.example.abc",
	}
	var created []string
	for name, content := range defaults {
		path := filepath.Join(listsDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.WriteFile(path, []byte(content+"\n"), 0644); err != nil {
				return fmt.Errorf("create %s: %w", name, err)
			}
			created = append(created, name)
		}
	}
	if len(created) > 0 {
		m.log.Info("zapret", "Созданы пользовательские списки: "+strings.Join(created, ", "))
	}
	return nil
}

// InstallServiceLogged устанавливает службу запрета, записывая процесс в
// logs/install.log (перезаписываемый, не накапливается), и проверяет, что
// служба создалась и отвечает.
func (m *Manager) InstallServiceLogged(strategyFile string) (*InstallResult, error) {
	logsDir := m.cfg.LogsDir()
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, fmt.Errorf("create logs dir: %w", err)
	}
	logPath := filepath.Join(logsDir, "install.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open install log: %w", err)
	}
	defer f.Close()

	wlog := func(step, msg string) {
		ts := time.Now().Format("2006-01-02 15:04:05")
		fmt.Fprintf(f, "[%s] [%s] %s\n", ts, step, msg)
		m.log.Info("install", "["+step+"] "+msg)
	}

	if strategyFile == "" {
		strategyFile = m.cfg.GetCurrentStrategy()
	}
	if strategyFile == "" {
		strategyFile = m.DefaultStrategyName()
	}

	wlog("init", fmt.Sprintf("Установка службы запрета, стратегия: %s", strategyFile))

	wlog("lists", "Проверка пользовательских списков...")
	if err := m.EnsureUserLists(); err != nil {
		wlog("warn", "Не удалось создать списки: "+err.Error())
	}

	strategyPath := m.cfg.StrategyPath(strategyFile)
	if _, err := os.Stat(strategyPath); err != nil {
		wlog("error", "Файл стратегии не найден: "+strategyPath)
		return &InstallResult{Errors: []string{"стратегия не найдена: " + strategyFile}}, nil
	}
	winws := filepath.Join(m.cfg.BinDir(), "winws.exe")
	if _, err := os.Stat(winws); err != nil {
		wlog("error", "winws.exe не найден: "+winws)
		return &InstallResult{Errors: []string{"winws.exe не найден — запрет не установлен"}}, nil
	}

	wlog("service", "Остановка и удаление прежней службы...")
	m.Stop()
	m.serviceRemove()

	wlog("service", "Создание и запуск службы...")
	if err := m.serviceCreate(strategyFile); err != nil {
		wlog("error", "Ошибка создания службы: "+err.Error())
		return &InstallResult{Errors: []string{err.Error()}}, nil
	}
	wlog("service", "Служба создана и запущена")

	wlog("verify", "Проверка состояния службы...")
	time.Sleep(1500 * time.Millisecond)
	running := m.isServiceRunning()
	svc := m.GetServiceStatus()
	wlog("verify", fmt.Sprintf("installed=%v running=%v pid=%d", svc.Installed, running, svc.PID))

	if !running {
		wlog("warn", "Служба не отвечает на запрос (RUNNING)")
	}

	m.cfg.SetCurrentStrategy(strategyFile)

	return &InstallResult{
		Success:  true,
		Version:  m.GetVersion(),
		Strategy: strategyFile,
		Running:  running,
	}, nil
}

// ============================================================
// BACKUP / RESTORE состояния zapret
// ============================================================

// BackupSnapshot — слепок состояния zapret перед обновлением.
type BackupSnapshot struct {
	Timestamp  time.Time         `json:"timestamp"`
	Strategy   string            `json:"strategy"`
	Version    string            `json:"version"`
	UserLists  map[string]string `json:"user_lists"`
	GameFilter string            `json:"game_filter"`
}

// CaptureState создаёт слепок текущего состояния: стратегия, версия,
// все пользовательские списки (*-user.txt) и режим game_filter.
func (m *Manager) CaptureState() *BackupSnapshot {
	snap := &BackupSnapshot{
		Timestamp: time.Now(),
		Strategy:  m.cfg.GetCurrentStrategy(),
		Version:   m.GetVersion(),
		UserLists: make(map[string]string),
	}

	listsDir := m.cfg.ListsDir()
	if entries, err := os.ReadDir(listsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasSuffix(name, "-user.txt") {
				if data, err := os.ReadFile(filepath.Join(listsDir, name)); err == nil {
					snap.UserLists[name] = string(data)
				}
			}
		}
	}

	if gfPath := filepath.Join(m.cfg.GetZapretPath(), "utils", "game_filter.enabled"); true {
		if data, err := os.ReadFile(gfPath); err == nil {
			snap.GameFilter = strings.TrimSpace(string(data))
		}
	}

	m.log.Info("zapret", fmt.Sprintf("CaptureState: стратегия=%s, списков=%d, game_filter=%q, версия=%s", snap.Strategy, len(snap.UserLists), snap.GameFilter, snap.Version))
	return snap
}

// RestoreState восстанавливает состояние из слепка: пользовательские списки,
// game_filter, стратегию и работоспособность службы.
func (m *Manager) RestoreState(snap *BackupSnapshot) error {
	if snap == nil {
		return nil
	}

	listsDir := m.cfg.ListsDir()
	os.MkdirAll(listsDir, 0755)

	for name, content := range snap.UserLists {
		if err := os.WriteFile(filepath.Join(listsDir, name), []byte(content), 0644); err != nil {
			m.log.Warn("zapret", "Восстановление списка "+name+": "+err.Error())
		}
	}

	m.EnsureUserLists()

	if snap.GameFilter != "" {
		gfPath := filepath.Join(m.cfg.GetZapretPath(), "utils", "game_filter.enabled")
		os.MkdirAll(filepath.Dir(gfPath), 0755)
		if err := os.WriteFile(gfPath, []byte(snap.GameFilter+"\n"), 0644); err != nil {
			m.log.Warn("zapret", "Восстановление game_filter: "+err.Error())
		}
	}

	if snap.Strategy != "" {
		m.cfg.SetCurrentStrategy(snap.Strategy)
		strategyPath := m.cfg.StrategyPath(snap.Strategy)
		winws := filepath.Join(m.cfg.BinDir(), "winws.exe")
		if _, err := os.Stat(strategyPath); err == nil {
			if _, err := os.Stat(winws); err == nil {
				m.log.Info("zapret", "RestoreState: переустановка службы "+snap.Strategy)
				if err := m.InstallService(snap.Strategy); err != nil {
					m.log.Warn("zapret", "RestoreState: служба не установлена: "+err.Error())
				}
			} else {
				m.log.Warn("zapret", "RestoreState: winws.exe отсутствует — запуск пропущен")
			}
		} else {
			m.log.Warn("zapret", "RestoreState: стратегия не найдена: "+snap.Strategy)
		}
	}

	m.log.Info("zapret", "RestoreState завершён")
	return nil
}
