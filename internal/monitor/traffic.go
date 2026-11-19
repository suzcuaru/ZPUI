package monitor

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"zpui/internal/logger"
	"zpui/internal/power"

	"golang.org/x/sys/windows"
)

type TrafficStats struct {
	DownloadBytes uint64    `json:"download_bytes"`
	UploadBytes   uint64    `json:"upload_bytes"`
	DownloadSpeed float64   `json:"download_speed"`
	UploadSpeed   float64   `json:"upload_speed"`
	Timestamp     time.Time `json:"timestamp"`
}

type TrafficMonitor struct {
	log     *logger.Logger
	current atomic.Pointer[TrafficStats]
	stopCh  chan struct{}
	stopped atomic.Bool

	mu      sync.Mutex
	lastIfs map[uint32]ifCounters
	session TrafficStats
	zeroed  atomic.Bool
}

// ifCounters — снимок 32-битных счётчиков одного интерфейса (MIB_IFROW).
type ifCounters struct{ in, out uint32 }

func NewTrafficMonitor(log *logger.Logger) *TrafficMonitor {
	m := &TrafficMonitor{
		log:    log,
		stopCh: make(chan struct{}),
	}

	m.current.Store(&TrafficStats{Timestamp: time.Now()})
	go m.monitorLoop()

	return m
}

func (m *TrafficMonitor) Stop() {
	if m.stopped.CompareAndSwap(false, true) {
		close(m.stopCh)
	}
}

func (m *TrafficMonitor) GetCurrentStats() *TrafficStats {
	return m.current.Load()
}

func (m *TrafficMonitor) monitorLoop() {
	// Горутин живёт весь сеанс приложения: паника здесь убивала бы весь
	// процесс (recover в main ловит только главный горутин).
	defer func() {
		if r := recover(); r != nil && m.log != nil {
			m.log.Error("monitor", fmt.Sprintf("traffic monitor panic: %v", r))
		}
	}()

	for {
		// Экономный режим растягивает опрос трафика 2с → 10с: счётчики
		// читаются в процессе, но каждый тик пишет скорость в UI-кэш.
		interval := power.Stretch(2 * time.Second)
		select {
		case <-m.stopCh:
			return
		case <-time.After(interval):
		}

		// Экономный режим (уровень 2+): мониторинг приостановлен, скорости
		// показываем нулями.
		if power.MonitoringPaused() {
			m.zeroSpeedsOnce()
			continue
		}
		m.zeroed.Store(false)

		stats := m.sample()
		if stats == nil {
			continue
		}

		prev := m.current.Load()
		if prev != nil && !prev.Timestamp.IsZero() {
			elapsed := stats.Timestamp.Sub(prev.Timestamp).Seconds()
			if elapsed > 0 {
				// Сессия монотонна (guard в sample), поэтому дельта без underflow.
				stats.DownloadSpeed = float64(stats.DownloadBytes-prev.DownloadBytes) / elapsed
				stats.UploadSpeed = float64(stats.UploadBytes-prev.UploadBytes) / elapsed
			}
		}

		m.current.Store(stats)
	}
}

func (m *TrafficMonitor) zeroSpeedsOnce() {
	if m.zeroed.CompareAndSwap(false, true) {
		if cur := m.current.Load(); cur != nil {
			frozen := *cur
			frozen.DownloadSpeed = 0
			frozen.UploadSpeed = 0
			frozen.Timestamp = time.Now()
			m.current.Store(&frozen)
		}
	}
}

// sample снимает счётчики интерфейсов через GetIfEntry (iphlpapi, в
// процессе; ранее каждые 2 секунды спавнился полный PowerShell) и накапливает
// трафик за сессию по per-interface дельтам.
//
// Почему GetIfEntry (MIB_IFROW, 32-бит), а не GetIfEntry2Ex (MIB_IFROW2,
// 64-бит): приложение собирается под windows/386, где Go выравнивает uint64
// по 4 байта — Windows, записывая MIB_IFROW2 с 8-байтным выравниванием,
// выходил за пределы Go-структуры и портил heap (случайные паники вида
// «assignment to entry in nil map» через ~2с после старта).
//
// Дельты, а не «сумма с момента загрузки Windows»: сумма по всем интерфейсам
// (включая WAN-miniports и vSwitch-ы WSL/Hyper-V) показывает терабайты, не
// имеющие отношения к реальному трафику; а сброс счётчиков интерфейса (сон,
// VPN, перезапуск WSL) без guard'а даёт ложные скачки скорости.
func (m *TrafficMonitor) sample() *TrafficStats {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.lastIfs == nil {
		m.lastIfs = make(map[uint32]ifCounters, len(ifaces))
	}
	now := make(map[uint32]ifCounters, len(ifaces))

	var dIn, dOut uint64
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		idx := uint32(ifc.Index)
		row := &windows.MibIfRow{Index: idx}
		if err := windows.GetIfEntry(row); err != nil {
			continue
		}
		// Отсекаем только pseudo-интерфейсы без MAC (тип 53 и т.п.).
		// ВАЖНО: не фильтруем по OperStatus — активные Wi-Fi/Ethernet
		// адаптеры в Windows часто отдают Dormant(5), а не Up(1), и фильтр
		// «убивал» весь мониторинг. Нулевые счётчики безвредны для дельт.
		if row.PhysAddrLen == 0 {
			continue
		}

		cur := ifCounters{in: row.InOctets, out: row.OutOctets}
		now[idx] = cur

		// Дельта только для интерфейса из прошлой выборки; сброс/переполнение
		// обрабатывает counterDelta, новый интерфейс — просто базлайн.
		if prev, ok := m.lastIfs[idx]; ok {
			dIn += counterDelta(prev.in, cur.in)
			dOut += counterDelta(prev.out, cur.out)
		}
	}
	m.lastIfs = now

	m.session.DownloadBytes += dIn
	m.session.UploadBytes += dOut

	return &TrafficStats{
		Timestamp:     time.Now(),
		DownloadBytes: m.session.DownloadBytes,
		UploadBytes:   m.session.UploadBytes,
	}
}

// counterDelta — разность 32-битных счётчиков октетов. cur < prev означает
// либо переполнение (близко к 2^32 — считаем через границу), либо сброс
// счётчика (сон/VPN/пересоздание адаптера — считаем нулем, без ложных
// гигабайт в скорости).
func counterDelta(prev, cur uint32) uint64 {
	if cur >= prev {
		return uint64(cur - prev)
	}
	if prev > 0xF8000000 {
		return uint64(^uint32(0)-prev) + uint64(cur) + 1
	}
	return 0
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
