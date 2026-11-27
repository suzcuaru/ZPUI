package pkginst

// Общая логика установщика/обновлятора/деинсталлятора ZPUI (порт
// installer/ZPUI.nsi и ZPUI-Update.nsi на Go). Пакет ничего не знает об UI —
// вызывающие Wails-приложения отрисовывают прогресс через колбэк Step.

import (
	"archive/zip"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"zpui/internal/extension"
	"zpui/internal/updater"
	"zpui/internal/zapret"
	"zpui/internal/zsvc"
)

// Step сообщает прогресс: фаза, доля 0..1, детальная строка (может быть пустой).
type Step func(phase string, value float64, detail string)

// Payload — открытый zip-пакет с деревьями x86/ и x64/ (+ README.md,
// uninstall.exe). Размер нужен для zip.NewReader.
type Payload struct {
	Reader io.ReaderAt
	Size   int64
}

// protectedUserRel — пользовательские файлы, снимаемые в temp перед накатом
// и восстанавливаемые после: пользовательские списки zapret, системный список
// исключений, скип-лист мониторинга и config.json. Без этого обновление/чистая
// установка затирали бы их содержимым из нового пакета (или удаляли вместе
// с каталогом).
var protectedUserRel = []string{
	"config.json",
	"skip-resources.txt",
	"skip-resources-system.txt",
	"domains.txt",
	"dns-system.json",
	"dns-user.json",
	"zapret/lists/list-general-user.txt",
	"zapret/lists/list-exclude-user.txt",
	"zapret/lists/ipset-exclude-user.txt",
	"zapret/lists/list-exclude-system.txt",
}

// UserFilesBackup — снимок пользовательских файлов в temp (каталог удаляется
// при Restore).
type UserFilesBackup struct {
	dir string
}

// BackupUserFiles копирует существующие пользовательские файлы установки в
// временный каталог. nil —(temp не создался): восстановление тогда no-op.
func BackupUserFiles(dir string) *UserFilesBackup {
	tmp, err := os.MkdirTemp("", "zpui-user-")
	if err != nil {
		return nil
	}
	b := &UserFilesBackup{dir: tmp}
	for _, rel := range protectedUserRel {
		src := filepath.Join(dir, filepath.FromSlash(rel))
		data, err := os.ReadFile(src)
		if err != nil || len(data) == 0 {
			continue
		}
		dst := filepath.Join(tmp, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			continue
		}
		_ = os.WriteFile(dst, data, 0o644)
	}
	return b
}

// Restore возвращает пользовательские файлы поверх свежераспакованных:
// если файл появился и в новом дистрибутиве — его затирает восстановленное
// пользовательское содержимое.
func (b *UserFilesBackup) Restore(dir string) {
	if b == nil || b.dir == "" {
		return
	}
	_ = filepath.Walk(b.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(b.dir, path)
		if rerr != nil {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil || len(data) == 0 {
			return nil
		}
		dst := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil
		}
		_ = os.WriteFile(dst, data, 0o644)
		return nil
	})
	_ = os.RemoveAll(b.dir)
	b.dir = ""
}

func isUserPath(rel string) bool {
	switch rel {
	case "config.json", "zpui.db", ".last_update", "gh_release_cache.json",
		// Пользовательские списки рядом с config.json: дефолтные экземпляры
		// есть в payload, поэтому без защиты обновление затирало их.
		"skip-resources.txt", "skip-resources-system.txt", "domains.txt", "exclusions-server.txt",
		// DNS 2.0: системный список ведёт сервер, пользовательский - юзер.
		"dns-system.json", "dns-user.json",
		// Пользовательские списки zapret: не распаковываются вовсе (плейсхолдеры
		// пакета не должны даже на миг затирать содержимое — возвращает их
		// Restore, но не распаковывать надёжнее).
		"zapret/lists/list-general-user.txt", "zapret/lists/list-exclude-user.txt",
		"zapret/lists/ipset-exclude-user.txt", "zapret/lists/list-exclude-system.txt":
		return true
	}
	for _, d := range []string{"logs", ".backup", "databases"} {
		if rel == d || strings.HasPrefix(rel, d+"/") {
			return true
		}
	}
	for _, suf := range []string{".db-journal", ".db-wal", ".db-shm", ".zip", ".old", ".log"} {
		if strings.HasSuffix(rel, suf) {
			return true
		}
	}
	return false
}

// KillApps гасит приложение, движок и мост Telegram (без вопросов, как NSI-обновлятор).
func KillApps() {
	for _, img := range []string{"ZPUI.exe", "winws.exe", "ZPUI-TgProxy.exe"} {
		cmd := exec.Command("taskkill", "/IM", img, "/F")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		_ = cmd.Run()
	}
}

// isLocked проверяет файл монопольным открытием — занят ли он процессом.
func isLocked(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	p16, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	h, err := windows.CreateFile(p16, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return true
	}
	_ = windows.CloseHandle(h)
	return false
}

// WaitUnlocked ждёт, пока путь перестанет быть занят (лимит — как в NSI).
func WaitUnlocked(path string, tries int) error {
	for i := 0; i < tries; i++ {
		time.Sleep(500 * time.Millisecond)
		if !isLocked(path) {
			return nil
		}
	}
	return fmt.Errorf("не удалось закрыть ZPUI — файл %s всё ещё занят", path)
}

