package zsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

var errBadArg = errors.New("bad argument")

// IPC-протокол: одна строка = одна команда, ответ — одна строка JSON.
// Аргументы вида "команда {json}" или "команда 1|0".

type ipcCommand struct {
	Name string
	Arg  string
}

// Аргументы JSON-команд (общие для сервера-парсера и клиента-сборщика).
type helloArg struct {
	Pid       int    `json:"pid"`
	Version   string `json:"version"`
	Hidden    bool   `json:"hidden"`
	Autostart bool   `json:"autostart"`
}

type updatingArg struct {
	TtlSec int `json:"ttl_sec"`
}

type restartArg struct {
	Hidden bool `json:"hidden"`
}

func parseIpcCommand(line string) ipcCommand {
	line = strings.TrimSpace(line)
	if i := strings.IndexByte(line, ' '); i >= 0 {
		return ipcCommand{Name: strings.ToLower(line[:i]), Arg: strings.TrimSpace(line[i+1:])}
	}
	return ipcCommand{Name: strings.ToLower(line)}
}

// parseHelloArg разбирает аргумент app-hello: pid, версия, флаги свёрнутого
// запуска и автозапуска (последний — гард краш-восстановления watchdog'а:
// службу пользовательский HKCU не виден).
func parseHelloArg(arg string) (pid uint32, version string, hidden, autostart bool, err error) {
	var a helloArg
	if err = json.Unmarshal([]byte(arg), &a); err != nil {
		return 0, "", false, false, err
	}
	return uint32(a.Pid), a.Version, a.Hidden, a.Autostart, nil
}

func parseUpdatingArg(arg string) (time.Duration, error) {
	var a updatingArg
	if err := json.Unmarshal([]byte(arg), &a); err != nil {
		return 0, err
	}
	if a.TtlSec < 0 {
		return 0, errBadArg
	}
	return time.Duration(a.TtlSec) * time.Second, nil
}

func parseRestartArg(arg string) (bool, error) {
	var a restartArg
	if strings.TrimSpace(arg) == "" {
		return false, nil
	}
	if err := json.Unmarshal([]byte(arg), &a); err != nil {
		return false, err
	}
	return a.Hidden, nil
}

func respOK() string { return "{\"ok\":true}\n" }

func respOKVersion(version string) string {
	return "{\"ok\":true,\"version\":" + jsonString(version) + "}\n"
}

func respErr(code string) string {
	return "{\"ok\":false,\"error\":" + jsonString(code) + "}\n"
}

func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

// pipeServer — акцептор pipe-соединений: по горутине на подключение.
type pipeServer struct {
	handler func(cmd ipcCommand) string
	log     *SvcLog
}

func (ps *pipeServer) serve(ctx context.Context) {
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		h, err := createPipeInstance()
		if err != nil {
			ps.log.Error("create pipe instance: " + err.Error())
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		connected, err := waitClient(ctx.Done(), h)
		if err != nil {
			_ = windows.CloseHandle(h)
			if ctx.Err() == nil {
				ps.log.Error("pipe accept: " + err.Error())
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
				}
			}
			continue
		}
		if !connected {
			_ = windows.CloseHandle(h)
			continue
		}
		go ps.serveConn(h)
	}
}

func (ps *pipeServer) serveConn(h windows.Handle) {
	defer windows.CloseHandle(h)
	defer windows.DisconnectNamedPipe(h)
	line, err := readLineTO(h, pipeCmdTimeout)
	if err != nil {
		ps.log.Error("pipe read: " + err.Error())
		return
	}
	if strings.TrimSpace(line) == "" {
		return
	}
	if !allowedCaller(h, ps.log) {
		ps.log.Warn("pipe: команда от чужого процесса отклонена: " + line)
		_ = writeAllTO(h, []byte(respErr("forbidden")), pipeCmdTimeout)
		return
	}
	resp := ps.handler(parseIpcCommand(line))
	if err := writeAllTO(h, []byte(resp), pipeCmdTimeout); err != nil {
		ps.log.Error("pipe write: " + err.Error())
	}
}

// allowedCaller — вызывающий должен быть из папки установки ZPUI.
// Сбой запроса пути не блокирует команду: служба не должна терять контроль
// над приложением из-за разового сбоя OpenProcess.
func allowedCaller(h windows.Handle, log *SvcLog) bool {
	var pid uint32
	if err := windows.GetNamedPipeClientProcessId(h, &pid); err != nil {
		log.Debug("pipe: client pid query failed: " + err.Error())
		return true
	}
	if pid == 0 {
		return true
	}
	ph, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		log.Debug(fmt.Sprintf("pipe: open process %d failed: %v", pid, err))
		return true
	}
	defer windows.CloseHandle(ph)
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(ph, 0, &buf[0], &n); err != nil {
		log.Debug(fmt.Sprintf("pipe: image path query %d failed: %v", pid, err))
		return true
	}
	dir := InstallDir()
	if dir == "" {
		return true
	}
	img := strings.ToLower(windows.UTF16ToString(buf[:n]))
	dir = strings.ToLower(filepath.Clean(dir))
	return img == dir || strings.HasPrefix(img, dir+string(os.PathSeparator))
}
