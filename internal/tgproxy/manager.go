package tgproxy

import (
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"zpui/internal/config"
	"zpui/internal/executil"
	"zpui/internal/logger"
)

// Version — fallback для go test/dev. В рантайме версия читается из
// components/tgproxy/version.txt (пишется сборкой из __version__ апстрима);
// ldflags-инжекция из build.bat обновляет эту переменную как запасную.
var Version = "1.11.3"

const (
	exeSubPath     = "components\\tgproxy\\ZPUI-TgProxy.exe"
	verSubPath     = "components\\tgproxy\\version.txt"
	readyTimeout   = 20 * time.Second
	restartWindow  = 60 * time.Second
	maxRestarts    = 5
	tgProxyLogName = "tgproxy.log"
)

// Manager управляет дочерним процессом ZPUI-TgProxy.exe (headless-сборка
// tg-ws-proxy): spawn/kill, ready-гейт по TCP, watchdog рестартов,
// статистика и per-IP сессии — из лога процесса.
type Manager struct {
	cfg *config.Config
	log *logger.Logger

	mu          sync.Mutex
	cmd         *exec.Cmd
	state       string // stopped | starting | running | error
	wantRunning bool
	exePath     string
	logPath     string
	watcher     *logWatcher
	restarts    []time.Time
	verCache    map[string]string
}

func NewManager(cfg *config.Config, log *logger.Logger) *Manager {
	return &Manager{cfg: cfg, log: log, state: "stopped"}
}

func (m *Manager) exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func (m *Manager) componentPath(sub string) string {
	return filepath.Join(m.exeDir(), sub)
}

// Version возвращает версию компонента. Источник — VERSIONINFO самого exe
// (вшивается сборкой из __version__ апстрима), затем components/tgproxy/version.txt,
// в последнюю очередь — fallback-переменная (ldflags). Кэш по mtime+размеру.
func (m *Manager) Version() string {
	exe := m.componentPath(exeSubPath)
	st, err := os.Stat(exe)
	if err != nil {
		return Version
	}
	key := fmt.Sprintf("%d:%d", st.ModTime().UnixNano(), st.Size())
	m.mu.Lock()
	if v, ok := m.verCache[key]; ok {
		m.mu.Unlock()
		return v
	}
	m.mu.Unlock()

	v := exeProductVersion(exe)
	if v == "" {
		if data, err := os.ReadFile(m.componentPath(verSubPath)); err == nil {
			if s := strings.TrimSpace(string(data)); s != "" {
				v = s
			}
		}
	}
	if v == "" {
		v = Version
	}
	m.mu.Lock()
	m.verCache = map[string]string{key: v}
	m.mu.Unlock()
	return v
}

