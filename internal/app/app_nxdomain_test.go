package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zpui/internal/blockcheck"
	"zpui/internal/config"
	"zpui/internal/logger"
)

func newNXTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), filepath.Join(dir, "zapret"))
	// Авто-исключение в конфиге по умолчанию выключено; большинство тестов
	// проверяет саму механику — включаем. Отдельный тест следит за дефолтом.
	cfg.AutoSkipDeadDomains = true
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

func nxResult(host string) blockcheck.BulkResult {
	return blockcheck.BulkResult{
		Name:        host,
		URL:         "https://" + host + "/",
		Host:        host,
		Verdict:     blockcheck.VerdictDNSBlock,
		DNSNotFound: true,
	}
}

func resolvedResult(host string) blockcheck.BulkResult {
	return blockcheck.BulkResult{
		Name:    host,
		URL:     "https://" + host + "/",
		Host:    host,
		OK:      true,
		Verdict: blockcheck.VerdictOK,
	}
}

func TestTrackNXDomainsAutoExclude(t *testing.T) {
	a := newNXTestApp(t)

	alive := resolvedResult("alive.example")
	nx := nxResult("dead.example")

	// Два подряд — рано, файла исключений ещё нет.
	a.trackNXDomains(&blockcheck.BulkReport{Default: []blockcheck.BulkResult{nx, alive}})
	a.trackNXDomains(&blockcheck.BulkReport{Default: []blockcheck.BulkResult{nx, alive}})
	if got := a.cfg.GetSkipResources(); len(got) != 0 {
		t.Fatalf("host excluded before threshold: %v", got)
	}

	// Третий подряд — хост в skip-resources.txt.
	a.trackNXDomains(&blockcheck.BulkReport{Default: []blockcheck.BulkResult{nx, alive}})
	got := a.cfg.GetSkipResources()
	if len(got) != 1 || got[0] != "dead.example" {
		t.Fatalf("skip resources = %v, want [dead.example]", got)
	}

	// В пользовательском списке исключений мёртвым доменам не место.
	if got := a.userExclusions(); len(got) != 0 {
		t.Fatalf("user exclusions touched: %v", got)
	}

	// Файл плоский: только домены, без секций и приписок.
	data, err := os.ReadFile(a.cfg.GetSkipResourcesFilePath())
	if err != nil {
		t.Fatalf("read skip-resources file: %v", err)
	}
	content := string(data)
	if strings.Contains(content, config.SkipAutoSection) || strings.Contains(content, config.SkipManualSection) {
		t.Errorf("section headers must not be written anymore:\n%s", content)
	}
	if strings.Contains(content, "# добавлено") || strings.Contains(content, "# auto:") {
		t.Errorf("annotation comments must not be written anymore:\n%s", content)
	}
	if !strings.Contains(content, "\ndead.example\n") && !strings.HasSuffix(content, "dead.example\n") {
		t.Errorf("host line not found in file:\n%s", content)
	}

	// Уже исключённый хост не дублируется; IP-литерал не исключается никогда.
	a.trackNXDomains(&blockcheck.BulkReport{Default: []blockcheck.BulkResult{nx, nxResult("192.0.2.10"), alive}})
	if got := a.cfg.GetSkipResources(); len(got) != 1 {
		t.Fatalf("skip resources after re-check = %v, want single entry", got)
	}
}

func TestTrackNXDomainsStreakResetOnResolve(t *testing.T) {
	a := newNXTestApp(t)

	alive := resolvedResult("alive.example")
	nx := nxResult("flaky.example")

	a.trackNXDomains(&blockcheck.BulkReport{Default: []blockcheck.BulkResult{nx, alive}})
	a.trackNXDomains(&blockcheck.BulkReport{Default: []blockcheck.BulkResult{nx, alive}})

	// Хост снова резолвится — счётчик сбрасывается.
	a.trackNXDomains(&blockcheck.BulkReport{Default: []blockcheck.BulkResult{resolvedResult("flaky.example"), alive}})

	a.trackNXDomains(&blockcheck.BulkReport{Default: []blockcheck.BulkResult{nx, alive}})
	a.trackNXDomains(&blockcheck.BulkReport{Default: []blockcheck.BulkResult{nx, alive}})
	if got := a.cfg.GetSkipResources(); len(got) != 0 {
		t.Fatalf("streak not reset after resolve: excluded %v", got)
	}

	// Третий сэмпл после сброса — порог достигнут.
	a.trackNXDomains(&blockcheck.BulkReport{Default: []blockcheck.BulkResult{nx, alive}})
	got := a.cfg.GetSkipResources()
	if len(got) != 1 || got[0] != "flaky.example" {
		t.Fatalf("skip resources = %v, want [flaky.example]", got)
	}
}

func TestTrackNXDomainsDisabledByDefault(t *testing.T) {
	a := newNXTestApp(t)
	a.cfg.AutoSkipDeadDomains = false

	// Функция выключена (дефолт): даже устойчивый NXDOMAIN с живой сетью
	// не попадает в skip-resources.txt.
	for i := 0; i < 5; i++ {
		a.trackNXDomains(&blockcheck.BulkReport{Default: []blockcheck.BulkResult{
			nxResult("dead.example"),
			resolvedResult("alive.example"),
		}})
	}
	if got := a.cfg.GetSkipResources(); len(got) != 0 {
		t.Fatalf("auto-exclude ran while disabled: %v", got)
	}

	// Включение начинает отсчёт с нуля: первые два такта после включения
	// ещё не добавляют (порог 3 подряд).
	a.cfg.AutoSkipDeadDomains = true
	a.trackNXDomains(&blockcheck.BulkReport{Default: []blockcheck.BulkResult{
		nxResult("dead.example"),
		resolvedResult("alive.example"),
	}})
	if got := a.cfg.GetSkipResources(); len(got) != 0 {
		t.Fatalf("stale streak applied after enable: %v", got)
	}
}