func extractOne(f *zip.File, dst string) error {
	if err := extractOnce(f, dst); err == nil {
		return nil
	}
	var lastErr error
	for i := 0; i < 3; i++ {
		time.Sleep(250 * time.Millisecond)
		if lastErr = extractOnce(f, dst); lastErr == nil {
			return nil
		}
	}
	return lastErr
}

func extractOnce(f *zip.File, dst string) error {
	// Запись каталога («dir/», size 0) должна создавать каталог, а не нулевой
	// файл: иначе дочерние записи падают в MkdirAll («предок — файл»).
	// Суффикс проверяем по ToSlash-имени: payload может хранить записи с
	// бэкслэшами («x64\skills\») — тогда ни IsDir(), ни "/"-суффикс не сработают.
	if f.FileInfo().IsDir() || strings.HasSuffix(filepath.ToSlash(f.Name), "/") {
		return os.MkdirAll(dst, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	src, err := f.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, src)
	return err
}

// payloadTree — список файлов нужной битности из пакета.
func payloadTree(p *Payload, archPrefix string) (main, wintun, service, tgproxy, zapret, root []*zip.File, err error) {
	r, err := zip.NewReader(p.Reader, p.Size)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("битый пакет: %w", err)
	}
	for _, f := range r.File {
		name := filepath.ToSlash(filepath.Clean(f.Name))
		if !f.FileInfo().IsDir() && !strings.Contains(name, "/") {
			root = append(root, f)
			continue
		}
		parts := strings.SplitN(name, "/", 2)
		if len(parts) != 2 || parts[0] != strings.TrimSuffix(archPrefix, "/") {
			continue
		}
		rel := parts[1]
		switch {
		case rel == "components" || strings.HasPrefix(rel, "components/"):
			// wintun — драйвер; service — exe службы ZPUI; tgproxy — мост
			// Telegram (остальные components/ легаси и не распаковываются).
			if strings.HasPrefix(rel, "components/wintun/") && !f.FileInfo().IsDir() {
				wintun = append(wintun, f)
			}
			if strings.HasPrefix(rel, "components/service/") && !f.FileInfo().IsDir() {
				service = append(service, f)
			}
			if strings.HasPrefix(rel, "components/tgproxy/") && !f.FileInfo().IsDir() {
				tgproxy = append(tgproxy, f)
			}
			// extension — пакет браузерного плагина и его установщик: те же
			// обычные файлы компонента, что и tgproxy (распаковка best-effort).
			if strings.HasPrefix(rel, "components/extension/") && !f.FileInfo().IsDir() {
				tgproxy = append(tgproxy, f)
			}
		case rel == "zapret" || strings.HasPrefix(rel, "zapret/"):
			if !f.FileInfo().IsDir() {
				zapret = append(zapret, f)
			}
		default:
			if !f.FileInfo().IsDir() {
				main = append(main, f)
			}
		}
	}
	return main, wintun, service, tgproxy, zapret, root, nil
}

// keepCRX — оставить уже установленный пакет плагина: пакет из установщика
// не новее лежащего на диске.
func keepCRX(dst string, f *zip.File) bool {
	have, err := extension.ReadVersion(dst)
	if err != nil || have == "" {
		return false
	}
	rc, err := f.Open()
	if err != nil {
		return false
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, 64<<20))
	if err != nil {
		return false
	}
	want, err := extension.ReadVersionBytes(data)
	if err != nil || want == "" {
		return false
	}
	return updater.CompareVersions(want, have) < 0
}

// relPath возвращает путь записи архива относительно dir; "" — если запись
// вне дерева префикса или пытается выйти за корень распаковки.
func relPath(f *zip.File, archPrefix string) string {
	name := filepath.ToSlash(filepath.Clean(f.Name))
	if !strings.HasPrefix(name, archPrefix) {
		return ""
	}
	rel := strings.TrimPrefix(name, archPrefix)
	if rel == "" || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, "/") {
		return ""
	}
	return rel
}

// hostArchPrefix — дерево пакета под реальную битность СИСТЕМЫ, а не процесса:
// Setup/Update собираются 32-битными (запуск на любой Windows), поэтому
// runtime.GOARCH у них всегда 386, и на 64-битном Windows без этой проверки
// ставилась x86-версия. Настоящая битность системы из 32-битного процесса
// видится через WOW64.
func hostArchPrefix() string {
	if runtime.GOARCH == "amd64" || runningUnderWOW64() {
		return "x64/"
	}
	return "x86/"
}

// runningUnderWOW64 — 32-битный процесс на 64-битном Windows.
func runningUnderWOW64() bool {
	var wow64 bool
	if err := windows.IsWow64Process(windows.CurrentProcess(), &wow64); err != nil {
		return false
	}
	return wow64
}

