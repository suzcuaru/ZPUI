package zsvc

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()
	if !c.Autostart {
		t.Errorf("Autostart: want true, got false")
	}
	if c.Hidden {
		t.Errorf("Hidden: want false, got true")
	}
	if c.Status != StatusIdle {
		t.Errorf("Status: want %q, got %q", StatusIdle, c.Status)
	}
	if c.LastExitKind != ExitNone {
		t.Errorf("LastExitKind: want %q, got %q", ExitNone, c.LastExitKind)
	}
	if c.CrashCount != 0 {
		t.Errorf("CrashCount: want 0, got %d", c.CrashCount)
	}
	if !c.LastAppStart.IsZero() || !c.LastExitAt.IsZero() || !c.CrashWindowStart.IsZero() || !c.WatchdogPausedUntil.IsZero() {
		t.Errorf("times must be zero in defaults")
	}
	if c.AppPath != "" || c.ServiceVersion != "" || c.LastCrashReason != "" {
		t.Errorf("strings must be empty in defaults")
	}
}

func TestStatusAndExitKindValidity(t *testing.T) {
	for _, s := range []string{StatusIdle, StatusWatching, StatusPaused, StatusFailed} {
		if !isValidStatus(s) {
			t.Errorf("status %q must be valid", s)
		}
	}
	if isValidStatus("bogus") {
		t.Errorf("bogus status must be invalid")
	}
	for _, k := range []string{ExitNone, ExitClean, ExitCrash, ExitLogoff} {
		if !isValidExitKind(k) {
			t.Errorf("exit kind %q must be valid", k)
		}
	}
	if isValidExitKind("bogus") {
		t.Errorf("bogus exit kind must be invalid")
	}
}
