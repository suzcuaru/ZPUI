package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"

	"zpui/internal/executil"
)

// Замена PowerShell-помощников «Wait-Process; Start-Process»: батник с
// tasklist-опросом. cmd+tasklist+ping на порядки легче powershell.exe
// (сотни МБ working set за вызов) и не тянет .NET-рантайм.

var restartSeq atomic.Int64

// spawnWaitAndStart запускает отсоединённого помощника: тот ждёт выхода
// текущего процесса (по PID) и стартует exePath с переданными аргументами
// (--hidden, чтобы перезапуск не разворачивал свернутое в трей приложение).
// Используется для Restart и запуска установщика после выхода — новый
// процесс должен дождаться освобождения single-instance мьютекса.
//
// Смерть процесса подтверждается ДВУКРАТНО с секундным интервалом: одна
// пустая выборка tasklist на проснувшейся машине — флак, а ложный «умер»
// даёт гонку с ещё живым процессом: новый экземпляр молча умирает на
// single-instance мьютексе, и приложение оказывается не запущено вовсе
// (наблюдалось утром после сна: два старта с разницей в 6 секунд).
func spawnWaitAndStart(exePath string, args ...string) error {
	pid := os.Getpid()
	img := filepath.Base(exePath)
	bat := filepath.Join(os.TempDir(), fmt.Sprintf("zpui-start-%d-%d.cmd", pid, restartSeq.Add(1)))
	argTail := ""
	for _, a := range args {
		argTail += " " + a
	}
	script := fmt.Sprintf(`@echo off
:w
tasklist /FI "PID eq %d" 2>nul | find "%d" >nul 2>&1
if errorlevel 1 goto gone
ping -n 2 127.0.0.1 >nul
goto w
:gone
ping -n 2 127.0.0.1 >nul
tasklist /FI "PID eq %d" 2>nul | find "%d" >nul 2>&1
if errorlevel 1 goto run
goto w
:run
start "" "%s"%s
ping -n 4 127.0.0.1 >nul
tasklist /FI "IMAGENAME eq %s" 2>nul | find /i "%s" >nul 2>&1
if errorlevel 1 start "" "%s"%s
del "%%~f0"
`, pid, pid, pid, pid, exePath, argTail, img, img, exePath, argTail)
	if err := os.WriteFile(bat, []byte(script), 0o644); err != nil {
		return err
	}
	if err := executil.DetachedCmd("cmd", "/C", bat).Start(); err != nil {
		// Запуск из задачи Планировщика: ZPUI живёт внутри job-объекта задания,
		// где BREAKAWAY запрещён, и попытка оторваться даёт «Access is denied»
		// (после автозапуска не работали ни перезапуск, ни установка обновлений).
		// Повторяем без отрыва: помощнику необязательно покидать задание,
		// важно, чтобы он вообще стартовал.
		if !errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
			return err
		}
		return executil.HiddenCmd("cmd", "/C", bat).Start()
	}
	return nil
}
