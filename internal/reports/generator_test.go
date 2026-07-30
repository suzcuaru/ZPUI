package reports

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"zpui/internal/database"
)

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		in   uint64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1 << 20, "1.0 MiB"},
		{(1 << 20) + (1 << 19), "1.5 MiB"},
		{1 << 30, "1.0 GiB"},
		{1 << 40, "1.0 TiB"},
		{1 << 50, "1.0 PiB"},
		{1 << 60, "1.0 EiB"},
	}
	for _, tc := range cases {
		got := formatBytes(tc.in)
		if got != tc.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestReportFilename(t *testing.T) {
	name := ReportFilename()
	re := regexp.MustCompile(`^ZPUI_Report_\d{4}-\d{2}-\d{2}_\d{6}\.md$`)
	if !re.MatchString(name) {
		t.Errorf("ReportFilename() = %q, does not match expected pattern", name)
	}
}

func TestSparkline(t *testing.T) {
	days := []database.ResourceDaily{
		{Date: "2024-01-03", Host: "h", Pct: 0},
		{Date: "2024-01-01", Host: "h", Pct: 100},
		{Date: "2024-01-02", Host: "h", Pct: 50},
	}
	out := sparkline(days)
	if len(out) != len([]rune(out)) {
		t.Errorf("sparkline returned non-rune-aligned string")
	}
	runes := []rune(out)
	if len(runes) != 3 {
		t.Fatalf("len = %d, want 3", len(runes))
	}
	if runes[0] != '█' {
		t.Errorf("first (earliest sorted date, pct=100) = %q, want '█'", string(runes[0]))
	}
	if runes[2] != '▁' {
		t.Errorf("last (pct=0) = %q, want '▁'", string(runes[2]))
	}

	clamped := sparkline([]database.ResourceDaily{{Date: "d", Pct: -5}})
	if []rune(clamped)[0] != '▁' {
		t.Errorf("negative pct not clamped: %q", clamped)
	}
	clampedHi := sparkline([]database.ResourceDaily{{Date: "d", Pct: 999}})
	if []rune(clampedHi)[0] != '█' {
		t.Errorf("over-100 pct not clamped: %q", clampedHi)
	}
}

func TestRenderMD_Empty(t *testing.T) {
	out := renderMD(ReportData{})
	if !strings.Contains(out, "# ZPUI Diagnostic Report") {
		t.Error("missing title")
	}
	if !strings.Contains(out, "No data for this period.") {
		t.Error("missing 'No data' for empty availability")
	}
	if !strings.Contains(out, "Windows (version unknown)") {
		t.Errorf("expected default OS text, got:\n%s", out)
	}
}

func TestRenderMD_Full(t *testing.T) {
	now := time.Now()
	applied := now.Add(-2 * time.Hour)

	avail := make([]database.AvailabilityRecord, 0, 35)
	for i := 0; i < 35; i++ {
		avail = append(avail, database.AvailabilityRecord{
			ID: "a", Timestamp: now.Add(-time.Duration(i) * time.Minute),
			Type: "all", TotalResources: 3, OKResources: 2, Pct: 66.6,
		})
	}

	data := ReportData{
		ReportDate:    "2024-01-01 12:00:00",
		ZPUIVersion:   "1.0.0",
		ZapretVersion: "0.9.9",
		Strategy:      "auto-strat",
		PeriodDays:    7,
		PeriodStart:   now.AddDate(0, 0, -7),
		PeriodEnd:     now,
		Availability:  avail,
		Devices: []database.SessionDevice{
			{Hostname: "phone", IP: "10.0.0.5", MAC: "AA:BB:CC:DD:EE:FF", TotalDL: 1 << 20, TotalUL: 1 << 19, IsOnline: true},
			{Hostname: "laptop", IP: "10.0.0.6", MAC: "11:22:33:44:55:66", IsOnline: false},
		},
		Snapshots: []database.TrafficSnapshot{
			{DLSpeed: 2 << 20, ULSpeed: 1 << 20, TotalDL: 1 << 23, TotalUL: 1 << 22, ConnCount: 5},
		},
		ActionLogs: buildActionLogs(55),
		SystemInfo: SystemInfo{
			OS: "Windows 11", CPUModel: "TestCPU", NumCores: 8, SystemRAMMB: 16384, AvailableRAM: 8000,
		},
		Operator:     &database.CurrentOperator{OperatorKey: "mts", OperatorName: "MTS"},
		OperatorInfo: &database.OperatorInfo{ISP: "MTS Inc", ASN: "AS1", City: "MSK", Org: "MTS Org"},
		ComponentVersions: []database.ComponentVersion{
			{ID: "core", InstalledVersion: "1.0.0", RemoteVersion: "1.0.1", RemoteSource: "github"},
		},
		Strategies: []database.OperatorStrategy{
			{Strategy: "s_active", IsActive: true, AvailabilityPct: 95.0, UseCount: 3, LastSource: "auto", AppliedAt: &applied},
			{Strategy: "s_inactive", IsActive: false, AvailabilityPct: -1, UseCount: 0, LastSource: "manual", AppliedAt: nil},
		},
		ResourceDaily: []database.ResourceDaily{
			{Date: "2024-01-01", Host: "discord.com", Pct: 80},
			{Date: "2024-01-02", Host: "discord.com", Pct: 90},
			{Date: "2024-01-01", Host: "youtube.com", Pct: 50},
		},
	}

	out := renderMD(data)

	checks := []string{
		"# ZPUI Diagnostic Report",
		"**ZPUI Version:** 1.0.0",
		"**Zapret Version:** 0.9.9",
		"**Strategy:** auto-strat",
		"## System Information",
		"**OS:** Windows 11",
		"**CPU:** TestCPU (8 cores)",
		"## Operator",
		"**Name:** MTS",
		"**ISP:** MTS Inc",
		"## Component Versions",
		"core",
		"## Strategies (2)",
		"s_active",
		"✓",
		"—",
		"## Resource Availability",
		"discord.com",
		"## Traffic Summary",
		"Peak download",
		"## Connected Devices",
		"phone",
		"online",
		"offline",
		"## Recent Actions (55)",
		"this-action-is-way-too-long-and-needs-truncation-now",
	}
	for _, c := range checks {
		if !strings.Contains(out, c) {
			t.Errorf("renderMD output missing %q", c)
		}
	}
}

func buildActionLogs(n int) []database.ActionLog {
	out := make([]database.ActionLog, n)
	for i := range out {
		if i%2 == 0 {
			out[i] = database.ActionLog{
				Timestamp: time.Now(),
				Category:  "cat",
				Action:    "this-action-is-way-too-long-and-needs-truncation-now",
				Details:   "also-very-long-details-that-should-be-truncated-here-yes",
			}
		} else {
			out[i] = database.ActionLog{Timestamp: time.Now(), Category: "cat", Action: "short", Details: "d"}
		}
	}
	return out
}

func TestGenerate_WithTempDB(t *testing.T) {
	dir := t.TempDir()
	if err := database.Init(filepath.Join(dir, "rep.db")); err != nil {
		t.Fatalf("Init: %v", err)
	}

	zapretDir := filepath.Join(dir, "zapret")
	if err := os.MkdirAll(zapretDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(zapretDir, "version.txt"), []byte("  9.9.9  \n"), 0644); err != nil {
		t.Fatal(err)
	}

	g := NewGenerator("3.3.3", zapretDir, "my-strategy")

	t.Run("default period when <=0", func(t *testing.T) {
		out, err := g.Generate(0)
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if !strings.Contains(out, "3.3.3") {
			t.Error("missing ZPUI version 3.3.3")
		}
		if !strings.Contains(out, "9.9.9") {
			t.Error("missing zapret version (trimmed) 9.9.9")
		}
		if !strings.Contains(out, "my-strategy") {
			t.Error("missing strategy")
		}
	})

	t.Run("explicit period", func(t *testing.T) {
		out, err := g.Generate(14)
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if !strings.Contains(out, "Diagnostic Report") {
			t.Error("missing title")
		}
	})
}

func TestCollectSystemInfo(t *testing.T) {
	si := collectSystemInfo()
	if si.NumCores <= 0 {
		t.Errorf("NumCores = %d", si.NumCores)
	}
	if si.OS == "" {
		t.Error("OS should be populated (or default)")
	}
	if si.CPUModel == "" {
		t.Error("CPUModel should be populated (or default)")
	}
}

func TestSaveToFile(t *testing.T) {
	content := "# test report\nhello"
	name := "ZPUI_Test_" + time.Now().Format("20060102_150405_999") + ".md"
	path, err := SaveToFile(content, name)
	if err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != content {
		t.Errorf("content mismatch: got %q", string(data))
	}
	if !strings.HasSuffix(path, name) {
		t.Errorf("path = %q, expected suffix %q", path, name)
	}
}
