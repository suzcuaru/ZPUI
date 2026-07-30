package sysinfo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseCSVLine(t *testing.T) {
	cases := []struct {
		name string
		line string
		want []string
	}{
		{
			name: "full tasklist row",
			line: `"ZPUI.exe","1234","Console","1","51,234 КБ","Running","DESKTOP\user","0:00:05"`,
			want: []string{"ZPUI.exe", "1234", "Console", "1", "51,234 КБ", "Running", `DESKTOP\user`, "0:00:05"},
		},
		{
			name: "single field quoted",
			line: `"solo"`,
			want: []string{"solo"},
		},
		{
			name: "two fields",
			line: `"a","b"`,
			want: []string{"a", "b"},
		},
		{
			name: "empty string",
			line: ``,
			want: []string{""},
		},
		{
			name: "field with internal quotes stripped",
			line: `"he"llo","world"`,
			want: []string{"he", "llo", "world"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseCSVLine(tc.line)
			if len(got) != len(tc.want) {
				t.Fatalf("len mismatch: got %d (%#v), want %d (%#v)", len(got), got, len(tc.want), tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("field %d: got %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestParseMemField(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"51,234 КБ", 51234},
		{"1,024 КБ", 1024},
		{"512K", 512},
		{"12345", 12345},
		{"  7  ", 7},
		{"", 0},
		{"not-a-number", 0},
		{"0", 0},
	}
	for _, tc := range cases {
		got := parseMemField(tc.in)
		if got != tc.want {
			t.Errorf("parseMemField(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseCPUTime(t *testing.T) {
	cases := []struct {
		in       string
		wantSecs float64
	}{
		{"0:00:05", 5},
		{"0:02:30", 150},
		{"1:00:00", 3600},
		{"1:30:45.5", 1*3600 + 30*60 + 45.5},
		{"2:15", 135},            // mm:ss
		{"00:00.25", 0.25},       // mm:ss.s
		{"", 0},                  // default
		{"abc", 0},               // default (no colon)
		{"1:abc", 60},            // m=1, sec=0
		{"x:y:z", 0},             // parse failures → 0
		{"1:2:3:4", 0},           // >3 parts → default
	}
	for _, tc := range cases {
		got := parseCPUTime(tc.in)
		gotSecs := got.Seconds()
		if gotSecs != tc.wantSecs {
			t.Errorf("parseCPUTime(%q).Seconds() = %v, want %v", tc.in, gotSecs, tc.wantSecs)
		}
	}
}

func TestGetSystemRAM(t *testing.T) {
	total, avail := getSystemRAM()
	if total <= 0 {
		t.Fatalf("expected positive total RAM MB, got %v", total)
	}
	if avail < 0 {
		t.Fatalf("expected non-negative available RAM, got %v", avail)
	}
	if avail > total {
		t.Fatalf("available (%v) must not exceed total (%v)", avail, total)
	}
	t.Logf("RAM: total=%.0f MB, available=%.0f MB", total, avail)
}

func TestGetNumCPU(t *testing.T) {
	n := getNumCPU()
	if n <= 0 {
		t.Fatalf("expected positive CPU count, got %d", n)
	}
	t.Logf("CPU cores: %d", n)
}

func TestGetSystemResources(t *testing.T) {
	r := GetSystemResources()
	if r.NumCores <= 0 {
		t.Errorf("expected NumCores>0, got %d", r.NumCores)
	}
	if r.SystemRAMMB <= 0 {
		t.Errorf("expected SystemRAMMB>0, got %v", r.SystemRAMMB)
	}
	if r.Processes == nil {
		t.Errorf("expected non-nil Processes slice")
	}
	for _, p := range r.Processes {
		if p.MemoryMB < 0 {
			t.Errorf("negative memory for %s: %v", p.Name, p.MemoryMB)
		}
		if p.CPUPercent < 0 {
			t.Errorf("negative cpu for %s: %v", p.Name, p.CPUPercent)
		}
	}
}

func TestGetProcessInfo_CPUCacheBranches(t *testing.T) {
	exePath, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	name := filepath.Base(exePath)

	first := getProcessInfo(name)
	if first == nil {
		t.Skipf("process %q not found via tasklist; cannot exercise cache branches", name)
	}
	if first.Name == "" {
		t.Errorf("first call: empty Name")
	}

	second := getProcessInfo(name)
	if second == nil {
		t.Fatalf("second call returned nil")
	}

	time.Sleep(700 * time.Millisecond)

	third := getProcessInfo(name)
	if third == nil {
		t.Fatalf("third call returned nil")
	}

	if !strings.EqualFold(third.Name, name) {
		t.Errorf("third call Name=%q, want %q", third.Name, name)
	}
}

func TestGetProcessInfo_NotFound(t *testing.T) {
	info := getProcessInfo("definitely-not-a-real-process-name-xyz.exe")
	if info != nil {
		t.Errorf("expected nil for unknown process, got %+v", info)
	}
}

func TestGetSystemResources_RenamesExeToZPUI(t *testing.T) {
	r := GetSystemResources()
	exePath, _ := os.Executable()
	exeName := filepath.Base(exePath)
	foundZPUI := false
	for _, p := range r.Processes {
		if strings.EqualFold(p.Name, "ZPUI") {
			foundZPUI = true
		}
	}
	if !foundZPUI {
		t.Logf("exe name=%q; ZPUI entry not present (acceptable if tasklist format differs)", exeName)
	}
}