// extractTree распаковывает файлы пакета в dir с защитой пользовательских
// данных. withZapret=false (обновлятор) — дерево zapret не трогается вовсе,
// даже если пакет его содержит; withZapret=true (установщик) — zapret
// распаковывается только поверх и только если комплектная версия не старее
// установленной (zapretKeepVer).
func extractTree(p *Payload, dir string, step Step, uninstallExe, withZapret bool, zapretKeepVer string) error {
	prefix := hostArchPrefix()
	mainFiles, wintun, serviceFiles, tgproxyFiles, zapret, root, err := payloadTree(p, prefix)
	if err != nil {
		return err
	}

	if !withZapret {
		zapret = nil
	} else if payVer, _ := payloadZapretVersion(p); zapretKeepVer != "" && payVer != "" && !updater.IsNewer(zapretKeepVer, payVer) {
		// Даунгрейд запрета недопустим: пользователь мог обновить его
		// в приложении до версии новее комплектной.
		if step != nil {
			step("", -1, fmt.Sprintf("запрет %s оставлен — в пакете %s старее", zapretKeepVer, payVer))
		}
		zapret = nil
	}

	// Анти-даунгрейд защищает только более новый Flowseal; zapret2-поколение
	// (и битый без версии) подлежит замене комплектным дистрибутивом.
	if len(zapret) > 0 {
		migrateZapretGeneration(dir, step)
	}

	total := len(mainFiles) + len(wintun) + len(tgproxyFiles) + len(zapret) + 1
	done := 0
	report := func(base, from, to float64) {
		if step != nil {
			step("", base+(from/float64(total+1))*(to-base), "")
		}
	}

	for _, f := range mainFiles {
		rel := relPath(f, prefix)
		if rel == "" || isUserPath(rel) {
			done++
			continue
		}
		if err := extractOne(f, filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			return fmt.Errorf("файл %s: %w", rel, err)
		}
		done++
		report(0.10, float64(done), 0.72)
	}

	// legacy components/ сносится, wintun и служба кладутся заново (как в NSI)
	_ = os.RemoveAll(filepath.Join(dir, "components"))
	for _, f := range wintun {
		rel := relPath(f, prefix)
		if rel == "" {
			continue
		}
		if err := extractOne(f, filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			return fmt.Errorf("wintun: %w", err)
		}
		done++
		report(0.10, float64(done), 0.72)
	}

	// Служба может быть запущена и держать старый exe — обновление её файла
	// best-effort: приложение добьёт на старте (часть 2), установку не ломаем.
	var svcSkip int
	for _, f := range serviceFiles {
		rel := relPath(f, prefix)
		if rel == "" {
			continue
		}
		if err := extractOne(f, filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			svcSkip++
		}
		done++
		report(0.10, float64(done), 0.72)
	}
	if svcSkip > 0 && step != nil {
		step("", -1, "файл службы занят, пропущен — обновит приложение")
	}

	// Мост Telegram и браузерный плагин: обычно не запущены в момент
	// установки (KillApps), но если файл занят — не ломаем установку,
	// обновит приложение.
	for _, f := range tgproxyFiles {
		rel := relPath(f, prefix)
		if rel == "" {
			continue
		}
		// Даунгрейд пакета плагина недопустим: пользователь мог обновить
		// его в приложении до версии новее комплектной.
		if filepath.ToSlash(rel) == "components/extension/"+extension.File {
			if keepCRX(filepath.Join(dir, filepath.FromSlash(rel)), f) {
				if step != nil {
					step("", -1, "плагин браузера оставлен — в пакете версия старее")
				}
				done++
				continue
			}
		}
		if err := extractOne(f, filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			if step != nil {
				step("", -1, "файл моста Telegram занят, пропущен — обновит приложение")
			}
		}
		done++
		report(0.10, float64(done), 0.72)
	}

	var skipped int
	for _, f := range zapret {
		rel := relPath(f, prefix)
		if rel == "" || isUserPath(rel) {
			done++
			continue
		}
		dst := filepath.Join(dir, filepath.FromSlash(rel))
		if _, err := os.Stat(dst); err == nil && isLocked(dst) {
			skipped++
			done++
			continue
		}
		if err := extractOne(f, dst); err != nil {
			skipped++
		}
		done++
		report(0.10, float64(done), 0.72)
	}

	for _, f := range root {
		if filepath.ToSlash(f.Name) == "README.md" {
			_ = extractOne(f, filepath.Join(dir, "README.md"))
			break
		}
	}
	if uninstallExe {
		for _, f := range root {
			if filepath.ToSlash(f.Name) == "uninstall.exe" {
				_ = extractOne(f, filepath.Join(dir, "uninstall.exe"))
				break
			}
		}
	}
	if skipped > 0 && step != nil {
		step("", -1, fmt.Sprintf("занятые файлы zapret пропущены: %d", skipped))
	}
	return nil
}

// UpdateExisting — обновление установленной копии (порт ZPUI-Update.nsi):
// остановка, очистка логов прошлой версии, замена файлов, версия в реестре,
// перезапуск выполняет вызывающий.
// writePendingZapretBackup кладёт рядом с ZPUI.exe свежую копию установленного
// запрета. Активной она станет только после успешного запуска движка (ZPUI
// сам переименует .new в zapret-backup.zip) — неудачное обновление сохранит
// прошлую рабочую копию.
func writePendingZapretBackup(dir string, step Step) {
	if step != nil {
		step("Резервная копия zapret:", 0.86, "")
	}
	dst := filepath.Join(dir, zapret.PendingBackupName)
	if err := zapret.WriteBackupArchive(filepath.Join(dir, "zapret"), dst); err != nil {
		if step != nil {
			step("", -1, "резервная копия zapret не создана: "+err.Error())
		}
	}
}

