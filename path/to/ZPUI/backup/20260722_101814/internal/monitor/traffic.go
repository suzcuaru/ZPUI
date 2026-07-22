package monitor

import (
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"zpui/internal/executil"
	"zpui/internal/logger"
)

type TrafficStats struct {
	DownloadBytes uint64    `json:"download_bytes"`
	UploadBytes   uint64    `json:"upload_bytes"`
	DownloadSpeed float64   `json:"download_speed"`
	UploadSpeed   float64   `json:"upload_speed"`
	Timestamp     time.Time `json:"timestamp"`
}

type TrafficMonitor struct {
	log           *logger.Logger
	current       atomic.Pointer[TrafficStats]
	previous      atomic.Pointer[TrafficStats]
	stopCh        chan struct{}
	running       bool
	mu            sync.Mutex
}

func NewTrafficMonitor(log *logger.Logger) *TrafficMonitor {
	m := &TrafficMonitor{
		log:    log,
		stopCh: make(chan struct{}),
	}

	initial := &TrafficStats{Timestamp: time.Now()}
	m.current.Store(initial)
	m.previous.Store(initial)

	go m.monitorLoop()

	return m
}

func (m *TrafficMonitor) Stop() {
	if m.running {
		m.running = false
		close(m.stopCh)
	}
}

func (m *TrafficMonitor) GetCurrentStats() *TrafficStats {
	return m.current.Load()
}

func (m *TrafficMonitor) monitorLoop() {
	m.running = true
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			stats := m.readNetworkStats()
			if stats != nil {
				prev := m.current.Load()
				m.previous.Store(prev)
				m.current.Store(stats)

				if prev != nil && !prev.Timestamp.IsZero() {
					elapsed := stats.Timestamp.Sub(prev.Timestamp).Seconds()
					if elapsed > 0 {
						stats.DownloadSpeed = float64(stats.DownloadBytes-prev.DownloadBytes) / elapsed
						stats.UploadSpeed = float64(stats.UploadBytes-prev.UploadBytes) / elapsed
					}
				}
			}
		}
	}
}

func (m *TrafficMonitor) readNetworkStats() *TrafficStats {
	stats := &TrafficStats{
		Timestamp: time.Now(),
	}

	cmd := executil.HiddenCmd("powershell", "-NoProfile", "-Command",
		`$bytes = Get-NetAdapterStatistics | Where-Object { $_.ReceivedBytes -gt 0 -or $_.SentBytes -gt 0 } | Measure-Object -Property ReceivedBytes,SentBytes -Sum; $bytes.Sum[0]; $bytes.Sum[1]`)

	output, err := cmd.Output()
	if err != nil {
		return stats
	}

	lines := regexp.MustCompile(`\r?\n`).Split(string(output), -1)
	var values []uint64
	for _, line := range lines {
		line = trimNonNumeric(line)
		if line == "" {
			continue
		}
		val, err := strconv.ParseUint(line, 10, 64)
		if err == nil {
			values = append(values, val)
		}
	}

	if len(values) >= 2 {
		stats.DownloadBytes = values[0]
		stats.UploadBytes = values[1]
	}

	return stats
}

func trimNonNumeric(s string) string {
	var result []byte
	for _, c := range []byte(s) {
		if (c >= '0' && c <= '9') || c == '.' {
			result = append(result, c)
		}
	}
	return string(result)
}

func FormatBytes(bytes uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.2f TB", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func FormatSpeed(bytesPerSec float64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytesPerSec >= GB:
		return fmt.Sprintf("%.2f GB/s", bytesPerSec/GB)
	case bytesPerSec >= MB:
		return fmt.Sprintf("%.2f MB/s", bytesPerSec/MB)
	case bytesPerSec >= KB:
		return fmt.Sprintf("%.2f KB/s", bytesPerSec/KB)
	default:
		return fmt.Sprintf("%.0f B/s", bytesPerSec)
	}
}

func GetNetworkInterfaces() []map[string]interface{} {
	return nil
}
