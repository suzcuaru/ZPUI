package zapret

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitArgs(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"single", "abc", []string{"abc"}},
		{"simple", "a b c", []string{"a", "b", "c"}},
		{"multi-spaces", "  a   b  ", []string{"a", "b"}},
		{"tabs", "a\tb\tc", []string{"a", "b", "c"}},
		{"quoted-keeps-quotes", `"a b" c`, []string{`"a b"`, "c"}},
		{"quote-toggle", `a"b c"d`, []string{`a"b c"d`}},
		{"only-quotes", `""`, []string{`""`}},
		{"trailing-space", "abc ", []string{"abc"}},
		{"leading-tab", "\tx y", []string{"x", "y"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := splitArgs(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("splitArgs(%q) = %v, want %v", c.in, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("splitArgs(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
				}
			}
		})
	}
}

func writeStrategyFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestParseStrategyArgs_Simple(t *testing.T) {
	_, cfg, _ := newTestManager(t)
	strategyPath := cfg.StrategyPath("general")
	writeStrategyFile(t, strategyPath, "@echo off\nwinws.exe --filter-tcp=443 --dpi-desync=fake\n")

	got, err := parseStrategyArgs(strategyPath, cfg.BinDir(), cfg.ListsDir(), "12", "12")
	if err != nil {
		t.Fatal(err)
	}
	want := "--filter-tcp 443 --dpi-desync=fake"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseStrategyArgs_Continuation(t *testing.T) {
	m, cfg, _ := newTestManager(t)
	_ = m
	strategyPath := cfg.StrategyPath("general")
	content := "@echo off\nwinws.exe --filter-tcp=443 ^\n--dpi-desync=fake ^\n--dup-autofreq\n"
	writeStrategyFile(t, strategyPath, content)

	got, err := parseStrategyArgs(strategyPath, cfg.BinDir(), cfg.ListsDir(), "12", "12")
	if err != nil {
		t.Fatal(err)
	}
	want := "--filter-tcp 443 --dpi-desync=fake --dup-autofreq"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseStrategyArgs_EqSplitOnlyForDashDash(t *testing.T) {
	m, cfg, _ := newTestManager(t)
	_ = m
	strategyPath := cfg.StrategyPath("general")
	writeStrategyFile(t, strategyPath, "winws.exe --filter-tcp=443 ipset=foo\n")

	got, err := parseStrategyArgs(strategyPath, cfg.BinDir(), cfg.ListsDir(), "12", "12")
	if err != nil {
		t.Fatal(err)
	}
	// "ipset=foo" не начинается с "--", поэтому не разбивается.
	if !strings.Contains(got, "ipset=foo") {
		t.Errorf("expected ipset=foo preserved, got %q", got)
	}
	if !strings.Contains(got, "--filter-tcp 443") {
		t.Errorf("expected --filter-tcp 443 split, got %q", got)
	}
}

func TestParseStrategyArgs_Macros(t *testing.T) {
	m, cfg, _ := newTestManager(t)
	_ = m
	strategyPath := cfg.StrategyPath("general")
	content := "winws.exe --ipset=%LISTS%\\general.txt --gf=%GameFilter% --bin=%BIN%\n"
	writeStrategyFile(t, strategyPath, content)

	got, err := parseStrategyArgs(strategyPath, cfg.BinDir(), cfg.ListsDir(), "1024-65535", "12")
	if err != nil {
		t.Fatal(err)
	}
	// %LISTS% → lists dir (с прямыми слешами, затем конвертировано в обратные)
	expectedLists := strings.ReplaceAll(filepath.ToSlash(cfg.ListsDir())+"/", "/", `\`)
	if !strings.Contains(got, expectedLists+`general.txt`) {
		t.Errorf("expected %q in %q", expectedLists+`general.txt`, got)
	}
	// %GameFilter% заменяется на gfTCP.
	if !strings.Contains(got, "--gf 1024-65535") {
		t.Errorf("expected --gf 1024-65535 in %q", got)
	}
	// %BIN% заменяется.
	expectedBin := strings.ReplaceAll(filepath.ToSlash(cfg.BinDir())+"/", "/", `\`)
	if !strings.Contains(got, "--bin "+expectedBin) {
		t.Errorf("expected --bin %q in %q", expectedBin, got)
	}
}

func TestParseStrategyArgs_Dp0Prefix(t *testing.T) {
	m, cfg, _ := newTestManager(t)
	_ = m
	strategyPath := cfg.StrategyPath("general")
	content := `winws.exe --ipset="%~dp0lists\general.txt"` + "\n"
	writeStrategyFile(t, strategyPath, content)

	got, err := parseStrategyArgs(strategyPath, cfg.BinDir(), cfg.ListsDir(), "12", "12")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `lists\general.txt`) {
		t.Errorf("expected lists\\general.txt in %q", got)
	}
	if !strings.HasPrefix(got, "--ipset ") {
		t.Errorf("expected --ipset split, got %q", got)
	}
}

func TestParseStrategyArgs_BreakConditions(t *testing.T) {
	m, cfg, _ := newTestManager(t)
	_ = m
	strategyPath := cfg.StrategyPath("general")
	content := "winws.exe --filter-tcp=443\nset FOO=bar\n--should-not-appear\n"
	writeStrategyFile(t, strategyPath, content)

	got, err := parseStrategyArgs(strategyPath, cfg.BinDir(), cfg.ListsDir(), "12", "12")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "should-not-appear") {
		t.Errorf("break after 'set ' failed: %q", got)
	}
	if strings.Contains(got, "FOO") {
		t.Errorf("break after 'set ' failed (FOO leaked): %q", got)
	}
	if !strings.Contains(got, "--filter-tcp 443") {
		t.Errorf("expected --filter-tcp 443 in %q", got)
	}
}

func TestParseStrategyArgs_TabCollapsedToSpace(t *testing.T) {
	m, cfg, _ := newTestManager(t)
	_ = m
	strategyPath := cfg.StrategyPath("general")
	writeStrategyFile(t, strategyPath, "winws.exe --filter-tcp=443\t--dpi-desync=fake\n")

	got, err := parseStrategyArgs(strategyPath, cfg.BinDir(), cfg.ListsDir(), "12", "12")
	if err != nil {
		t.Fatal(err)
	}
	want := "--filter-tcp 443 --dpi-desync=fake"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseStrategyArgs_NoWinws(t *testing.T) {
	m, cfg, _ := newTestManager(t)
	_ = m
	strategyPath := cfg.StrategyPath("general")
	writeStrategyFile(t, strategyPath, "@echo off\necho hello\n")

	_, err := parseStrategyArgs(strategyPath, cfg.BinDir(), cfg.ListsDir(), "12", "12")
	if err == nil {
		t.Fatal("expected error when no winws.exe line")
	}
}

func TestParseStrategyArgs_FileNotFound(t *testing.T) {
	m, cfg, _ := newTestManager(t)
	_ = m
	_, err := parseStrategyArgs(filepath.Join(cfg.GetZapretPath(), "missing.bat"), cfg.BinDir(), cfg.ListsDir(), "12", "12")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestStartWinws_StrategyNotFound(t *testing.T) {
	m, _, _ := newTestManager(t)
	_, _, err := m.startWinws("does-not-exist")
	if err == nil {
		t.Fatal("expected error for missing strategy")
	}
}

func TestStartWinws_StrategyUnparseable(t *testing.T) {
	m, cfg, _ := newTestManager(t)
	writeStrategyFile(t, cfg.StrategyPath("general"), "@echo off\necho no winws here\n")
	_, _, err := m.startWinws("general")
	if err == nil {
		t.Fatal("expected parse error for strategy without winws")
	}
}

func TestStartWinws_WinwsNotFound(t *testing.T) {
	m, cfg, _ := newTestManager(t)
	writeStrategyFile(t, cfg.StrategyPath("general"), "winws.exe --filter-tcp=443\n")
	_, _, err := m.startWinws("general")
	if err == nil {
		t.Fatal("expected winws.exe not found error")
	}
}