func UpdateExisting(p *Payload, dir, newVer string, step Step) error {
	if newVer == "" || newVer == "0.0.0" {
		return errors.New("не задана версия обновления")
	}
	if step != nil {
		step("Закрытие ZPUI и движка…", 0.03, "")
	}
	// Служба гасится до KillApps: её watchdog иначе считает смерть ZPUI.exe
	// крашем, а её exe остался бы заблокированным на время замены файлов.
	_ = zsvc.Stop()
	KillApps()
	if err := WaitUnlocked(filepath.Join(dir, "ZPUI.exe"), 12); err != nil {
		return err
	}
	if step != nil {
		step("Очистка логов прошлой версии…", 0.07, "")
	}
	ClearLogs(dir)
	ub := BackupUserFiles(dir)
	defer ub.Restore(dir)
	if step != nil {
		step("Замена файлов приложения…", 0.10, "")
	}
	// Zapret2-остатки заменяются комплектным Flowseal только если пакет его
	// несёт (Setup — да, обновлятор — нет): без замены каталог лучше оставить
	// восстановлению при старте приложения.
	genReplaced := false
	if payloadHasZapret(p) {
		genReplaced = migrateZapretGeneration(dir, step)
	}
	if err := extractTree(p, dir, step, false, genReplaced, ""); err != nil {
		return err
	}
	if step != nil {
		step("Обновление ярлыков и реестра…", 0.88, "")
	}
	writeInstallRegistry(dir, newVer)
	writePendingZapretBackup(dir, step)
	registerService(dir, step)
	CleanupUpdateLeftovers(dir)
	return nil
}

// ClearLogs — журнал и текстовые логи предыдущей версии не должны
// подмешиваться к логам новой: чистим logs/ и databases/logs.db
// (пересоздаются приложением). Файлы уже не заняты — KillApps+WaitUnlocked.
func ClearLogs(dir string) {
	if entries, err := os.ReadDir(filepath.Join(dir, "logs")); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				_ = os.Remove(filepath.Join(dir, "logs", e.Name()))
			}
		}
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(filepath.Join(dir, "databases", "logs.db"+suffix))
	}
}

