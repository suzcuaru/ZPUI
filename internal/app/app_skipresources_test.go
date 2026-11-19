package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zpui/internal/config"
	"zpui/internal/logger"
)

func newSkipTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), filepath.Join(dir, "zapret"))
	// logdb держит файл открытым дольше жизни теста — отдельный каталог вне
	// t.TempDir(), чтобы автоочистка не падала на занятом дескрипторе.
	logDir, err := os.MkdirTemp("", "zpui-test-logdb")
	if err != nil {
		t.Fatalf("logdb dir: %v", err)
	}
	logMgr, err := logger.New(dir, logDir)
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	t.Cleanup(func() { logMgr.Close() })
	return &App{cfg: cfg, log: logMgr}
}

// Синк исключений с сервера (state.sync) пишет системный набор в
// skip-resources-system.txt, а не в DPI-базу list-exclude-system.txt.
func TestWriteSystemExclusionsTargetsSkipSystemFile(t *testing.T) {
	a := newSkipTestApp(t)

	if err := a.writeSystemExclusions([]string{"zed.example", "alpha.example", "# junk", "  "}); err != nil {
		t.Fatalf("writeSystemExclusions: %v", err)
	}

	data, err := os.ReadFile(a.cfg.GetSkipResourcesSystemFilePath())
	if err != nil {
		t.Fatalf("system skip file not written: %v", err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "# skip-resources-system.txt") {
		t.Errorf("header missing:\n%s", text)
	}
	if !strings.Contains(text, "alpha.example") || !strings.Contains(text, "zed.example") {
		t.Errorf("domains missing from system skip file:\n%s", text)
	}
	if strings.Contains(text, "junk") {
		t.Errorf("comment entry leaked into domain list:\n%s", text)
	}
	if i, j := strings.Index(text, "alpha.example"), strings.Index(text, "zed.example"); j < i {
		t.Errorf("domains not sorted:\n%s", text)
	}

	if _, err := os.Stat(filepath.Join(a.cfg.ListsDir(), "list-exclude-system.txt")); !os.IsNotExist(err) {
		t.Errorf("list-exclude-system.txt must not be touched, stat err = %v", err)
	}
}

func TestEnsureSkipResourcesSystemFile(t *testing.T) {
	a := newSkipTestApp(t)
	path := a.cfg.GetSkipResourcesSystemFilePath()

	a.ensureSkipResourcesSystemFile()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ensure did not create file: %v", err)
	}
	if !strings.HasPrefix(string(data), "# skip-resources-system.txt") {
		t.Errorf("unexpected default content:\n%s", string(data))
	}

	if err := os.WriteFile(path, []byte("custom.example\n"), 0644); err != nil {
		t.Fatal(err)
	}
	a.ensureSkipResourcesSystemFile()
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "custom.example\n" {
		t.Errorf("ensure overwrote existing file: %q", string(data))
	}
}

// GetSkipResources биндинг отдаёт read-only блок system для UI.
func TestGetSkipResourcesBindingSystemBlock(t *testing.T) {
	a := newSkipTestApp(t)
	a.ensureSkipResourcesSystemFile()

	resp := a.GetSkipResources()
	sys, ok := resp["system"].(map[string]interface{})
	if !ok {
		t.Fatalf("system block missing from response: %v", resp)
	}
	lines, ok := sys["lines"].([]string)
	if !ok {
		t.Fatalf("system.lines missing: %v", sys)
	}
	if count, _ := sys["count"].(int); count != len(lines) {
		t.Errorf("system.count = %v, lines = %v", sys["count"], lines)
	}
	if _, ok := resp["lines"].([]string); !ok {
		t.Errorf("user lines missing from response: %v", resp)
	}
}

// skip_user в секции data.exclusions (ZPUI Sync) — это skip-resources.txt
// целиком (с комментариями-причинами), а не DPI-хостлисты.
func TestSyncExclusionsSkipUserFromSkipResources(t *testing.T) {
	a := newSkipTestApp(t)

	if err := os.WriteFile(a.cfg.GetSkipResourcesFilePath(), []byte("# auto: исключён автоматически\nuser-skip.example\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(a.cfg.ListsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.cfg.ListsDir(), "list-exclude-user.txt"), []byte("dpi-user.example\n"), 0644); err != nil {
		t.Fatal(err)
	}

	sec := a.sectionExclusions()
	skipUser, _ := sec["skip_user"].(string)
	if !strings.Contains(skipUser, "user-skip.example") || strings.Contains(skipUser, "dpi-user.example") {
		t.Fatalf("skip_user = %q, want only user-skip.example", skipUser)
	}
}
