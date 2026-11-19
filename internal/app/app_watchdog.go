package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const (
	configStallDumpAfter    = 30 * time.Second
	configStallRestartAfter = 2 * time.Minute
	configStallDumpGap      = time.Minute
	configStallRestartGap   = 10 * time.Minute
)

// watchConfigSaves detects a save stuck under the config lock (the UI hangs
// because every config read waits for it), dumps goroutines for diagnosis and
// restarts the app if the stall does not clear.
func (a *App) watchConfigSaves() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	var lastDump, lastRestart time.Time
	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			age := a.cfg.SaveStallAge()
			if age < configStallDumpAfter {
				continue
			}
			if time.Since(lastDump) >= configStallDumpGap {
				lastDump = time.Now()
				a.dumpGoroutines(age)
			}
			if age >= configStallRestartAfter && time.Since(lastRestart) >= configStallRestartGap {
				lastRestart = time.Now()
				a.log.Error("app", fmt.Sprintf("config save stuck for %s — restarting ZPUI to recover", age.Round(time.Second)))
				go a.Restart()
			}
		}
	}
}

func (a *App) dumpGoroutines(stall time.Duration) {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, len(buf)*2)
	}
	header := fmt.Sprintf("config save stalled for %s (started %s)\n\n",
		stall.Round(time.Second), time.Now().Add(-stall).Format(time.RFC3339))
	path := filepath.Join(a.cfg.LogsDir(), "stacks-"+time.Now().Format("20060102-150405")+".txt")
	if err := os.WriteFile(path, append([]byte(header), buf...), 0644); err != nil {
		a.log.Error("app", "config stall: dump failed: "+err.Error())
		return
	}
	a.log.Error("app", fmt.Sprintf("config save stalled for %s — goroutines dumped to %s", stall.Round(time.Second), path))
}