// InstallFresh — полная установка/переустановка (порт секций ZPUI.nsi):
// файлы, ярлыки, uninstaller, запись ARP. clean=true — сначала полное удаление.
// desktop/startMenu — какие ярлыки создавать. Более новая установленная версия
// запрета переживает даже чистую переустановку (папка уносится в сторону
// и возвращается поверх).
func InstallFresh(p *Payload, dir, ver string, clean, desktop, startMenu bool, step Step) error {
	if step != nil {
		step("Подготовка…", 0.02, "")
	}
	// Служба гасится до KillApps/удаления: watchdog не должен увидеть смерть
	// ZPUI.exe как краш, её exe не должен быть заблокирован при распаковке.
	_ = zsvc.Stop()
	KillApps()
	// Снимок пользовательских файлов ДО возможного полного удаления (clean):
	// после распаковки они возвращаются поверх, включая чистую установку.
	ub := BackupUserFiles(dir)
	var autostartOn bool
	defer func() {
		ub.Restore(dir)
		if autostartOn {
			// Конфиг уже восстановлен — флаг «свёрнутого запуска» читается из него.
			restoreAutostart(dir)
		}
	}()

	keepVer := zapretVersionFromDir(dir)
	if zapret2Installed(dir) {
		// zapret2-поколение не «переживает» установку как более новый Flowseal:
		// версия из его service.bat не должна блокировать замену на комплектный.
		keepVer = ""
	}
	var keptDir string
	if clean {
		if payVer, _ := payloadZapretVersion(p); keepVer != "" && payVer != "" && !updater.IsNewer(payVer, keepVer) {
			kept := filepath.Join(dir, "zapret.keep")
			if os.Rename(filepath.Join(dir, "zapret"), kept) == nil {
				keptDir = kept
			} else {
				keepVer = ""
			}
		} else {
			keepVer = ""
		}
		if step != nil {
			step("Удаление предыдущей копии…", 0.05, "")
		}
		autostartOn = RemoveInstallation(dir, true)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if step != nil {
		step("Распаковка файлов…", 0.10, "")
	}
	if err := extractTree(p, dir, step, true, true, keepVer); err != nil {
		return err
	}
	if keptDir != "" {
		// Restore уже вернул пользовательские списки в свежую папку — она
		// больше не нужна: сохранённая (новее) становится каноничной.
		_ = os.RemoveAll(filepath.Join(dir, "zapret"))
		_ = os.Rename(keptDir, filepath.Join(dir, "zapret"))
	}
	if step != nil {
		step("Ярлыки и реестр…", 0.90, "")
	}
	if err := CreateShortcuts(dir, desktop, startMenu); err != nil {
		return err
	}
	writeInstallRegistry(dir, ver)
	writeUninstallRegistry(dir, ver)
	writePendingZapretBackup(dir, step)
	registerService(dir, step)
	CleanupUpdateLeftovers(dir)
	return nil
}

// registerService — регистрация службы ZPUI, если в установке лежит
// components\service\ZPUI-Service.exe: старые (легаси zpui_service и прежняя
// ZPUI) сносятся, затем Install+Start. Ошибки не фатальны для установки —
// приложение добьёт регистрацию на старте.
func registerService(dir string, step Step) {
	svcExe := filepath.Join(dir, "components", "service", "ZPUI-Service.exe")
	if _, err := os.Stat(svcExe); err != nil {
		return
	}
	if step != nil {
		step("Регистрация службы ZPUI…", 0.93, "")
	}
	removeLegacyServices()
	if err := zsvc.Install(svcExe); err != nil {
		if step != nil {
			step("", -1, "служба не зарегистрирована: "+err.Error())
		}
		return
	}
	if err := zsvc.Start(); err != nil {
		if step != nil {
			step("", -1, "служба не запущена: "+err.Error())
		}
		return
	}
	if step != nil {
		step("", -1, "служба ZPUI установлена и запущена")
	}
}

// RemoveInstallation — полное удаление (порт секции Uninstall ZPUI.nsi).
// keepData=true оставляет только то, что удалять нельзя по соображениям
// обновления (не используется деинсталлятором — тот удаляет всё).
// Возвращает true, если до удаления был включён автозапуск: чистая
// установка возвращает его после распаковки, иначе он терялся навсегда
// (Run-запись сносилась, а пара «релей + задача ZPUI-Start» не
// восстанавливалась).
func RemoveInstallation(dir string, withData bool) bool {
	autostartOn := autostartWasEnabled()
	// Служба гасится до KillApps: её watchdog не должен считать
	// смерть ZPUI.exe крашем, её exe не должен блокировать удаление файлов.
	removeServices()
	KillApps()
	time.Sleep(500 * time.Millisecond)

	for _, name := range []string{"ZPUI.exe", "selfupdate.exe", "ZPUI-Service.exe", "security.exe", "wizard.exe", "uninstall.exe"} {
		_ = os.Remove(filepath.Join(dir, name))
	}
	for _, name := range []string{"checksums.sha256", "versions.json", "version.txt", "README.md", "zpui.db", "config.json"} {
		_ = os.Remove(filepath.Join(dir, name))
	}
	for _, sub := range []string{"components", "zapret", "databases", "mods", "backups", "skills", ".backup", "logs"} {
		_ = os.RemoveAll(filepath.Join(dir, sub))
	}
	_ = os.Remove(dir)

	if withData {
		// %APPDATA%\ZPUI.exe — дефолтный каталог данных WebView2 ZPUI (go-webview2
		// кладёт их в %APPDATA%\<имя exe>). Общие профили Microsoft\WebView2 и
		// EdgeWebView — данные всех приложений машины, их не трогаем.
		for _, p := range []string{appDataPath("ZPUI"), appDataPath("ZPUI.exe"), localAppDataPath("ZPUI")} {
			if p != "" {
				_ = os.RemoveAll(p)
			}
		}
	}

	// ярлыки
	if home, err := os.UserHomeDir(); err == nil {
		_ = os.Remove(filepath.Join(home, "Desktop", "ZPUI.lnk"))
		_ = os.Remove(filepath.Join(home, "Desktop", "ЗАПРЕТ.lnk"))
		if sm := startMenuPath(); sm != "" {
			_ = os.RemoveAll(filepath.Join(sm, "ZPUI"))
		}
	}

	// реестр (родной вид + легаси-записи 32-битных установок)
	for _, key := range []string{
		`Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI`,
		`Software\ZPUI`,
	} {
		deleteRegTree(registry.WOW64_64KEY, key)
		deleteRegTree(registry.WOW64_32KEY, key)
	}
	for _, key := range []string{
		`Software\Microsoft\Windows\CurrentVersion\Run`,
		`Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`,
	} {
		if k, err := registry.OpenKey(registry.CURRENT_USER, key, registry.SET_VALUE); err == nil {
			_ = k.DeleteValue("ZPUI")
			k.Close()
		}
	}

	// "ZPUI" — задача прежней схемы автозапуска, "ZPUI-Start" — текущей,
	// "ZPUI-Restore" — внутренняя задача краш-восстановления службы.
	for _, args := range [][]string{
		{"schtasks", "/delete", "/tn", "ZPUI", "/f"},
		{"schtasks", "/delete", "/tn", zsvc.StartTaskName, "/f"},
		{"schtasks", "/delete", "/tn", "ZPUI-Restore", "/f"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		_ = cmd.Run()
	}
	return autostartOn
}

// autostartWasEnabled — Run-запись автозапуска существует и не выключена
// в Диспетчере задач (StartupApproved\Run, первый байт нечётный = выкл).
func autostartWasEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	if _, _, err := k.GetStringValue("ZPUI"); err != nil {
		return false
	}
	if k2, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`, registry.QUERY_VALUE); err == nil {
		defer k2.Close()
		if v, _, err := k2.GetBinaryValue("ZPUI"); err == nil && len(v) > 0 && v[0]&1 == 1 {
			return false
		}
	}
	return true
}

// restoreAutostart возвращает автозапуск после чистой переустановки: та же
// пара «Run-запись-релей + задача ZPUI-Start», что создаёт приложение своим
// тумблером. Шаг не фатален для установки: если не удался, автозапуск
// остаётся выключенным, тумблер в приложении включит его снова.
func restoreAutostart(dir string) {
	exe := filepath.Join(dir, "ZPUI.exe")
	if _, err := os.Stat(exe); err != nil {
		return
	}
	if err := zsvc.CreateStartTask(exe, startMinimizedFromConfig(dir)); err != nil {
		return
	}
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE); err == nil {
		_ = k.SetStringValue("ZPUI", `"`+exe+`" --run-start-task`)
		k.Close()
	}
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`, registry.SET_VALUE); err == nil {
		_ = k.DeleteValue("ZPUI")
		k.Close()
	}
}

