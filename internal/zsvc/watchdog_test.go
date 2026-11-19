package zsvc

import (
	"testing"
	"time"
)

func TestBackoffFor(t *testing.T) {
	tests := []struct {
		crashNum int
		want     time.Duration
	}{
		{1, 1 * time.Second},
		{2, 5 * time.Second},
		{3, 15 * time.Second},
		{4, 15 * time.Second},
		{100, 15 * time.Second},
		{0, 1 * time.Second},
	}
	for _, tt := range tests {
		if got := backoffFor(tt.crashNum); got != tt.want {
			t.Errorf("backoffFor(%d) = %v, want %v", tt.crashNum, got, tt.want)
		}
	}
}

func TestCrashLimitReached(t *testing.T) {
	if crashLimitReached(4) {
		t.Errorf("4 crashes must not reach limit")
	}
	for _, n := range []int{5, 6, 10} {
		if !crashLimitReached(n) {
			t.Errorf("%d crashes must reach limit", n)
		}
	}
	if crashLimitReached(0) {
		t.Errorf("0 crashes must not reach limit")
	}
}

func TestNextCrashCount(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)

	// Первый краш: счётчик = 1, окно стартует сейчас.
	cnt, ws := nextCrashCount(0, time.Time{}, now)
	if cnt != 1 || !ws.Equal(now) {
		t.Errorf("first crash: got cnt=%d ws=%v, want 1/%v", cnt, ws, now)
	}

	// Второй краш в пределах часа: счётчик растёт, окно не двигается.
	cnt, ws = nextCrashCount(1, now, now.Add(30*time.Minute))
	if cnt != 2 || !ws.Equal(now) {
		t.Errorf("second crash: got cnt=%d ws=%v, want 2/%v", cnt, ws, now)
	}

	// Краш спустя час с лишним: окно перезапускается.
	cnt, ws = nextCrashCount(4, now, now.Add(crashWindow+time.Minute))
	if cnt != 1 || !ws.Equal(now.Add(crashWindow+time.Minute)) {
		t.Errorf("window reset: got cnt=%d ws=%v", cnt, ws)
	}

	// Граница: ровно час — окно ещё живо.
	cnt, _ = nextCrashCount(3, now, now.Add(crashWindow))
	if cnt != 4 {
		t.Errorf("exact hour boundary: got cnt=%d, want 4", cnt)
	}
}

func TestEngineTick(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	cooldown := now.Add(engineReviveCooldown)

	// Живой движок: сброс счётчика, подъём не нужен.
	if revive, ticks := engineTick(true, true, true, false, 5, time.Time{}, now); revive || ticks != 0 {
		t.Errorf("alive: got revive=%v ticks=%d, want false/0", revive, ticks)
	}
	// Служба не установлена: сброс.
	if revive, ticks := engineTick(false, false, false, false, 1, time.Time{}, now); revive || ticks != 0 {
		t.Errorf("not installed: got revive=%v ticks=%d, want false/0", revive, ticks)
	}
	// Автозапуск службы отключён (SERVICE_DISABLED): только сброс счётчика.
	if revive, ticks := engineTick(true, false, false, false, 1, time.Time{}, now); revive || ticks != 0 {
		t.Errorf("autostart off: got revive=%v ticks=%d, want false/0", revive, ticks)
	}
	// Первый мёртвый тик: счётчик = 1, подъёма ещё нет.
	if revive, ticks := engineTick(true, true, false, false, 0, time.Time{}, now); revive || ticks != 1 {
		t.Errorf("first dead tick: got revive=%v ticks=%d, want false/1", revive, ticks)
	}
	// Второй подряд мёртвый тик (30с): подъём.
	if revive, ticks := engineTick(true, true, false, false, 1, time.Time{}, now); !revive || ticks != 2 {
		t.Errorf("second dead tick: got revive=%v ticks=%d, want true/2", revive, ticks)
	}
	// Пауза установки движка (svcUpdating): счётчик растёт, подъёма нет.
	if revive, ticks := engineTick(true, true, false, true, 1, time.Time{}, now); revive || ticks != 2 {
		t.Errorf("updating: got revive=%v ticks=%d, want false/2", revive, ticks)
	}
	// Cooldown после недавней попытки: подъёма нет, счётчик растёт.
	if revive, ticks := engineTick(true, true, false, false, 2, cooldown, now); revive || ticks != 3 {
		t.Errorf("cooldown: got revive=%v ticks=%d, want false/3", revive, ticks)
	}
	// Cooldown истёк: подъём снова возможен.
	if revive, _ := engineTick(true, true, false, false, 3, cooldown, now.Add(engineReviveCooldown+time.Second)); !revive {
		t.Errorf("cooldown expired: got revive=false, want true")
	}
}
