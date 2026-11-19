package power

import "testing"

func TestMasterSwitch(t *testing.T) {
	Set(false, LevelMax, 0)
	if Active() || MonitoringPaused() || ServicesPaused() {
		t.Error("master off: nothing should be active")
	}
	Set(true, LevelMax, 0) // порог «всегда»
	if !Active() {
		t.Error("master on + always: Active should be true")
	}
	if !MonitoringPaused() || !ServicesPaused() {
		t.Error("max level: both pauses should be true")
	}
}

func TestLevels(t *testing.T) {
	Set(true, LevelLow, 0)
	if MonitoringPaused() || ServicesPaused() {
		t.Error("low level: no pauses expected")
	}
	Set(true, LevelMedium, 0)
	if !MonitoringPaused() {
		t.Error("medium: monitoring pause expected")
	}
	if ServicesPaused() {
		t.Error("medium: services pause NOT expected")
	}
	Set(true, 99, 0) // некорректный уровень — клампится в низкий
	if Level() != LevelLow {
		t.Errorf("level clamp = %d, want %d", Level(), LevelLow)
	}
	Set(true, LevelLow, 0)
}

func TestStretch(t *testing.T) {
	Set(true, LevelLow, 0)
	if got := Stretch(2 * 1e9); got != 10*1e9 {
		t.Errorf("Stretch in power saving = %v, want x5", got)
	}
	Set(false, LevelLow, 0)
	if got := Stretch(2 * 1e9); got != 2*1e9 {
		t.Errorf("Stretch normal = %v, want unchanged", got)
	}
}

// Порог: «всегда»(0) и «от АКБ»(-1) валидны, проценты 10..50 валидны,
// остальное клампится в «всегда». При «от АКБ» активность совпадает с
// реальным состоянием батареи.
func TestThresholdClamp(t *testing.T) {
	Set(true, LevelLow, 77)
	if got := currentThreshold(); got != ThresholdAlways {
		t.Errorf("threshold 77 should clamp to always(0), got %d", got)
	}
	Set(true, LevelLow, -1)
	if got := currentThreshold(); got != ThresholdOnBattery {
		t.Errorf("threshold -1 (on battery) should be kept, got %d", got)
	}
	if Active() != onBatteryPower() {
		t.Error("on-battery threshold: Active should match onBatteryPower")
	}
	Set(true, LevelLow, 30)
	if got := currentThreshold(); got != 30 {
		t.Errorf("threshold 30 should be kept, got %d", got)
	}
	Set(true, LevelLow, ThresholdAlways)
}

func currentThreshold() int {
	mu.Lock()
	defer mu.Unlock()
	return threshold
}

// Железо: не должно паниковать на любой системе.
func TestHardwareQueries(t *testing.T) {
	_ = HasBattery()
	_, _ = BatteryPercent()
}