// startMinimizedFromConfig — флаг «запускать свёрнутым» из (восстановленного)
// config.json; он определяет --hidden в задаче автозапуска.
func startMinimizedFromConfig(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		return false
	}
	var cfg struct {
		StartMinimized bool `json:"start_minimized"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return false
	}
	return cfg.StartMinimized
}

// removeLegacyServices — sc delete легаси-имени службы zpui_service.
func removeLegacyServices() {
	cmd := exec.Command("sc", "delete", "zpui_service")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	_ = cmd.Run()
}

// removeServices — stop+delete службы ZPUI (новая, через SCM-хелперы zsvc)
// и legacy-имени (zpui_service) через sc delete как страховку.
func removeServices() {
	_ = zsvc.Remove()
	removeLegacyServices()
}

// FindInstall находит установленную копию: родной вид реестра, затем
// легаси-записи 32-битных установок (WOW64-редирект). Запись реестра
// засчитывается только если в указанном каталоге лежит живой ZPUI.exe —
// остаточный ключ после ручной чистки не должен включать для новичка
// вопрос «обновить или чистая установка».
func FindInstall() (dir, ver string, err error) {
	for _, acc := range []uint32{registry.QUERY_VALUE | registry.WOW64_64KEY, registry.QUERY_VALUE} {
		k, e := registry.OpenKey(registry.LOCAL_MACHINE, `Software\ZPUI`, acc)
		if e != nil {
			continue
		}
		d, _, e := k.GetStringValue("InstallDir")
		v, _, _ := k.GetStringValue("Version")
		k.Close()
		if e != nil || d == "" {
			continue
		}
		if _, sterr := os.Stat(filepath.Join(d, "ZPUI.exe")); sterr != nil {
			continue // каталога нет — установка умерла, считаем чистую установку
		}
		return d, v, nil
	}
	return "", "", errors.New("ZPUI не установлен на этом компьютере")
}

func writeInstallRegistry(dir, ver string) {
	if k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, `Software\ZPUI`, registry.SET_VALUE|registry.WOW64_64KEY); err == nil {
		_ = k.SetStringValue("InstallDir", dir)
		_ = k.SetStringValue("Version", ver)
		k.Close()
	}
}

func writeUninstallRegistry(dir, ver string) {
	if k, _, err := registry.CreateKey(registry.LOCAL_MACHINE,
		`Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI`, registry.SET_VALUE|registry.WOW64_64KEY); err == nil {
		_ = k.SetStringValue("DisplayName", "ZPUI")
		_ = k.SetStringValue("DisplayVersion", ver)
		_ = k.SetStringValue("DisplayIcon", filepath.Join(dir, "ZPUI.exe"))
		_ = k.SetStringValue("InstallLocation", dir)
		_ = k.SetStringValue("Publisher", "SuzucaRU")
		_ = k.SetStringValue("UninstallString", `"`+filepath.Join(dir, "uninstall.exe")+`"`)
		_ = k.SetStringValue("QuietUninstallString", `"`+filepath.Join(dir, "uninstall.exe")+`" /S`)
		_ = k.SetDWordValue("NoModify", 1)
		_ = k.SetDWordValue("NoRepair", 1)
		_ = k.SetDWordValue("EstimatedSize", dirSizeKB(dir))
		k.Close()
	}
}

func dirSizeKB(dir string) uint32 {
	var total int64
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return uint32(total / 1024)
}

// deleteRegTree удаляет ключ в заданном виде реестра (WOW64-флаг).
func deleteRegTree(view uint32, path string) {
	i := strings.LastIndexByte(path, '\\')
	if i < 0 {
		return
	}
	parent, sub := path[:i], path[i+1:]
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, parent, registry.QUERY_VALUE|registry.SET_VALUE|view)
	if err != nil {
		return
	}
	_ = registry.DeleteKey(k, sub)
	k.Close()
}

// CreateShortcuts создаёт ярлыки (IShellLink через скрытый PowerShell —
// Wails-процесс не тащит COM-обвязку): рабочий стол и/или «Пуск».
func CreateShortcuts(dir string, desktop, startMenu bool) error {
	exe := filepath.Join(dir, "ZPUI.exe")
	if _, err := os.Stat(exe); err != nil {
		return nil
	}
	var targets []string
	if desktop {
		if home, err := os.UserHomeDir(); err == nil {
			targets = append(targets, filepath.Join(home, "Desktop", "ZPUI.lnk"))
		}
	}
	if startMenu {
		if sm := startMenuPath(); sm != "" {
			if err := os.MkdirAll(filepath.Join(sm, "ZPUI"), 0o755); err == nil {
				targets = append(targets, filepath.Join(sm, "ZPUI", "ZPUI.lnk"))
			}
		}
	}
	for _, lnk := range targets {
		script := fmt.Sprintf(`$s=(New-Object -ComObject WScript.Shell).CreateShortcut('%s');$s.TargetPath='%s';$s.WorkingDirectory='%s';$s.IconLocation='%s,0';$s.Description='ZPUI — Zapret DPI bypass controller';$s.Save()`,
			escapePS(lnk), escapePS(exe), escapePS(dir), escapePS(exe))
		cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-EncodedCommand", psEncode(script))
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("ярлык %s: %v: %s", lnk, err, strings.TrimSpace(string(out)))
		}
	}
	// легаси-имена
	if home, err := os.UserHomeDir(); err == nil {
		_ = os.Remove(filepath.Join(home, "Desktop", "ЗАПРЕТ.lnk"))
	}
	return nil
}

func escapePS(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// psEncode — UTF-16LE base64 для -EncodedCommand: пути с апострофами и
// прочими спецсимволами не ломают скрипт на этапе парсинга PowerShell.
func psEncode(script string) string {
	u16 := utf16.Encode([]rune(script))
	b := make([]byte, len(u16)*2)
	for i, v := range u16 {
		binary.LittleEndian.PutUint16(b[i*2:], v)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func startMenuPath() string {
	pf := os.Getenv("PROGRAMDATA")
	if pf == "" {
		return ""
	}
	return filepath.Join(pf, "Microsoft", "Windows", "Start Menu", "Programs")
}

func appDataPath(sub string) string {
	ad := os.Getenv("APPDATA")
	if ad == "" {
		return ""
	}
	return filepath.Join(ad, filepath.FromSlash(sub))
}

func localAppDataPath(sub string) string {
	ad := os.Getenv("LOCALAPPDATA")
	if ad == "" {
		return ""
	}
	return filepath.Join(ad, filepath.FromSlash(sub))
}

// parseZapretVersion — LOCAL_VERSION из service.bat (как detectZapretVersion
// в приложении): set "LOCAL_VERSION=1.10.4" → 1.10.4.
func parseZapretVersion(script string) string {
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "REM ") || strings.HasPrefix(upper, "::") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(upper, "LOCAL_VERSION") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				v := strings.TrimSpace(parts[1])
				if idx := strings.Index(v, "&"); idx >= 0 {
					v = v[:idx]
				}
				v = strings.Trim(strings.TrimSpace(v), `"`)
				if v != "" {
					return v
				}
			}
		}
	}
	return ""
}

