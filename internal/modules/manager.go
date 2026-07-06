package modules

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"zpui/internal/logger"
)

const CREATE_NO_WINDOW = 0x08000000

type State string

const (
	StateStopped  State = "stopped"
	StateRunning  State = "running"
	StateError    State = "error"
)

type procInfo struct {
	cmd    *exec.Cmd
	state  State
	exitAt time.Time
	err    string
}

type Manager struct {
	mu       sync.Mutex
	log      *logger.Logger
	rootDir  string
	disabled func(string) bool
	procs    map[string]*procInfo
}

func NewManager(rootDir string, log *logger.Logger, isDisabled func(string) bool) *Manager {
	return &Manager{
		log:      log,
		rootDir:  rootDir,
		disabled: isDisabled,
		procs:    make(map[string]*procInfo),
	}
}

func (m *Manager) RootDir() string { return m.rootDir }

func (m *Manager) Discover() []*DiscoveredModule {
	return Discover(m.rootDir)
}

func (m *Manager) Start(manifest *Manifest) error {
	m.mu.Lock()
	if p, ok := m.procs[manifest.ID]; ok && p.state == StateRunning {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	if !manifest.HasEntry() {
		return fmt.Errorf("module %q: entry %q not found", manifest.ID, manifest.Entry)
	}

	cmd := exec.Command(manifest.EntryPath(), manifest.Args...)
	cmd.Dir = manifest.Dir()
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: CREATE_NO_WINDOW,
	}

	if err := cmd.Start(); err != nil {
		m.mu.Lock()
		m.procs[manifest.ID] = &procInfo{state: StateError, err: err.Error()}
		m.mu.Unlock()
		m.log.Error("modules", fmt.Sprintf("start %q failed: %v", manifest.ID, err))
		return err
	}

	m.mu.Lock()
	m.procs[manifest.ID] = &procInfo{cmd: cmd, state: StateRunning}
	m.mu.Unlock()

	m.log.Info("modules", fmt.Sprintf("started %q (pid %d)", manifest.ID, cmd.Process.Pid))

	id := manifest.ID
	go func() {
		err := cmd.Wait()
		m.mu.Lock()
		p := m.procs[id]
		if p != nil {
			p.state = StateStopped
			p.exitAt = time.Now()
			if err != nil {
				p.err = err.Error()
			}
		}
		m.mu.Unlock()
		m.log.Info("modules", fmt.Sprintf("stopped %q: %v", id, err))
	}()

	return nil
}

func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	p, ok := m.procs[id]
	m.mu.Unlock()
	if !ok || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	m.log.Info("modules", fmt.Sprintf("stopping %q", id))
	return p.cmd.Process.Kill()
}

func (m *Manager) Restart(manifest *Manifest) error {
	_ = m.Stop(manifest.ID)
	time.Sleep(300 * time.Millisecond)
	return m.Start(manifest)
}

func (m *Manager) StateOf(id string) State {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.procs[id]; ok {
		return p.state
	}
	return StateStopped
}

func (m *Manager) AutoStartAll(mods []*DiscoveredModule) {
	for _, dm := range mods {
		if m.disabled != nil && m.disabled(dm.Manifest.ID) {
			continue
		}
		if !dm.Manifest.AutoStart || !dm.EntryOK {
			continue
		}
		if err := m.Start(dm.Manifest); err != nil {
			m.log.Warn("modules", fmt.Sprintf("autostart %q failed: %v", dm.Manifest.ID, err))
		}
	}
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.procs))
	for id := range m.procs {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		_ = m.Stop(id)
	}
}

func (m *Manager) Status(mod *DiscoveredModule) map[string]interface{} {
	state := m.StateOf(mod.Manifest.ID)
	status := map[string]interface{}{
		"id":        mod.Manifest.ID,
		"name":      mod.Manifest.Name,
		"version":   mod.Manifest.Version,
		"author":    mod.Manifest.Author,
		"description": mod.Manifest.Description,
		"state":     string(state),
		"entry_ok":  mod.EntryOK,
		"autostart": mod.Manifest.AutoStart,
	}
	if extra := ReadStatusFile(mod.Dir); extra != nil {
		for k, v := range extra {
			if _, exists := status[k]; !exists {
				status[k] = v
			}
		}
	}
	return status
}

func DefaultRootDir(exeDir string) string {
	return filepath.Join(exeDir, "modules")
}

func ResolveExeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}