func TestTrackNXDomainsNetworkDeadGuard(t *testing.T) {
	a := newNXTestApp(t)

	// Ни один ресурс в прогоне не резолвится — сеть не подтверждена,
	// авто-добавление запрещено даже после многих прогонов.
	for i := 0; i < 5; i++ {
		a.trackNXDomains(&blockcheck.BulkReport{Default: []blockcheck.BulkResult{
			nxResult("dead1.example"),
			nxResult("dead2.example"),
		}})
	}
	if got := a.cfg.GetSkipResources(); len(got) != 0 {
		t.Fatalf("auto-excluded while network unconfirmed: %v", got)
	}

	// Другой сбой DNS (не NXDOMAIN, флаг не выставлен) тоже не даёт добавлять:
	// счётчик не растёт и живых хостов нет.
	dnsFail := nxResult("dead3.example")
	dnsFail.DNSNotFound = false
	for i := 0; i < 5; i++ {
		a.trackNXDomains(&blockcheck.BulkReport{Default: []blockcheck.BulkResult{dnsFail}})
	}
	if got := a.cfg.GetSkipResources(); len(got) != 0 {
		t.Fatalf("auto-excluded on non-NXDOMAIN dns failure: %v", got)
	}
}

func TestMigrateLegacyServerExclusions(t *testing.T) {
	a := newNXTestApp(t)
	legacy := filepath.Join(filepath.Dir(a.cfg.GetSkipResourcesFilePath()), "exclusions-server.txt")
	legacyBody := "# старый конвейер v4\nold-dead.example\nsub.alive.example\n"
	if err := os.WriteFile(legacy, []byte(legacyBody), 0o644); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}

	a.migrateLegacyServerExclusions()

	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy exclusions-server.txt still present")
	}
	if _, err := os.Stat(legacy + ".migrated"); err != nil {
		t.Fatalf("exclusions-server.txt.migrated missing: %v", err)
	}
	got := a.cfg.GetSkipResources()
	if len(got) != 2 || got[0] != "old-dead.example" || got[1] != "sub.alive.example" {
		t.Fatalf("skip resources after migration = %v", got)
	}
	data, err := os.ReadFile(a.cfg.GetSkipResourcesFilePath())
	if err != nil {
		t.Fatalf("read skip-resources file: %v", err)
	}
	// Плоский формат: перенесённые домены лежат строками, без секций.
	if strings.Contains(string(data), config.SkipManualSection) {
		t.Errorf("section headers must not be written anymore:\n%s", data)
	}
	if got := a.userExclusions(); len(got) != 0 {
		t.Errorf("user exclusions touched by migration: %v", got)
	}

	// Повторный запуск безопасен: файла уже нет, дублей не появляется.
	a.migrateLegacyServerExclusions()
	if got := a.cfg.GetSkipResources(); len(got) != 2 {
		t.Fatalf("duplicates after re-migration: %v", got)
	}
}

func TestExcludedHostLists(t *testing.T) {
	a := newNXTestApp(t)
	a.ensureExclusionFiles()

	if err := a.writeSystemExclusions([]string{"system-dead.example"}); err != nil {
		t.Fatalf("write system exclusions: %v", err)
	}
	if err := a.addUserExclusion("user-dead.example"); err != nil {
		t.Fatalf("add user exclusion: %v", err)
	}

	for _, host := range []string{
		"system-dead.example",
		"sub.system-dead.example",
		"user-dead.example",
		"sub.user-dead.example",
	} {
		if !a.excludedHost(host) {
			t.Errorf("excludedHost(%q) = false, want true", host)
		}
	}
	if a.excludedHost("alive.example") {
		t.Errorf("excludedHost(alive.example) = true, want false")
	}

	// Дедупликация: повторное добавление того же домена не создаёт дубль.
	if err := a.addUserExclusion("user-dead.example"); err != nil {
		t.Fatalf("re-add: %v", err)
	}
	if got := a.userExclusions(); len(got) != 1 {
		t.Fatalf("exclusions after re-add = %v, want single entry", got)
	}

	// Пользовательский файл не перезаписывается ensure'ом.
	a.ensureExclusionFiles()
	if got := a.userExclusions(); len(got) != 1 {
		t.Fatalf("ensure touched user list: %v", got)
	}
}

func TestParseStrategyInjectsSystemExclude(t *testing.T) {
	a := newNXTestApp(t)
	listsDir := a.cfg.ListsDir()
	if err := os.MkdirAll(listsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(a.systemExcludePath(), []byte("# sys\ndead.example\n"), 0o644); err != nil {
		t.Fatalf("write system list: %v", err)
	}
	if got := listDomains(a.systemExcludePath()); len(got) != 1 || got[0] != "dead.example" {
		t.Fatalf("system list parse = %v", got)
	}
}