// zapretVersionFromDir — версия установленного запрета (zapret/service.bat).
func zapretVersionFromDir(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "zapret", "service.bat"))
	if err != nil {
		return ""
	}
	return parseZapretVersion(string(data))
}

// zapretUserLists — пользовательские списки в каталоге zapret, сохраняемые
// при смене поколения движка (тот же набор, что в protectedUserRel).
var zapretUserLists = []string{
	"lists/list-general-user.txt",
	"lists/list-exclude-user.txt",
	"lists/ipset-exclude-user.txt",
	"lists/list-exclude-system.txt",
}

// zapret2Installed — установленный zapret не является Flowseal-дистрибутивом:
// bin/winws2.exe от zapret2, либо service.bat отсутствует/без версии
// (поколение не распознать — замене подлежит).
func zapret2Installed(dir string) bool {
	zapretDir := filepath.Join(dir, "zapret")
	if fi, err := os.Stat(zapretDir); err != nil || !fi.IsDir() {
		return false
	}
	if _, err := os.Stat(filepath.Join(zapretDir, "bin", "winws2.exe")); err == nil {
		return true
	}
	data, err := os.ReadFile(filepath.Join(zapretDir, "service.bat"))
	if err != nil {
		return true
	}
	return parseZapretVersion(string(data)) == ""
}

// migrateZapretGeneration сносит каталог zapret zapret2-поколения перед
// распаковкой Flowseal из пакета, сохранив пользовательские списки.
// Возвращает true, если каталог был удалён.
func migrateZapretGeneration(dir string, step Step) bool {
	zapretDir := filepath.Join(dir, "zapret")
	if !zapret2Installed(dir) {
		return false
	}
	if step != nil {
		step("", -1, "обнаружен дистрибутив zapret2 — замена на комплектный zapret")
	}
	saved := make(map[string][]byte, len(zapretUserLists))
	for _, rel := range zapretUserLists {
		if data, err := os.ReadFile(filepath.Join(zapretDir, filepath.FromSlash(rel))); err == nil {
			saved[rel] = data
		}
	}
	if err := os.RemoveAll(zapretDir); err != nil {
		if step != nil {
			step("", -1, "не удалось удалить старый каталог zapret: "+err.Error())
		}
		return false
	}
	for rel, data := range saved {
		dst := filepath.Join(zapretDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			continue
		}
		_ = os.WriteFile(dst, data, 0o644)
	}
	return true
}

