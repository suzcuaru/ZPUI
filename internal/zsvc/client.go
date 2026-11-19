package zsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/windows"
)

// Client — pipe-клиент для приложения (часть 2): best-effort вызовы к службе,
// каждый с таймаутом (по умолчанию 2с, сокращается дедлайном ctx).
// Ошибка — обычный error; вызывающий сам решает фолбэк (служба может быть
// не установлена/не запущена — это нормальная ситуация).
type Client struct {
	// Timeout — максимальная длительность одного вызова.
	Timeout time.Duration
}

func NewClient() *Client { return &Client{Timeout: 2 * time.Second} }

func (c *Client) timeout() time.Duration {
	if c == nil || c.Timeout <= 0 {
		return 2 * time.Second
	}
	return c.Timeout
}

func (c *Client) call(ctx context.Context, line string) (string, error) {
	timeout := c.timeout()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if dl, ok := ctx.Deadline(); ok {
		if d := time.Until(dl); d > 0 && d < timeout {
			timeout = d
		}
	}
	h, err := dialPipe(timeout)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	if err := writeAllTO(h, append([]byte(line), '\n'), timeout); err != nil {
		return "", err
	}
	return readLineTO(h, timeout)
}

func dialPipe(timeout time.Duration) (windows.Handle, error) {
	p16, err := windows.UTF16PtrFromString(PipeName)
	if err != nil {
		return 0, err
	}
	create := func() (windows.Handle, error) {
		return windows.CreateFile(p16,
			windows.GENERIC_READ|windows.GENERIC_WRITE,
			0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OVERLAPPED, 0)
	}
	h, err := create()
	if err == nil {
		return h, nil
	}
	if err != windows.ERROR_PIPE_BUSY {
		return 0, err
	}
	// Занятый инстанс: повторяем до дедлайна (клиентов мало — приложение).
	deadline := time.Now().Add(timeout)
	for {
		time.Sleep(25 * time.Millisecond)
		if h, err = create(); err == nil {
			return h, nil
		}
		if err != windows.ERROR_PIPE_BUSY {
			return 0, err
		}
		if time.Now().After(deadline) {
			return 0, windows.ERROR_PIPE_BUSY
		}
	}
}

// Call — «сырой» вызов: строка команды, строка ответа.
func (c *Client) Call(ctx context.Context, line string) (string, error) {
	return c.call(ctx, line)
}

// decodeResp проверяет {"ok":bool,"error":...}; !ok или битый JSON — ошибка.
func decodeResp(resp string) error {
	var m struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(resp), &m); err != nil {
		return fmt.Errorf("bad service response: %w", err)
	}
	if !m.OK {
		if m.Error == "" {
			m.Error = "failed"
		}
		return errors.New(m.Error)
	}
	return nil
}

func (c *Client) simple(ctx context.Context, line string) error {
	resp, err := c.call(ctx, line)
	if err != nil {
		return err
	}
	return decodeResp(resp)
}

// Ping — проверка живости службы; возвращает версию exe службы.
func (c *Client) Ping(ctx context.Context) (version string, err error) {
	resp, err := c.call(ctx, "ping")
	if err != nil {
		return "", err
	}
	var m struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(resp), &m); err != nil {
		return "", err
	}
	if !m.OK {
		return "", errors.New("ping failed")
	}
	return m.Version, nil
}

// Status — полный JSON статуса службы (как в ответе IPC status).
func (c *Client) Status(ctx context.Context) (string, error) {
	return c.call(ctx, "status")
}

// Hello — приложение сообщило о запуске: pid, версия, флаги свёрнутого
// запуска и пользовательского автозапуска (гард краш-восстановления).
func (c *Client) Hello(ctx context.Context, pid int, version string, hidden, autostart bool) error {
	b, err := json.Marshal(helloArg{Pid: pid, Version: version, Hidden: hidden, Autostart: autostart})
	if err != nil {
		return err
	}
	return c.simple(ctx, "app-hello "+string(b))
}

// Exiting — приложение завершается штатно.
func (c *Client) Exiting(ctx context.Context) error {
	return c.simple(ctx, "app-exiting")
}

// Restarting — приложение завершается для перезапуска.
func (c *Client) Restarting(ctx context.Context) error {
	return c.simple(ctx, "app-restarting")
}

// Updating — приложение обновляется, не считать крашем ttl секунд.
func (c *Client) Updating(ctx context.Context, ttlSec int) error {
	b, err := json.Marshal(updatingArg{TtlSec: ttlSec})
	if err != nil {
		return err
	}
	return c.simple(ctx, "app-updating "+string(b))
}

// RestartApp — служба перезапускает приложение (скрыто, если hidden).
func (c *Client) RestartApp(ctx context.Context, hidden bool) error {
	b, err := json.Marshal(restartArg{Hidden: hidden})
	if err != nil {
		return err
	}
	return c.simple(ctx, "restart-app "+string(b))
}

// CloseApp — служба штатно закрывает приложение.
func (c *Client) CloseApp(ctx context.Context) error {
	return c.simple(ctx, "close-app")
}
