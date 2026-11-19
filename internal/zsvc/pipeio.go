package zsvc

import (
	"bytes"
	"errors"
	"io"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Минимальный own-листенер named pipe поверх x/sys/windows (встроенного нет):
// overlapped ConnectNamedPipe + таймауты на все операции ввода-вывода.

const (
	pipeBufferSize = 4096
	pipeCmdTimeout = 5 * time.Second
	pipeMaxCmdLen  = 64 * 1024
	pipePollMs     = 250

	// DACL pipe: команды принимаются только от SYSTEM и администраторов.
	// Приложение всегда работает elevated; всем остальным — отказ на открытии.
	pipeSDDL = "D:P(A;;GA;;;SY)(A;;GA;;;BA)"
)

func createPipeInstance() (windows.Handle, error) {
	var sa windows.SecurityAttributes
	saPtr := (*windows.SecurityAttributes)(nil)
	if sd, err := windows.SecurityDescriptorFromString(pipeSDDL); err == nil {
		sa.Length = uint32(unsafe.Sizeof(sa))
		sa.SecurityDescriptor = sd
		saPtr = &sa
	}
	return windows.CreateNamedPipe(
		windows.StringToUTF16Ptr(PipeName),
		windows.PIPE_ACCESS_DUPLEX|windows.FILE_FLAG_OVERLAPPED,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT,
		windows.PIPE_UNLIMITED_INSTANCES,
		pipeBufferSize, pipeBufferSize, 0, saPtr)
}

// waitClient ждёт подключения клиента к инстансу (overlapped, с отменой по stop).
func waitClient(stop <-chan struct{}, h windows.Handle) (bool, error) {
	ev, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(ev)
	var ol windows.Overlapped
	ol.HEvent = ev
	err = windows.ConnectNamedPipe(h, &ol)
	if err == nil {
		return true, nil
	}
	if err == windows.ERROR_PIPE_CONNECTED {
		return true, nil
	}
	if err != windows.ERROR_IO_PENDING {
		return false, err
	}
	for {
		select {
		case <-stop:
			_ = windows.CancelIoEx(h, &ol)
			return false, errors.New("pipe server stopped")
		default:
		}
		code, werr := windows.WaitForSingleObject(ev, pipePollMs)
		if werr != nil {
			return false, werr
		}
		if code != windows.WAIT_OBJECT_0 {
			continue
		}
		var n uint32
		if gerr := windows.GetOverlappedResult(h, &ol, &n, false); gerr != nil {
			if gerr == windows.ERROR_PIPE_CONNECTED {
				return true, nil
			}
			return false, gerr
		}
		return true, nil
	}
}

// pipeTransfer выполняет одну overlapped-операцию чтения/записи с дедлайном.
func pipeTransfer(h windows.Handle, buf []byte, write bool, deadline time.Time) (int, error) {
	ev, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(ev)
	var ol windows.Overlapped
	ol.HEvent = ev
	var n uint32
	if write {
		err = windows.WriteFile(h, buf, &n, &ol)
	} else {
		err = windows.ReadFile(h, buf, &n, &ol)
	}
	if err == windows.ERROR_IO_PENDING {
		for {
			remain := time.Until(deadline)
			if remain <= 0 {
				_ = windows.CancelIoEx(h, &ol)
				return int(n), errors.New("pipe i/o timeout")
			}
			ms := uint32(remain / time.Millisecond)
			if ms > pipePollMs {
				ms = pipePollMs
			}
			if ms < 1 {
				ms = 1
			}
			code, werr := windows.WaitForSingleObject(ev, ms)
			if werr != nil {
				return int(n), werr
			}
			if code != windows.WAIT_OBJECT_0 {
				continue
			}
			err = windows.GetOverlappedResult(h, &ol, &n, false)
			break
		}
	}
	if err == windows.ERROR_BROKEN_PIPE {
		err = io.EOF
	}
	return int(n), err
}

func pipeRead(h windows.Handle, buf []byte, deadline time.Time) (int, error) {
	return pipeTransfer(h, buf, false, deadline)
}

func pipeWrite(h windows.Handle, buf []byte, deadline time.Time) (int, error) {
	return pipeTransfer(h, buf, true, deadline)
}

// readLineTO читает строку команды (до '\n' или EOF) с таймаутом.
func readLineTO(h windows.Handle, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var line []byte
	buf := make([]byte, pipeBufferSize)
	for {
		n, err := pipeRead(h, buf, deadline)
		if n > 0 {
			line = append(line, buf[:n]...)
			if i := bytes.IndexByte(line, '\n'); i >= 0 {
				return string(line[:i]), nil
			}
			if len(line) > pipeMaxCmdLen {
				return "", errors.New("pipe: command too long")
			}
		}
		if err != nil {
			if err == io.EOF {
				return string(line), nil
			}
			return "", err
		}
		if n == 0 {
			return string(line), nil
		}
	}
}

// writeAllTO пишет весь буфер с таймаутом.
func writeAllTO(h windows.Handle, data []byte, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for len(data) > 0 {
		n, err := pipeWrite(h, data, deadline)
		if err != nil {
			return err
		}
		if n == 0 {
			return errors.New("pipe: write stalled")
		}
		data = data[n:]
	}
	return nil
}
