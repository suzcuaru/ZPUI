package app

import (
	"fmt"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"zpui/internal/database"
	"zpui/internal/power"
)

// Самоизмерение потребления: через минуту после старта (даём системе
// устояться) 30 секунд собираем CPU/RAM/потоки/хендлы/спавны процессов-
// помощников и пишем одну запись в app_metrics. Хранение 7 дней (чистка
// в startDataRotation), данные попадают в диагностический отчёт.

var (
	procGetProcessHandleCount = syscall.NewLazyDLL("kernel32.dll").NewProc("GetProcessHandleCount")
	procGetProcessMemoryInfo  = syscall.NewLazyDLL("psapi.dll").NewProc("GetProcessMemoryInfo")
)

// processMemoryCounters — PROCESS_MEMORY_COUNTERS. Поля-размеры — SIZE_T:
// uintptr (4 байта на 386, 8 на x64). uint32 здесь ломал раскладку на amd64 —
// GetProcessMemoryInfo отвечал ошибкой (cb меньше ожидаемого), поля
// оставались нулями, и в отчёте было «применение: 0 МБ».
type processMemoryCounters struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

// helperImages — процессы-помощники, спавн которых считаем признаком
// «тяжёлой» фоновой активности (нативные API их почти не оставляют).
var helperImages = map[string]bool{
	"powershell.exe": true, "cmd.exe": true, "sc.exe": true,
	"tasklist.exe": true, "taskkill.exe": true, "reg.exe": true,
	"hostname.exe": true, "arp.exe": true, "netsh.exe": true, "wmic.exe": true,
}

// startMetricsCollector — фоновый воркер самоизмерения при запуске.
func (a *App) startMetricsCollector() {
	select {
	case <-time.After(60 * time.Second):
	case <-a.stopCh:
		return
	}

	metric := a.measureApp(30 * time.Second)
	if metric == nil {
		return
	}
	if err := database.InsertAppMetric(metric); err != nil {
		a.log.Warn("metrics", "insert app metric: "+err.Error())
		return
	}
	a.log.Info("metrics", fmt.Sprintf(
		"self-measurement: cpu %.2f%% (%d/%d ms), ws %.1f MB, threads %d, handles %d, helpers %d, power %v",
		metric.CPUPercent, metric.CPUMs, metric.WallMs, metric.WorkingSetMB,
		metric.Threads, metric.Handles, metric.Children, metric.PowerSaving))
}

// measureApp снимает показатели процесса за окно времени.
func (a *App) measureApp(window time.Duration) *database.AppMetric {
	handle := windows.CurrentProcess()
	pid := windows.GetCurrentProcessId()

	cpuStart := processCpuMs(handle)
	if cpuStart < 0 {
		return nil
	}
	start := time.Now()
	select {
	case <-time.After(window):
	case <-a.stopCh:
		return nil
	}
	cpuEnd := processCpuMs(handle)
	if cpuEnd < 0 {
		return nil
	}
	wall := time.Since(start)

	var mem processMemoryCounters
	mem.CB = uint32(unsafe.Sizeof(mem))
	r, _, _ := procGetProcessMemoryInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&mem)), uintptr(mem.CB))
	if r == 0 {
		// без памяти процесса отчёт теряет «Применение» — фиксируем причину
		a.log.Warn("metrics", fmt.Sprintf("GetProcessMemoryInfo failed (cb=%d)", mem.CB))
		return nil
	}

	cpuMs := cpuEnd - cpuStart
	cpuPct := 0.0
	if wallMs := wall.Milliseconds(); wallMs > 0 {
		cpuPct = float64(cpuMs) * 100 / float64(wallMs)
	}

	return &database.AppMetric{
		Timestamp:    time.Now(),
		CPUPercent:   cpuPct,
		WallMs:       wall.Milliseconds(),
		CPUMs:        cpuMs,
		WorkingSetMB: float64(mem.WorkingSetSize) / (1 << 20),
		PrivateMB:    float64(mem.PagefileUsage) / (1 << 20),
		Threads:      countThreads(pid),
		Handles:      processHandleCount(handle),
		Children:     countHelperChildren(pid),
		PowerSaving:  power.Active(),
	}
}

// processCpuMs — суммарное kernel+user время процесса в мс; -1 при ошибке.
func processCpuMs(handle windows.Handle) int64 {
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return -1
	}
	ftToMs := func(ft windows.Filetime) int64 {
		return int64(uint64(ft.HighDateTime)<<32|uint64(ft.LowDateTime)) / 10000
	}
	return ftToMs(kernel) + ftToMs(user)
}

func processHandleCount(handle windows.Handle) int {
	var count uint32
	r, _, _ := procGetProcessHandleCount.Call(uintptr(handle), uintptr(unsafe.Pointer(&count)))
	if r == 0 {
		return 0
	}
	return int(count)
}

// countThreads — число потоков процесса через Toolhelp32Snapshot.
func countThreads(pid uint32) int {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(snap)

	count := 0
	var te windows.ThreadEntry32
	te.Size = uint32(unsafe.Sizeof(te))
	if err := windows.Thread32First(snap, &te); err != nil {
		return 0
	}
	for {
		if te.OwnerProcessID == pid {
			count++
		}
		if err := windows.Thread32Next(snap, &te); err != nil {
			return count
		}
	}
}

// countHelperChildren — сколько процессов-помощников заспавнено нашим
// процессом прямо сейчас.
func countHelperChildren(pid uint32) int {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(snap)

	count := 0
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if err := windows.Process32First(snap, &pe); err != nil {
		return 0
	}
	for {
		if pe.ParentProcessID == pid && helperImages[strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))] {
			count++
		}
		if err := windows.Process32Next(snap, &pe); err != nil {
			return count
		}
	}
}
