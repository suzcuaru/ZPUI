package zsvc

import (
	"errors"
	"fmt"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceDisplayName = "ZPUI Service"
	serviceDescription = "Управление запуском и восстановлением ZPUI"

	stopPollInterval = 500 * time.Millisecond
	stopTimeout      = 10 * time.Second
)

// Install регистрирует службу (или обновляет конфиг существующей):
// автоматический запуск БЕЗ отложенного старта, SCM-recovery — рестарт
// через 60с, сброс счётчика раз в сутки.
// Установку выполняет pkginst (или приложение в части 2), не сам exe службы.
func Install(binPath string) error {
	if binPath == "" {
		return errors.New("пустой путь к exe службы")
	}
	abs, err := filepath.Abs(binPath)
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("scm connect: %w", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(ServiceName)
	if err == nil {
		defer s.Close()
		cfg, err := s.Config()
		if err != nil {
			return err
		}
		cfg.BinaryPathName = abs
		cfg.DisplayName = serviceDisplayName
		cfg.Description = serviceDescription
		cfg.StartType = mgr.StartAutomatic
		cfg.DelayedAutoStart = false
		if err := s.UpdateConfig(cfg); err != nil {
			return err
		}
	} else {
		s, err = m.CreateService(ServiceName, abs, mgr.Config{
			StartType:        mgr.StartAutomatic,
			DisplayName:      serviceDisplayName,
			Description:      serviceDescription,
			DelayedAutoStart: false,
		})
		if err != nil {
			return fmt.Errorf("create service: %w", err)
		}
		defer s.Close()
	}
	// Сбой службы не должен оставлять систему без надзора: SCM перезапустит её.
	if err := s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
	}, 86400); err != nil {
		return fmt.Errorf("recovery actions: %w", err)
	}
	return nil
}

// Start запускает установленную службу. «Уже запущена» — не ошибка: старт
// конкурирует с SCM-recovery и параллельным repair, и 1056 в этом случае
// штатный исход.
func Start() error {
	s, done, err := openService()
	if err != nil {
		return err
	}
	defer done()
	if st, qerr := s.Query(); qerr == nil && st.State != svc.Stopped {
		return nil
	}
	err = s.Start()
	if err == nil {
		return nil
	}
	var errno syscall.Errno
	if errors.As(err, &errno) && errno == windows.ERROR_SERVICE_ALREADY_RUNNING {
		return nil
	}
	return err
}

// Stop останавливает службу (graceful, ожидание до 10с). Не установлена — nil.
// Служба в START_PENDING stop не принимает (1061): ждём входа в RUNNING,
// затем останавливаем.
func Stop() error {
	s, done, err := openService()
	if err != nil {
		return err
	}
	defer done()
	st, err := s.Query()
	if err != nil {
		return err
	}
	if st.State == svc.Stopped {
		return nil
	}
	if _, err := s.Control(svc.Stop); err != nil {
		var errno syscall.Errno
		if !errors.As(err, &errno) || errno != windows.ERROR_SERVICE_CANNOT_ACCEPT_CTRL {
			return err
		}
		if st, qerr := s.Query(); qerr == nil && st.State == svc.StartPending {
			if err := waitServiceState(s, svc.Running, stopTimeout); err != nil {
				return err
			}
			if _, err := s.Control(svc.Stop); err != nil {
				return err
			}
		}
	}
	return waitServiceState(s, svc.Stopped, stopTimeout)
}

func waitServiceState(s *mgr.Service, want svc.State, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		st, err := s.Query()
		if err != nil {
			return err
		}
		if st.State == want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("служба не достигла состояния %d за %s (текущее: %d)", want, timeout, st.State)
		}
		time.Sleep(stopPollInterval)
	}
}

// Remove останавливает и удаляет службу. Не установлена — nil (идемпотентно).
func Remove() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("scm connect: %w", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(ServiceName)
	if err != nil {
		return nil
	}
	defer s.Close()
	if st, err := s.Query(); err == nil && st.State != svc.Stopped {
		_, _ = s.Control(svc.Stop)
		deadline := time.Now().Add(stopTimeout)
		for {
			st, err := s.Query()
			if err != nil || st.State == svc.Stopped {
				break
			}
			if time.Now().After(deadline) {
				break
			}
			time.Sleep(stopPollInterval)
		}
	}
	return s.Delete()
}

// IsInstalled — зарегистрирована ли служба.
func IsInstalled() (bool, error) {
	m, err := mgr.Connect()
	if err != nil {
		return false, err
	}
	defer m.Disconnect()
	s, err := m.OpenService(ServiceName)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return false, nil
		}
		return false, err
	}
	s.Close()
	return true, nil
}

// BinaryPath — путь к exe службы из конфигурации SCM.
func BinaryPath() (string, error) {
	s, done, err := openService()
	if err != nil {
		return "", err
	}
	defer done()
	cfg, err := s.Config()
	if err != nil {
		return "", err
	}
	return cfg.BinaryPathName, nil
}

func openService() (*mgr.Service, func(), error) {
	m, err := mgr.Connect()
	if err != nil {
		return nil, nil, fmt.Errorf("scm connect: %w", err)
	}
	s, err := m.OpenService(ServiceName)
	if err != nil {
		m.Disconnect()
		return nil, nil, fmt.Errorf("open service: %w", err)
	}
	return s, func() { s.Close(); m.Disconnect() }, nil
}