// exeProductVersion читает ProductVersion из VERSIONINFO exe (Win32 API).
func exeProductVersion(path string) string {
	var zero windows.Handle
	size, err := windows.GetFileVersionInfoSize(path, &zero)
	if err != nil || size == 0 {
		return ""
	}
	buf := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buf[0])); err != nil {
		return ""
	}
	var trans *uint16
	var transLen uint32
	if err := windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\VarFileInfo\Translation`, unsafe.Pointer(&trans), &transLen); err != nil || trans == nil || transLen < 4 {
		return ""
	}
	langCode := unsafe.Slice(trans, transLen/2)
	sub := fmt.Sprintf(`\StringFileInfo\%04x%04x\ProductVersion`, langCode[0], langCode[1])
	var str *uint16
	var strLen uint32
	if err := windows.VerQueryValue(unsafe.Pointer(&buf[0]), sub, unsafe.Pointer(&str), &strLen); err != nil || str == nil || strLen == 0 {
		return ""
	}
	return windows.UTF16PtrToString(str)
}

// ComponentInstalled сообщает, лежит ли exe компонента рядом с приложением.
func (m *Manager) ComponentInstalled() bool {
	_, err := os.Stat(m.componentPath(exeSubPath))
	return err == nil
}

func (m *Manager) Start() error {
	m.mu.Lock()
	if m.wantRunning {
		m.mu.Unlock()
		return nil
	}
	pc := m.cfg.GetTgProxyConfig()
	secretBytes, err := hex.DecodeString(pc.Secret)
	if err != nil || len(secretBytes) == 0 {
		m.mu.Unlock()
		return fmt.Errorf("неверный секрет Telegram-моста")
	}
	exePath := m.componentPath(exeSubPath)
	if _, err := os.Stat(exePath); err != nil {
		m.mu.Unlock()
		m.log.Error("tgproxy", "component not installed: "+exePath)
		return fmt.Errorf("компонент Telegram-моста не установлен")
	}
	logPath := filepath.Join(m.cfg.LogsDir(), tgProxyLogName)
	// Старый лог (прошлого запуска) сбрасываем: счётчики и per-IP считаются
	// только из лога текущего процесса.
	_ = os.Remove(logPath)
	// Осиротевший мост от рухнувшего прошлого экземпляра приложения держит
	// порт моста — каждый новый спавн падает с exit 1 до «6 рестартов,
	// сдаюсь». Свой живой мост сюда не доходит (гард wantRunning выше),
	// второй экземпляр приложения невозможен (named mutex).
	executil.HiddenCmd("taskkill", "/IM", filepath.Base(exeSubPath), "/F").Run()
	_ = os.Remove(logPath + ".1")
	m.exePath = exePath
	m.logPath = logPath
	m.wantRunning = true
	m.restarts = nil
	cmd, err := m.spawnLocked(pc, logPath)
	if err != nil {
		m.wantRunning = false
		m.state = "error"
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()

	m.log.Info("tgproxy", "starting component v"+m.Version()+" on "+net.JoinHostPort(pc.BindHost, strconv.Itoa(pc.Port)))
	go m.supervise(cmd, pc, logPath)
	return nil
}

// spawnLocked запускает процесс; вызывает под m.mu.
func (m *Manager) spawnLocked(pc config.TelegramProxyConfig, logPath string) (*exec.Cmd, error) {
	cmd := executil.HiddenCmd(m.exePath, buildArgs(pc, logPath)...)
	if err := cmd.Start(); err != nil {
		m.log.Error("tgproxy", "spawn: "+err.Error())
		return nil, fmt.Errorf("не удалось запустить компонент моста: %w", err)
	}
	m.cmd = cmd
	m.state = "starting"
	return cmd, nil
}

// supervise — жизненный цикл одного/перезапущенного процесса: ready-гейт,
// вотчер лога, рестарты при падении (не более maxRestarts за restartWindow).
// cmd.Wait() вызывается ровно один раз — в фоновой горутине; её канал
// позволяет waitReady мгновенно увидеть ранний вылет процесса.
func (m *Manager) supervise(first *exec.Cmd, pc config.TelegramProxyConfig, logPath string) {
	cmd := first
	for {
		waitCh := make(chan error, 1)
		go func(c *exec.Cmd) { waitCh <- c.Wait() }(cmd)
		if !m.waitReady(pc, cmd, waitCh) {
			return
		}
		m.mu.Lock()
		m.state = "running"
		w := startLogWatcher(logPath, m.log)
		m.watcher = w
		m.mu.Unlock()
		m.log.Info("tgproxy", "bridge is running")

		err := <-waitCh

		m.mu.Lock()
		if w != nil {
			w.stop()
			m.watcher = nil
		}
		m.cmd = nil
		if !m.wantRunning {
			m.state = "stopped"
			m.mu.Unlock()
			m.log.Info("tgproxy", "bridge stopped")
			return
		}
		now := time.Now()
		m.restarts = append(m.restarts, now)
		window := now.Add(-restartWindow)
		recent := m.restarts[:0]
		for _, t := range m.restarts {
			if t.After(window) {
				recent = append(recent, t)
			}
		}
		m.restarts = recent
		if len(m.restarts) > maxRestarts {
			m.state = "error"
			m.mu.Unlock()
			m.log.Error("tgproxy", fmt.Sprintf("component keeps crashing (%d restarts in %s), giving up: %v", len(m.restarts), restartWindow, err))
			return
		}
		m.state = "starting"
		m.mu.Unlock()
		m.log.Warn("tgproxy", fmt.Sprintf("component exited (%v), restarting (%d/%d)", err, len(m.restarts), maxRestarts))
		time.Sleep(time.Second)
		m.mu.Lock()
		cmd, err = m.spawnLocked(m.cfg.GetTgProxyConfig(), logPath)
		if err != nil {
			m.state = "error"
			m.mu.Unlock()
			return
		}
		m.mu.Unlock()
	}
}

// waitReady ждёт открытия порта моста (до readyTimeout); при выходе процесса
// (признаётся по закрытию waitCh — ProcessState заполняется только Wait) или
// таймауте помечает состояние и возвращает false.
func (m *Manager) waitReady(pc config.TelegramProxyConfig, cmd *exec.Cmd, waitCh <-chan error) bool {
	port := strconv.Itoa(pc.Port)
	probeHost := pc.BindHost
	if probeHost == "" || probeHost == "0.0.0.0" || probeHost == "::" {
		probeHost = "127.0.0.1"
	}
	deadline := time.Now().Add(readyTimeout)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		want := m.wantRunning
		m.mu.Unlock()
		if !want {
			return false // Stop() во время прогрева
		}
		select {
		case err := <-waitCh:
			m.finishStopped(fmt.Sprintf("component exited before ready (%v)", err))
			return false
		default:
		}
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(probeHost, port), 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	m.finishStopped("component did not open port in " + readyTimeout.String())
	executil.HiddenCmd("taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	return false
}

// finishStopped помечает ошибку запуска; если процесс уже гасили через Stop —
// оставляем честный «stopped».
func (m *Manager) finishStopped(msg string) {
	m.mu.Lock()
	m.wantRunning = false
	if m.state == "stopped" {
		m.mu.Unlock()
		return
	}
	m.state = "error"
	m.mu.Unlock()
	m.log.Error("tgproxy", msg)
}

func (m *Manager) Stop() {
	m.mu.Lock()
	m.wantRunning = false
	cmd := m.cmd
	if w := m.watcher; w != nil {
		w.stop()
		m.watcher = nil
	}
	m.state = "stopped"
	m.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		executil.HiddenCmd("taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
		m.log.Info("tgproxy", "bridge stopped")
	}
}

func (m *Manager) Restart() error {
	m.Stop()
	time.Sleep(300 * time.Millisecond)
	return m.Start()
}

func (m *Manager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state == "running"
}

func (m *Manager) GetStatus() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// ClientsByIP возвращает активные MTProto-сессии по IP клиентов
// (восстановлено из лога процесса: строки открытия/закрытия сессий).
func (m *Manager) ClientsByIP() map[string]int {
	m.mu.Lock()
	w := m.watcher
	m.mu.Unlock()
	if w == nil {
		return map[string]int{}
	}
	return w.clientsByIP()
}

func (m *Manager) GetStats() map[string]interface{} {
	m.mu.Lock()
	w := m.watcher
	state := m.state
	m.mu.Unlock()
	stats := map[string]interface{}{
		"active":       int64(0),
		"total":        int64(0),
		"bytes_in":     int64(0),
		"bytes_out":    int64(0),
		"ws_ok":        int64(0),
		"ws_fail":      int64(0),
		"ws_errors":    int64(0),
		"cfproxy":      int64(0),
		"tcp_fallback": int64(0),
		"fronting":     int64(0),
		"masked":       int64(0),
		"pool_hits":    int64(0),
		"pool_misses":  int64(0),
		"h2":           int64(0),
		"h2_try":       int64(0),
	}
	if w != nil {
		for k, v := range w.countersSnapshot() {
			stats[k] = v
		}
		// Активные сессии — живьём из трекинга соединений, а не из
		// минутной stats-строки.
		stats["active"] = w.activeCount()
	}
	stats["running"] = state == "running"
	return stats
}

// GetLink возвращает основную ссылку tg://proxy (ee при включённом Fake TLS,
// иначе dd).
func (m *Manager) GetLink() string {
	pc := m.cfg.GetTgProxyConfig()
	host := linkHost(pc.BindHost)
	if pc.FakeTLSDomain != "" {
		if ee := eeLink(host, pc.Port, pc.Secret, pc.FakeTLSDomain); ee != "" {
			return ee
		}
	}
	return fmt.Sprintf("tg://proxy?server=%s&port=%d&secret=dd%s", host, pc.Port, pc.Secret)
}

// GetLinks возвращает обе ссылки: dd и ee (ee пуст, если Fake TLS выключен).
func (m *Manager) GetLinks() (dd, ee string) {
	pc := m.cfg.GetTgProxyConfig()
	host := linkHost(pc.BindHost)
	dd = fmt.Sprintf("tg://proxy?server=%s&port=%d&secret=dd%s", host, pc.Port, pc.Secret)
	ee = eeLink(host, pc.Port, pc.Secret, pc.FakeTLSDomain)
	return dd, ee
}

// RegenerateSecret генерирует новый secret и (если нужно) перезапускает мост.
func (m *Manager) RegenerateSecret() (string, error) {
	wasRunning := m.IsRunning()
	if wasRunning {
		m.Stop()
	}
	pc := m.cfg.GetTgProxyConfig()
	pc.Secret = "" // триггер автогенерации при следующем Get
	if err := m.cfg.SetTgProxyConfig(pc); err != nil {
		return "", err
	}
	newSecret := m.cfg.GetTgProxyConfig().Secret
	if wasRunning {
		if err := m.Start(); err != nil {
			m.log.Warn("tgproxy", "restart after secret regen failed: "+err.Error())
		}
	}
	return newSecret, nil
}

// eeLink строит ee-secret ссылку для Fake TLS: secret=ee<secret><domain_hex>.
func eeLink(host string, port int, secret, fakeTLSDomain string) string {
	if fakeTLSDomain == "" {
		return ""
	}
	domainHex := fmt.Sprintf("%x", fakeTLSDomain)
	return fmt.Sprintf("tg://proxy?server=%s&port=%d&secret=ee%s%s", host, port, secret, domainHex)
}

// linkHost: если 0.0.0.0 — определяет LAN IP, иначе возвращает как есть.
func linkHost(bind string) string {
	if bind == "0.0.0.0" || bind == "" {
		if ip := detectLANIP(); ip != "" {
			return ip
		}
		return "127.0.0.1"
	}
	return bind
}

func detectLANIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return addr.IP.String()
	}
	return ""
}