// payloadHasZapret — пакет содержит дерево zapret (Setup несёт, обновлятор нет).
func payloadHasZapret(p *Payload) bool {
	_, _, _, _, zapret, _, err := payloadTree(p, hostArchPrefix())
	if err != nil {
		return false
	}
	return len(zapret) > 0
}

// payloadZapretVersion — версия запрета в пакете (x86/x64/zapret/service.bat).
func payloadZapretVersion(p *Payload) (string, error) {
	r, err := zip.NewReader(p.Reader, p.Size)
	if err != nil {
		return "", err
	}
	want := hostArchPrefix() + "zapret/service.bat"
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if filepath.ToSlash(filepath.Clean(f.Name)) != want {
			continue
		}
		src, err := f.Open()
		if err != nil {
			return "", err
		}
		defer src.Close()
		data, err := io.ReadAll(src)
		if err != nil {
			return "", err
		}
		return parseZapretVersion(string(data)), nil
	}
	return "", nil
}

// LaunchApp запускает установленное приложение отсоединённо и ВСЕГДА с
// развёрнутым окном: --hidden применяется только к автозапуску при входе в
// Windows. Запуск после установки/обновления — видимый (решение владельца
// 13.11.2026: после обновления окно должно разворачиваться, а не уходить в трей).
func LaunchApp(dir string) {
	exe := filepath.Join(dir, "ZPUI.exe")
	if _, err := os.Stat(exe); err != nil {
		return
	}
	cmd := exec.Command(exe)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008} // DETACHED_PROCESS
	_ = cmd.Start()
}

// IsElevated — запущен ли процесс с правами администратора.
func IsElevated() bool {
	tok, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return false
	}
	defer tok.Close()
	return tok.IsElevated()
}

// Elevate перезапускает процесс с повышением (скрытый PowerShell RunAs).
func Elevate() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	args := ""
	if len(os.Args) > 1 {
		quoted := make([]string, len(os.Args[1:]))
		for i, a := range os.Args[1:] {
			quoted[i] = "'" + escapePS(a) + "'"
		}
		args = " -ArgumentList " + strings.Join(quoted, ",")
	}
	script := "Start-Process '" + escapePS(exe) + "' -Verb RunAs" + args
	cmd := exec.Command("powershell", "-NoProfile", "-EncodedCommand", psEncode(script))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd.Run() == nil
}

// WebView2Installed проверяет наличие Evergreen Runtime (нужен Wails-окну).
func WebView2Installed() bool {
	keys := []struct {
		view uint32
		path string
	}{
		{registry.WOW64_64KEY, `SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`},
		{0, `SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`},
		{registry.WOW64_64KEY, `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`},
	}
	for _, k := range keys {
		if rk, err := registry.OpenKey(registry.LOCAL_MACHINE, k.path, registry.QUERY_VALUE|k.view); err == nil {
			_, _, err := rk.GetStringValue("pv")
			rk.Close()
			if err == nil {
				return true
			}
		}
	}
	return false
}

// NoWebViewError — ошибка отсутствия WebView2 Runtime.
var NoWebViewError = errors.New("не установлен WebView2 Runtime")

// CheckWebView2 завершает процесс с понятным сообщением, если Runtime нет.
func CheckWebView2() {
	if WebView2Installed() {
		return
	}
	MessageBoxStop("ZPUI", "Для работы установщика нужен WebView2 Runtime.\r\nОн входит в Windows 10/11 и обычно уже установлен.\r\n\r\nУстановите «Evergreen WebView2 Runtime» с сайта Microsoft и запустите установщик снова.")
	os.Exit(1)
}

// RoundOwnWindow скругляет углы собственного окна (Windows 11, DWM).
// На Windows 10 вызов тихо ничего не меняет — углы останутся прямыми.
func RoundOwnWindow() bool {
	user32 := syscall.NewLazyDLL("user32.dll")
	dwm := syscall.NewLazyDLL("dwmapi.dll")
	procEnumWindows := user32.NewProc("EnumWindows")
	procGetWindowPID := user32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible := user32.NewProc("IsWindowVisible")
	procDwmSetWindowAttribute := dwm.NewProc("DwmSetWindowAttribute")

	me := uint32(os.Getpid())
	var hwnd uintptr
	cb := syscall.NewCallback(func(h, l uintptr) uintptr {
		var pid uint32
		procGetWindowPID.Call(h, uintptr(unsafe.Pointer(&pid)))
		if pid == me {
			vis, _, _ := procIsWindowVisible.Call(h)
			if vis != 0 {
				hwnd = h
				return 0
			}
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)
	if hwnd == 0 {
		return false
	}
	pref := uint32(2) // DWMWCP_ROUND
	r, _, _ := procDwmSetWindowAttribute.Call(hwnd, 33, uintptr(unsafe.Pointer(&pref)), 4)
	return r == 0
}

// MessageBoxStop показывает системный блокирующий MessageBox с ошибкой.
func MessageBoxStop(title, text string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	procMessageBox := user32.NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString(title)
	b, _ := syscall.UTF16PtrFromString(text)
	procMessageBox.Call(0, uintptr(unsafe.Pointer(b)), uintptr(unsafe.Pointer(t)), 0x10)
}
