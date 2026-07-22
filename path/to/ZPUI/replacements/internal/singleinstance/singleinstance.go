// Package singleinstance обеспечивает запуск приложения в единственном экземпляре.
// Использует Windows-мьютекс для детекта второго экземпляра.
package singleinstance

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
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
	otherPID := findOtherZPUI()
	if otherPID != 0 {
		title, _ := windows.UTF16PtrFromString("ZPUI — уже запущена")
		msg, _ := windows.UTF16PtrFromString(
			fmt.Sprintf("Программа уже запущена (PID: %d).\n\nЗакрыть другое окно и открыть это?", otherPID),
		)
		btn, _ := windows.MessageBox(windows.HWND(0), msg, title, windows.MB_YESNO|windows.MB_ICONWARNING|windows.MB_TOPMOST)
		const idYes = 6
		if btn == idYes {
			// Пытаемся закрыть другое окно
			killCmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(otherPID))
			killCmd.Run()
			// После убийства можно запускаться — создаём мьютекс заново
			name2, _ := windows.UTF16PtrFromString(mutexName)
			h2, err2 := windows.CreateMutex(nil, false, name2)
			if err2 == nil {
				return func() { windows.CloseHandle(h2) }, nil
			}
			if h2 != 0 {
				windows.CloseHandle(h2)
			}
			// Если не получилось — показываем ошибку
			title2, _ := windows.UTF16PtrFromString("Ошибка")
			msg2, _ := windows.UTF16PtrFromString("Не удалось закрыть другое окно программы.\nПопробуйте закрыть его вручную в Диспетчере задач.")
			windows.MessageBox(windows.HWND(0), msg2, title2, windows.MB_OK|windows.MB_ICONERROR|windows.MB_TOPMOST)
		}
	}

	// Пользователь отказался закрывать — выходим
	return nil, fmt.Errorf("другое окно ZPUI уже запущено")
}

// findOtherZPUI ищет другой процесс zpui.exe (с PID, отличным от текущего).
func findOtherZPUI() int {
	myPID := os.Getpid()
	exeName := filepath.Base(os.Args[0])
	if strings.HasSuffix(strings.ToLower(exeName), ".exe") {
		exeName = exeName[:len(exeName)-4]
	}

	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("IMAGENAME eq %s.exe", exeName), "/FO", "CSV", "/NH")
	out, err := cmd.Output()
	if err != nil {
		return 0
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// CSV: "zpui.exe","1234","Console","1","1234 K"
		parts := strings.Split(line, "\",\"")
		if len(parts) < 2 {
			continue
		}
		pidStr := strings.Trim(parts[1], "\"")
		pid, err := strconv.Atoi(pidStr)
		if err != nil || pid == myPID {
			continue
		}
		return pid
	}
	return 0
}
