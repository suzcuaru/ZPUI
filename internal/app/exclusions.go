package app

// Система исключений доменов v5:
//
//	skip-resources.txt (рядом с config.json) — пользовательский скип-лист:
//	редактируется в UI, автодопис мёртвых доменов, мигрированный контент
//	старого exclusions-server.txt.
//	skip-resources-system.txt (рядом с config.json) — системный скип-лист:
//	владелец приложение, дефолт — только шапка, целиком заменяется при смене
//	версии набора с сервера (op state.sync, см. app_v5.go). Оба файла учитываются
//	гейтами проверок (cfg.IsSkippedResource) и автоподборов (internal/zapret).
//	zapret/lists/list-exclude-system.txt — статическая DPI-база (банки/гос,
//	go:embed), инжектится в секции winws; сервером больше не обновляется.
//	zapret/lists/list-exclude-user.txt — пользовательский DPI-хостлист: если
//	нет — создаётся с шапкой, есть — не трогается никогда. Сюда дописываются
//	ручные добавления из UI (с дедупликацией).
//
// Старый конвейер v4 (pending→confirmed→alive, exclusions-server.txt) снесён:
// клиент больше не пишет серверные исключения в списки дистрибутива.

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

//go:embed list-exclude-system-default.txt
var systemExcludeDefault string

const (
	systemExcludeFile = "list-exclude-system.txt"
	userExcludeFile   = "list-exclude-user.txt"
)

func (a *App) systemExcludePath() string {
	return filepath.Join(a.cfg.ListsDir(), systemExcludeFile)
}

func (a *App) userExcludePath() string {
	return filepath.Join(a.cfg.ListsDir(), userExcludeFile)
}

// legacySystemDefault — первый вшитый дефолт системного списка (только шапка,
// без доменов): у установленных до 19.10 копий на диске именно он. Такой файл
// безопасно заменить новым базовым списком — системный файл принадлежит
// приложению и пользователем не редактируется.
const legacySystemDefault = `# list-exclude-system.txt — системный список исключений доменов (ZPUI).
# Файл обновляется приложением с сервера: ручные правки будут перезаписаны.
# Пользовательские исключения — в list-exclude-user.txt.`

// ensureExclusionFiles — при старте оба файла существуют: системный с вшитым
// дефолтом (до первой синка с сервером), пользовательский с шапкой.
func (a *App) ensureExclusionFiles() {
	if err := os.MkdirAll(a.cfg.ListsDir(), 0o755); err != nil {
		a.log.Warn("exclusions", "mkdir lists: "+err.Error())
		return
	}
	if data, err := os.ReadFile(a.systemExcludePath()); err != nil {
		if err := os.WriteFile(a.systemExcludePath(), []byte(systemExcludeDefault), 0o644); err != nil {
			a.log.Warn("exclusions", "create system list: "+err.Error())
		}
	} else if strings.TrimSpace(string(data)) == legacySystemDefault {
		// Старый пустой дефолт — поднимаем до нового базового списка.
		if err := os.WriteFile(a.systemExcludePath(), []byte(systemExcludeDefault), 0o644); err != nil {
			a.log.Warn("exclusions", "upgrade system list: "+err.Error())
		} else {
			a.log.Info("exclusions", "system list upgraded to base set")
		}
	}
	if _, err := os.Stat(a.userExcludePath()); os.IsNotExist(err) {
		header := "# list-exclude-user.txt — пользовательские исключения доменов.\n" +
			"# Строки с # игнорируются. Файл никогда не перезаписывается приложением.\n"
		if err := os.WriteFile(a.userExcludePath(), []byte(header), 0o644); err != nil {
			a.log.Warn("exclusions", "create user list: "+err.Error())
		}
	}
	a.migrateLegacyServerExclusions()
}

// migrateLegacyServerExclusions — одноразовый перенос старого файла серверных
// исключений (конвейер v4 снесён, файл больше не читается). Домены там —
// найденные мёртвые домены, им место в skip-resources.txt: переносим с
// авто-комментарием и дедупликацией, сам файл переименовывается в .migrated.
// Повторные запуски безопасны: отсутствие файла + дедупликация.
func (a *App) migrateLegacyServerExclusions() {
	legacy := filepath.Join(filepath.Dir(a.cfg.GetSkipResourcesFilePath()), "exclusions-server.txt")
	if _, err := os.Stat(legacy); err != nil {
		return
	}
	domains := listDomains(legacy)
	existing := a.cfg.GetSkipResources()
	added := 0
	for _, d := range domains {
		covered := false
		for _, e := range existing {
			if d == e || strings.HasSuffix(d, "."+e) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		comment := fmt.Sprintf("перенесено из старого списка серверных исключений, %s", time.Now().Format("02.01.2006"))
		if err := a.cfg.AddSkipResourceWithComment(d, comment); err == nil {
			added++
		}
	}
	if err := os.Rename(legacy, legacy+".migrated"); err != nil {
		a.log.Warn("exclusions", "rename legacy exclusions-server.txt: "+err.Error())
	}
	a.log.Info("exclusions", fmt.Sprintf("migrated legacy server exclusions: %d domains, %d added to skip-resources.txt", len(domains), added))
}

// listDomains — домены файла (строки без #).
func listDomains(path string) []string {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.ToLower(strings.TrimSpace(line))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

func (a *App) systemExclusions() []string { return listDomains(a.systemExcludePath()) }

func (a *App) userExclusions() []string { return a.cfg.GetUserExclusions() }

// writeSystemExclusions — полная замена системного skip-файла при смене
// версии набора с сервера (skip-resources-system.txt рядом с config.json).
// DPI-база list-exclude-system.txt сервером больше не обновляется.
func (a *App) writeSystemExclusions(domains []string) error {
	sort.Strings(domains)
	var b strings.Builder
	b.WriteString("# skip-resources-system.txt — системный список пропускаемых ресурсов, обновляется с сервера ZPUI.\n")
	b.WriteString("# Редактирование не предусмотрено: файл целиком заменяется при смене версии набора с сервера.\n")
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" || strings.HasPrefix(d, "#") {
			continue
		}
		b.WriteString(d + "\n")
	}
	return os.WriteFile(a.cfg.GetSkipResourcesSystemFilePath(), []byte(b.String()), 0o644)
}

// addUserExclusion — дописать домен в пользовательский список с дедупликацией.
// Уже существующий домен (или его родитель) не дублируется. В файле только
// сами домены — никаких служебных приписок.
func (a *App) addUserExclusion(host string) error {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimSuffix(host, ".")
	if host == "" || strings.HasPrefix(host, "#") {
		return fmt.Errorf("пустое имя домена")
	}
	if strings.ContainsAny(host, " \t/\\") {
		return fmt.Errorf("недопустимые символы в домене")
	}
	existing := a.userExclusions()
	for _, d := range existing {
		if host == d || strings.HasSuffix(host, "."+d) {
			return nil
		}
	}
	f, err := os.OpenFile(a.userExcludePath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		// Каталог lists мог не существовать (движок ещё не устанавливался).
		if mkErr := os.MkdirAll(filepath.Dir(a.userExcludePath()), 0o755); mkErr != nil {
			return err
		}
		if f, err = os.OpenFile(a.userExcludePath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err != nil {
			return err
		}
	}
	defer f.Close()
	_, err = f.WriteString(host + "\n")
	return err
}

// excludedHost — хост исключён из проверок доступности: единый гейт конфига
// (skip-файлы + пользовательский и системный списки исключений).
func (a *App) excludedHost(host string) bool {
	return a.cfg.IsExcludedHost(host)
}
