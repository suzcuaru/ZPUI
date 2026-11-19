package app

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestModuleBackoff(t *testing.T) {
	cases := []struct {
		restartNum int
		want       time.Duration
	}{
		{1, time.Second},
		{2, 5 * time.Second},
		{3, 15 * time.Second},
		{4, 60 * time.Second},
		{7, 60 * time.Second},
		{0, time.Second},
		{-3, time.Second},
	}
	for _, c := range cases {
		if got := moduleBackoff(c.restartNum); got != c.want {
			t.Errorf("moduleBackoff(%d) = %v, want %v", c.restartNum, got, c.want)
		}
	}
}

func TestModuleRestartsInWindow(t *testing.T) {
	now := time.Now()
	recent := []time.Time{
		now.Add(-1 * time.Minute),
		now.Add(-9 * time.Minute),
		now.Add(-11 * time.Minute),
		now.Add(-30 * time.Minute),
	}
	if got := moduleRestartsInWindow(recent, now, moduleRestartWindow); got != 2 {
		t.Errorf("moduleRestartsInWindow = %d, want 2", got)
	}
	if got := moduleRestartsInWindow(nil, now, moduleRestartWindow); got != 0 {
		t.Errorf("moduleRestartsInWindow(nil) = %d, want 0", got)
	}
}

func TestSupervisorLimitFivePerTenMinutes(t *testing.T) {
	reg := newModuleRegistry()
	now := time.Now()
	for i := 1; i <= 5; i++ {
		backoff, giveUp := reg.failOnce("x", now.Add(time.Duration(i)*time.Second), 0, "boom")
		if giveUp {
			t.Fatalf("panic #%d: unexpected giveUp", i)
		}
		if backoff <= 0 {
			t.Fatalf("panic #%d: zero backoff", i)
		}
	}
	// 5 рестартов в окне исчерпали лимит — шестая паника не рестартует.
	_, giveUp := reg.failOnce("x", now.Add(6*time.Second), 0, "boom")
	if !giveUp {
		t.Fatal("limit 5/10min: expected giveUp on 6th panic")
	}
	st, ok := reg.statusOf("x")
	if !ok || st.Status != modFailed {
		t.Fatalf("status = %q, want %q", st.Status, modFailed)
	}
	if st.Restarts != 5 || st.TotalRestarts != 5 {
		t.Fatalf("restarts = %d/%d, want 5/5", st.Restarts, st.TotalRestarts)
	}
}

func TestSupervisorStableReset(t *testing.T) {
	reg := newModuleRegistry()
	now := time.Now()
	for i := 1; i <= 4; i++ {
		reg.failOnce("x", now.Add(time.Duration(i)*time.Second), 0, "boom")
	}
	// Модуль прожил дольше moduleStableAfter и снова упал: старые метки
	// выпали из окна (лимит не сработал), счётчик подряд сброшен.
	later := now.Add(11 * time.Minute)
	backoff, giveUp := reg.failOnce("x", later, moduleStableAfter+time.Minute, "boom")
	if giveUp {
		t.Fatal("stable module must not hit the limit after window slid")
	}
	if backoff != time.Second {
		t.Fatalf("backoff after stable reset = %v, want 1s", backoff)
	}
	st, _ := reg.statusOf("x")
	if st.Restarts != 1 {
		t.Fatalf("restarts after reset = %d, want 1", st.Restarts)
	}
}

func TestSupervisorBeginEnd(t *testing.T) {
	reg := newModuleRegistry()
	reg.begin("m")
	if st, _ := reg.statusOf("m"); st.Status != modRunning {
		t.Fatalf("after begin: %q, want %q", st.Status, modRunning)
	}
	reg.end("m")
	if st, _ := reg.statusOf("m"); st.Status != modStopped {
		t.Fatalf("after end: %q, want %q", st.Status, modStopped)
	}
	// failed не перезаписывается штатным завершением.
	reg.failOnce("f", time.Now(), 0, "boom")
	reg.failOnce("f", time.Now(), 0, "boom")
	reg.failOnce("f", time.Now(), 0, "boom")
	reg.failOnce("f", time.Now(), 0, "boom")
	reg.failOnce("f", time.Now(), 0, "boom")
	reg.failOnce("f", time.Now(), 0, "boom")
	reg.end("f")
	if st, _ := reg.statusOf("f"); st.Status != modFailed {
		t.Fatalf("failed overwritten by end: %q", st.Status)
	}
}

func TestPanicBrief(t *testing.T) {
	got := panicBrief("runtime error: index out of range [5]\ngoroutine stack...")
	if got != "runtime error: index out of range [5]" {
		t.Fatalf("panicBrief = %q", got)
	}
	long := strings.Repeat("a", moduleBriefMax+50)
	if n := len(panicBrief(long)); n != moduleBriefMax {
		t.Fatalf("panicBrief len = %d, want %d", n, moduleBriefMax)
	}
}

func TestRunSupervised(t *testing.T) {
	if msg := runSupervised(func() {}); msg != "" {
		t.Fatalf("normal fn: msg = %q, want empty", msg)
	}
	msg := runSupervised(func() { panic(errors.New("kaboom")) })
	if !strings.Contains(msg, "kaboom") || !strings.Contains(msg, "goroutine") {
		t.Fatalf("panic fn: msg = %q", msg)
	}
	if brief := panicBrief(msg); brief != "kaboom" {
		t.Fatalf("brief = %q", brief)
	}
}
