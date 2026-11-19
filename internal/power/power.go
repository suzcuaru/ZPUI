package power

import (
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Экономный режим: один мастер-тумблер + степень экономии (уровень 1..3) и
// порог заряда батареи (0 = «всегда», иначе 10..50%). Фоновые циклы читают
// Active()/MonitoringPaused()/ServicesPaused() каждый тик — отдельной
// рассылки изменений не требуется, всё восстанавливается само.

const (
	LevelLow    = 1 // растяжка интервалов
	LevelMedium = 2 // + пауза мониторинга трафика и снапшотов
	LevelMax    = 3 // + пауза устройств, ISP-детекта и синхронизации
)

// Порог заряда: 0 = «всегда», -1 = «от АКБ» (всё время работы от батареи),
// иначе 10..50 % с шагом 5.
const (
	ThresholdAlways    = 0
	ThresholdOnBattery = -1
)

var (
	mu        sync.Mutex
	enabled   bool
	level     = LevelMedium
	threshold = ThresholdAlways
)

// Set обновляет настройки экономного режима (вызывается при загрузке конфига
// и при его изменении из UI).
func Set(enabledOn bool, lvl, thr int) {
	if lvl < LevelLow || lvl > LevelMax {
		lvl = LevelLow
	}
	switch {
	case thr == ThresholdAlways || thr == ThresholdOnBattery:
	case thr >= 10 && thr <= 50:
	default:
		thr = ThresholdAlways
	}
	mu.Lock()
	enabled, level, threshold = enabledOn, lvl, thr
	mu.Unlock()
}

// Active — экономный режим активен: включён мастер-тумблер и либо порог
// «всегда», либо «от АКБ» (ноутбук не на зарядке), либо заряд батареи
// опустился до порога. На зарядке процент растёт выше порога и режим
// выключается сам. На ПК без батареи пороги неприменимы — включение
// режима активирует его сразу.
func Active() bool {
	mu.Lock()
	on, thr := enabled, threshold
	mu.Unlock()
	if !on {
		return false
	}
	switch thr {
	case ThresholdAlways:
		return true
	case ThresholdOnBattery:
		return onBatteryPower()
	}
	pct, has := BatteryPercent()
	if !has {
		return true // нет АКБ (ПК) — порог неприменим
	}
	return pct <= thr
}

// Level — настроенная степень экономии (1..3); имеет смысл при Active().
func Level() int {
	mu.Lock()
	defer mu.Unlock()
	return level
}

// Stretch возвращает интервал, растянутый в 5 раз при активном режиме
// (уровень 1 и выше).
func Stretch(base time.Duration) time.Duration {
	if Active() {
		return base * 5
	}
	return base
}

// MonitoringPaused — уровень 2+: мониторинг трафика и записи снапшотов
// приостановлены (скорости показываются нулями).
func MonitoringPaused() bool {
	return Active() && Level() >= LevelMedium
}

// ServicesPaused — уровень 3: трекер устройств, ISP-детект и sync-маяк
// приостановлены.
func ServicesPaused() bool {
	return Active() && Level() >= LevelMax
}

// systemPowerStatus — SYSTEM_POWER_STATUS (kernel32.GetSystemPowerStatus).
type systemPowerStatus struct {
	ACLineStatus        byte // 1 = от сети, 0 = от батареи, 255 = неизвестно
	BatteryFlag         byte // 128 = нет батареи
	BatteryLifePercent  byte // 0..100, 255 = неизвестно
	Reserved1           byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

var (
	procGetSystemPowerStatus = syscall.NewLazyDLL("kernel32.dll").NewProc("GetSystemPowerStatus")
)

// onBatteryPower — ноутбук питается от батареи (не на зарядке).
func onBatteryPower() bool {
	var ps systemPowerStatus
	r, _, _ := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&ps)))
	return r != 0 && ps.ACLineStatus == 0
}

// HasBattery — на устройстве есть батарея (ноутбук).
func HasBattery() bool {
	var ps systemPowerStatus
	r, _, _ := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&ps)))
	if r == 0 {
		return false
	}
	return ps.BatteryFlag != 128
}

// BatteryPercent — текущий процент заряда (0..100) и признак его доступности.
func BatteryPercent() (int, bool) {
	var ps systemPowerStatus
	r, _, _ := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&ps)))
	if r == 0 {
		return 0, false
	}
	if ps.BatteryLifePercent == 255 || ps.BatteryFlag == 128 {
		return 0, false
	}
	return int(ps.BatteryLifePercent), true
}
