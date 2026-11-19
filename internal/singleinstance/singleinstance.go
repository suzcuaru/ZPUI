// Package singleinstance обеспечивает запуск приложения в единственном экземпляре.
// Использует Windows-мьютекс для детекта второго экземпляра.
package singleinstance

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"zpui/internal/executil"
)

const mutexName = "Local\\ZPUI-Instance-Mutex"

// Check проверяет, не запущено ли уже приложение.
// Если другой экземпляр найден, спрашивает пользователя через MessageBox — закрыть его или выйти.
// cleanup вызывается при завершении приложения для освобождения мьютекса.
// err != nil означает, что нужно завершить приложение (другой экземпляр остаётся).
func Check(exePath string) (cleanup func(), err error) {
	name, _ := windows.UTF16PtrFromString(mutexName)
	h, err := windows.CreateMutex(nil, false, name)
	if err == nil {
		// Мы — первый/единственный экземпляр
		return func() { windows.CloseHandle(h) }, nil
	}

	// Закрываем хендл, который CreateMutex мог вернуть даже при ERROR_ALREADY_EXISTS
	if h != 0 {
		windows.CloseHandle(h)
	}

	if err != windows.ERROR_ALREADY_EXISTS {
		return nil, fmt.Errorf("CreateMutex: %w", err)
	}

	// Другой экземпляр уже запущен
	otherPIDs := findOtherZPUI()
	if len(otherPIDs) > 0 {
		title, _ := windows.UTF16PtrFromString("ZPUI — уже запущено")
		msg, _ := windows.UTF16PtrFromString(
			fmt.Sprintf("ZPUI уже запущен (PID: %d).\n\nЗакрыть его и открыть это приложение?", otherPIDs[0]),
		)
		btn, _ := windows.MessageBox(windows.HWND(0), msg, title, windows.MB_YESNO|windows.MB_ICONWARNING|windows.MB_TOPMOST)
		const idYes = 6
		if btn == idYes {
			// /T — с деревом процессов: без него дети (мост Telegram, winws)
			// оставались жить и держали порт/драйвер после «закрытия».
			for _, pid := range otherPIDs {
				executil.HiddenCmd("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)).Run()
			}
			// Терминация для объектов системы асинхронна: мьютекс могло
			// держать ещё доживающее приложение — даём до ~2 секунд.
			for i := 0; i < 10; i++ {
				name2, _ := windows.UTF16PtrFromString(mutexName)
				h2, err2 := windows.CreateMutex(nil, false, name2)
				if err2 == nil {
					return func() { windows.CloseHandle(h2) }, nil
				}
				if h2 != 0 {
					windows.CloseHandle(h2)
				}
				time.Sleep(200 * time.Millisecond)
			}
			// Не получилось — просим закрыть вручную.
			title2, _ := windows.UTF16PtrFromString("Ошибка")
			msg2, _ := windows.UTF16PtrFromString("Не удалось закрыть другой экземпляр ZPUI.\nЗакройте его вручную в Диспетчере задач.")
			windows.MessageBox(windows.HWND(0), msg2, title2, windows.MB_OK|windows.MB_ICONERROR|windows.MB_TOPMOST)
		}
	}

	// Пользователь отказался закрывать — выходим
	return nil, fmt.Errorf("другое окно ZPUI уже запущено")
}

// findOtherZPUI ищет все другие процессы ZPUI.exe (PID, отличные от текущего).
func findOtherZPUI() []int {
	myPID := os.Getpid()
	exeName := filepath.Base(os.Args[0])
	if strings.HasSuffix(strings.ToLower(exeName), ".exe") {
		exeName = exeName[:len(exeName)-4]
	}

	cmd := executil.HiddenCmd("tasklist", "/FI", fmt.Sprintf("IMAGENAME eq %s.exe", exeName), "/FO", "CSV", "/NH")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var pids []int
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// CSV: "ZPUI.exe","1234","Console","1","1234 K"
		parts := strings.Split(line, "\",\"")
		if len(parts) < 2 {
			continue
		}
		pidStr := strings.Trim(parts[1], "\"")
		pid, err := strconv.Atoi(pidStr)
		if err != nil || pid == myPID {
			continue
		}
		pids = append(pids, pid)
	}
	return pids
}
