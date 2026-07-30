package zapret

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"zpui/internal/blockcheck"
)

func writeGameFilter(t *testing.T, m *Manager, content string) {
	t.Helper()
	utilsDir := filepath.Join(m.cfg.GetZapretPath(), "utils")
	if err := os.MkdirAll(utilsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(utilsDir, "game_filter.enabled"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadGameFilter_DisabledWhenNoFile(t *testing.T) {
	m, _, _ := newTestManager(t)
	mode, tcp, udp := m.LoadGameFilter()
	if mode != "disabled" || tcp != "12" || udp != "12" {
		t.Errorf("got (%q,%q,%q), want (disabled,12,12)", mode, tcp, udp)
	}
}

func TestLoadGameFilter_Modes(t *testing.T) {
	cases := []struct {
		content string
		wMode   string
		wTCP    string
		wUDP    string
	}{
		{"all", "all", "1024-65535", "1024-65535"},
		{"tcp", "tcp", "1024-65535", "12"},
		{"udp", "udp", "12", "1024-65535"},
		{"ALL", "all", "1024-65535", "1024-65535"},
		{"  tcp\n", "tcp", "1024-65535", "12"},
		{"unknown", "disabled", "12", "12"},
		{"disabled", "disabled", "12", "12"},
	}
	for _, c := range cases {
		t.Run(c.content, func(t *testing.T) {
			m, _, _ := newTestManager(t)
			writeGameFilter(t, m, c.content)
			mode, tcp, udp := m.LoadGameFilter()
			if mode != c.wMode || tcp != c.wTCP || udp != c.wUDP {
				t.Errorf("LoadGameFilter(%q) = (%q,%q,%q), want (%q,%q,%q)",
					c.content, mode, tcp, udp, c.wMode, c.wTCP, c.wUDP)
			}
		})
	}
}

func TestSetGameFilter_Invalid(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.SetGameFilter("bogus"); err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestSetGameFilter_WriteAndRemove(t *testing.T) {
	m, _, _ := newTestManager(t)
	flag := filepath.Join(m.cfg.GetZapretPath(), "utils", "game_filter.enabled")

	for _, mode := range []string{"all", "tcp", "udp"} {
		if err := m.SetGameFilter(mode); err != nil {
			t.Fatalf("SetGameFilter(%q): %v", mode, err)
		}
		data, err := os.ReadFile(flag)
		if err != nil {
			t.Fatalf("file not written for %q: %v", mode, err)
		}
		if string(data) != mode {
			t.Errorf("file content = %q, want %q", data, mode)
		}
		mode2, tcp, udp := m.LoadGameFilter()
		if mode2 != mode {
			t.Errorf("LoadGameFilter mode = %q, want %q", mode2, mode)
		}
		switch mode {
		case "all":
			if tcp != "1024-65535" || udp != "1024-65535" {
				t.Errorf("tcp/udp = %q/%q", tcp, udp)
			}
		case "tcp":
			if tcp != "1024-65535" || udp != "12" {
				t.Errorf("tcp/udp = %q/%q", tcp, udp)
			}
		case "udp":
			if tcp != "12" || udp != "1024-65535" {
				t.Errorf("tcp/udp = %q/%q", tcp, udp)
			}
		}
	}

	// "disabled" удаляет файл.
	if err := m.SetGameFilter("disabled"); err != nil {
		t.Fatalf("SetGameFilter(disabled): %v", err)
	}
	if _, err := os.Stat(flag); !os.IsNotExist(err) {
		t.Errorf("expected flag removed, got err=%v", err)
	}
	if m.gameFilterTCP != "12" || m.gameFilterUDP != "12" {
		t.Errorf("after disable tcp/udp = %q/%q", m.gameFilterTCP, m.gameFilterUDP)
	}

	// Повторный disabled на отсутствующий файл возвращает ошибку удаления.
	if err := m.SetGameFilter("disabled"); err == nil {
		t.Error("expected error when removing already-absent flag")
	}
}

func writeStrategies(t *testing.T, m *Manager, names []string) {
	t.Helper()
	for _, n := range names {
		writeStrategyFile(t, m.cfg.StrategyPath(n), "winws.exe --filter-tcp=443\n")
	}
}

func TestListStrategies(t *testing.T) {
	m, _, _ := newTestManager(t)
	writeStrategies(t, m, []string{"general", "general (ALT)", "general2"})

	// Не-стратегии: service-префикс, не general, не .bat.
	writeStrategyFile(t, m.cfg.StrategyPath("service"), "winws.exe --filter-tcp=443\n")
	writeStrategyFile(t, filepath.Join(m.cfg.GetZapretPath(), "general.txt"), "x")
	writeStrategyFile(t, filepath.Join(m.cfg.GetZapretPath(), "other.bat"), "x")
	if err := os.MkdirAll(filepath.Join(m.cfg.GetZapretPath(), "generalsub"), 0755); err != nil {
		t.Fatal(err)
	}

	got := m.ListStrategies()
	if len(got) != 3 {
		t.Fatalf("got %d strategies, want 3: %+v", len(got), got)
	}
	// Сортировка по Filename.
	want := []string{"general (ALT).bat", "general.bat", "general2.bat"}
	for i, s := range got {
		if s.Filename != want[i] {
			t.Errorf("[%d] Filename = %q, want %q", i, s.Filename, want[i])
		}
		if s.Name != want[i][:len(want[i])-4] {
			t.Errorf("[%d] Name = %q", i, s.Name)
		}
		if s.Current {
			t.Errorf("[%d] should not be current", i)
		}
	}
}

func TestListStrategies_CurrentMarked(t *testing.T) {
	m, _, _ := newTestManager(t)
	writeStrategies(t, m, []string{"general", "general2"})
	if err := m.cfg.SetCurrentStrategy("general2.bat"); err != nil {
		t.Fatal(err)
	}
	got := m.ListStrategies()
	for _, s := range got {
		if s.Filename == "general2.bat" && !s.Current {
			t.Errorf("general2.bat must be current")
		}
		if s.Filename == "general.bat" && s.Current {
			t.Errorf("general.bat must not be current")
		}
	}
}

func TestListStrategies_EmptyDir(t *testing.T) {
	m, _, _ := newTestManager(t)
	if got := m.ListStrategies(); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestListStrategies_MissingDir(t *testing.T) {
	m, _, _ := newTestManager(t)
	m.cfg.SetZapretPath(m.cfg.GetZapretPath() + "\\nonexistent")
	if got := m.ListStrategies(); got != nil {
		t.Errorf("expected nil for missing dir, got %v", got)
	}
}

func TestDefaultStrategyName_PrefersAlt(t *testing.T) {
	m, _, _ := newTestManager(t)
	writeStrategies(t, m, []string{"general", "general (ALT)", "general2"})
	if got := m.DefaultStrategyName(); got != "general (ALT).bat" {
		t.Errorf("got %q, want general (ALT).bat", got)
	}
}

func TestDefaultStrategyName_FallbackToFirst(t *testing.T) {
	m, _, _ := newTestManager(t)
	writeStrategies(t, m, []string{"general", "general2"})
	if got := m.DefaultStrategyName(); got != "general.bat" {
		t.Errorf("got %q, want general.bat", got)
	}
}

func TestDefaultStrategyName_Empty(t *testing.T) {
	m, _, _ := newTestManager(t)
	if got := m.DefaultStrategyName(); got != "general.bat" {
		t.Errorf("got %q, want general.bat", got)
	}
}

func TestGetCurrentStrategy(t *testing.T) {
	m, _, _ := newTestManager(t)
	if got := m.GetCurrentStrategy(); got != "general.bat" {
		t.Errorf("default = %q, want general.bat", got)
	}
}

func TestIsAutoTestRunning_Toggle(t *testing.T) {
	m, _, _ := newTestManager(t)
	_ = m
	defer resetAutoTestState()
	if m.IsAutoTestRunning() {
		t.Fatal("should be false initially")
	}
	autoTestMu.Lock()
	autoTestActive = true
	autoTestMu.Unlock()
	if !m.IsAutoTestRunning() {
		t.Error("should be true after set")
	}
	m.CancelAutoTest()
	if m.IsAutoTestRunning() {
		t.Error("should be false after cancel")
	}
}

func TestCancelAutoTest_FiresCancelFunc(t *testing.T) {
	defer resetAutoTestState()
	ctx, cancel := context.WithCancel(context.Background())
	fired := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(fired)
	}()

	autoTestMu.Lock()
	autoTestCancel = cancel
	autoTestMu.Unlock()

	m, _, _ := newTestManager(t)
	m.CancelAutoTest()

	<-fired
	autoTestMu.Lock()
	nilAfter := autoTestCancel == nil
	autoTestMu.Unlock()
	if !nilAfter {
		t.Error("autoTestCancel should be nil after cancel")
	}
}

func TestRunAutoTest_AlreadyRunning(t *testing.T) {
	defer resetAutoTestState()
	autoTestMu.Lock()
	autoTestActive = true
	autoTestMu.Unlock()

	m, _, _ := newTestManager(t)
	results := make(chan AutoTestResult, 4)
	done := make(chan struct{})

	go m.RunAutoTest(context.Background(), results, done)

	select {
	case r := <-results:
		if r.Type != "done" || r.Error == "" {
			t.Errorf("expected done+error, got %+v", r)
		}
	case <-done:
	}

	<-done
}

func TestAutoSelectAndApply_AlreadyRunning(t *testing.T) {
	defer resetAutoTestState()
	autoTestMu.Lock()
	autoTestActive = true
	autoTestMu.Unlock()

	m, _, _ := newTestManager(t)
	results := make(chan AutoTestResult, 4)
	done := make(chan struct{})

	go m.AutoSelectAndApply(context.Background(), results, done)

	<-done
	select {
	case r := <-results:
		if r.Type != "done" || r.Error == "" {
			t.Errorf("expected done+error, got %+v", r)
		}
	default:
		t.Error("expected a result message")
	}
}

func TestFilterSkippedTargets(t *testing.T) {
	m, _, _ := newTestManager(t)
	// skip-resources.txt рядом с config.json.
	skipPath := filepath.Join(filepath.Dir(m.cfg.StrategyPath("x")), "..", "skip-resources.txt")
	skipPath = filepath.Clean(skipPath)
	if err := os.WriteFile(skipPath, []byte("discord.com\n# comment\n\ndrive.google.com\n"), 0644); err != nil {
		t.Fatal(err)
	}

	in := []blockcheck.BulkTarget{
		{Name: "DiscordMain"},
		{Name: "YouTubeWeb"},
		{Name: "Drive"},
	}
	out := m.filterSkippedTargets(in)
	if len(out) != 1 {
		t.Fatalf("got %d targets, want 1: %+v", len(out), out)
	}
	if out[0].Name != "YouTubeWeb" {
		t.Errorf("got %q, want YouTubeWeb", out[0].Name)
	}
}

func TestLoadTestTargets_Defaults(t *testing.T) {
	m, _, _ := newTestManager(t)
	got := m.loadTestTargets()
	if len(got) != 4 {
		t.Fatalf("got %d targets, want 4 defaults", len(got))
	}
}

func TestLoadTestTargets_FromFile(t *testing.T) {
	m, _, _ := newTestManager(t)
	listsDir := m.cfg.ListsDir()
	if err := os.MkdirAll(listsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(listsDir, "list-general.txt"), []byte("discord.com\n# skip\n\nyoutube.com\n"), 0644); err != nil {
		t.Fatal(err)
	}
	got := m.loadTestTargets()
	if len(got) != 2 {
		t.Fatalf("got %d targets, want 2", len(got))
	}
}

func TestSetStrategy_StrategyNotFound(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.SetStrategy("nope"); err == nil {
		t.Fatal("expected error for missing strategy file")
	}
}
