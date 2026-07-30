package sysinfo

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"zpui/internal/executil"
)

type ProcessInfo struct {
	Name       string  `json:"name"`
	PID        int     `json:"pid"`
	MemoryMB   float64 `json:"memory_mb"`
	CPUPercent float64 `json:"cpu_percent"`
}

type SystemResources struct {
	Processes    []ProcessInfo `json:"processes"`
	TotalMemMB   float64       `json:"total_mem_mb"`
	CPUPercent   float64       `json:"cpu_percent"`
	SystemRAMMB  float64       `json:"system_ram_mb"`
	AvailableRAM float64       `json:"available_ram_mb"`
	CPUModel     string        `json:"cpu_model"`
	NumCores     int           `json:"num_cores"`
}

var (
	prevCPUTimes  = map[string]time.Duration{}
	prevCPUPercent = map[string]float64{}
	prevCPUSample  = map[string]time.Time{}
	cacheMu        sync.Mutex
)

func GetSystemResources() SystemResources {
	result := SystemResources{
		Processes: []ProcessInfo{},
	}

	exePath, _ := os.Executable()
	exeName := filepath.Base(exePath)

	processes := []string{exeName, "winws.exe"}
	for _, name := range processes {
		if info := getProcessInfo(name); info != nil {
			if strings.EqualFold(name, exeName) {
				info.Name = "ZPUI"
			}
			result.Processes = append(result.Processes, *info)
			result.TotalMemMB += info.MemoryMB
		}
	}

	for _, p := range result.Processes {
		if p.CPUPercent > result.CPUPercent {
			result.CPUPercent = p.CPUPercent
		}
	}

	result.SystemRAMMB, result.AvailableRAM = getSystemRAM()
	result.NumCores = getNumCPU()

	return result
}

func getProcessInfo(name string) *ProcessInfo {
	cmd := executil.HiddenCmd("tasklist", "/FI", "IMAGENAME eq "+name, "/FO", "CSV", "/NH", "/V")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(strings.ToLower(line), strings.ToLower(name)) {
			continue
		}

		fields := parseCSVLine(line)
		if len(fields) < 8 {
			continue
		}

		pid, _ := strconv.Atoi(strings.TrimSpace(fields[1]))
		memKB := parseMemField(fields[4])
		cpuDur := parseCPUTime(fields[7])

		info := &ProcessInfo{
			Name:     name,
			PID:      pid,
			MemoryMB: memKB / 1024,
		}

		cacheMu.Lock()
		if prev, ok := prevCPUTimes[name]; ok {
			elapsed := time.Since(prevCPUSample[name]).Seconds()
			if elapsed > 0.5 {
				cpuDelta := cpuDur - prev
				deltaSec := cpuDelta.Seconds()
				if deltaSec >= 0 && deltaSec < elapsed*100 {
					info.CPUPercent = deltaSec / elapsed * 100
				}
			} else {
				info.CPUPercent = prevCPUPercent[name]
			}
		}
		prevCPUTimes[name] = cpuDur
		prevCPUPercent[name] = info.CPUPercent
		prevCPUSample[name] = time.Now()
		cacheMu.Unlock()

		return info
	}

	return nil
}

func parseCPUTime(s string) time.Duration {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")
	var h, m int
	var sec float64
	switch len(parts) {
	case 3:
		h, _ = strconv.Atoi(parts[0])
		m, _ = strconv.Atoi(parts[1])
		sec, _ = strconv.ParseFloat(parts[2], 64)
	case 2:
		m, _ = strconv.Atoi(parts[0])
		sec, _ = strconv.ParseFloat(parts[1], 64)
	default:
		return 0
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(sec*float64(time.Second))
}

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	ExtendedVirtual      uint64
}

func getSystemRAM() (totalMB, availMB float64) {
	dll := windows.NewLazySystemDLL("kernel32.dll")
	proc := dll.NewProc("GlobalMemoryStatusEx")
	ms := &memoryStatusEx{Length: 64}
	ret, _, _ := proc.Call(uintptr(unsafe.Pointer(ms)))
	if ret == 0 {
		return 0, 0
	}
	return float64(ms.TotalPhys / 1024 / 1024), float64(ms.AvailPhys / 1024 / 1024)
}

func getNumCPU() int {
	dll := windows.NewLazySystemDLL("kernel32.dll")
	proc := dll.NewProc("GetSystemInfo")
	type systemInfo struct {
		wProcessorArchitecture      uint16
		wReserved                   uint16
		dwPageSize                  uint32
		lpMinimumApplicationAddress uintptr
		lpMaximumApplicationAddress uintptr
		dwActiveProcessorMask       uintptr
		dwNumberOfProcessors        uint32
		dwProcessorType             uint32
		dwAllocationGranularity     uint32
		wProcessorLevel             uint16
		wProcessorRevision          uint16
	}
	si := &systemInfo{}
	proc.Call(uintptr(unsafe.Pointer(si)))
	return int(si.dwNumberOfProcessors)
}

func parseCSVLine(line string) []string {
	line = strings.Trim(line, "\"")
	parts := strings.Split(line, "\",\"")
	for i, p := range parts {
		parts[i] = strings.Trim(p, "\"")
	}
	return parts
}

func parseMemField(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, " КБ", "")
	s = strings.ReplaceAll(s, "K", "")
	val, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return val
}