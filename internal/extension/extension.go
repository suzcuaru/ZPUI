// Пакет браузерного расширения ZPUI: установка в Chromium-браузеры
// (Chrome, Edge, Яндекс, Opera, Brave, Vivaldi) механизлом external extensions.
//
// Как это работает (проверено на Chrome 158 и Edge 158): браузер при старте
// читает HKCU\Software\<вендор>\<браузер>\Extensions\<ID> и ставит пакет из
// значения `path`. Работает ТОЛЬКО подписанный CRX3: распакованная папка и
// zip в `path` молча игнорируются. Пакета два: в установке приложения
// (components\extension\) и standalone — %LOCALAPPDATA%\ZPUI\extension\,
// его кладёт ZPUI-Extension-Setup (cmd\extsetup).
//
// Chromium не из магазина включает внешнее расширение в отключённом виде —
// пользователь включает его один раз на странице расширений браузера
// (OpenExtensionsPage открывает её прямо на карточке).
//
// Стабильный ID = хеш публичного ключа из manifest.json ("key"). Приватный
// ключ живёт в ZPUI-Extension\build\key.pem (в дистрибутив не входит):
// им подписывается CRX утилитой ZPUI-Extension\tools\mkcrx.go.
package extension

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows/registry"

	"zpui/internal/executil"
)

// ID — стабильный идентификатор расширения (совпадает с manifest.json key).
const ID = "mlcbhlmemjlonmolmhlkdfhghdhfjhin"

// File — имя пакета в components\extension\.
const File = "zpui-extension.crx"

// Browser — Chromium-браузер: ветка внешних расширений в HKCU, исполняемый
// файл, места поиска и схема внутренних страниц (edge://, browser:// ...).
type Browser struct {
	ID    string
	Name  string
	Key   string
	Exe   string
	Paths []string // относительно корней поиска
	Scope string   // chrome | edge | browser | opera | brave | vivaldi
	// Profiles — каталоги профилей относительно ${LOCALAPPDATA}: внутри
	// Profile* / Default с файлом Preferences, либо сам каталог (Opera).
	Profiles []string
}

var browsers = []Browser{
	{ID: "chrome", Name: "Google Chrome", Key: `Software\Google\Chrome\Extensions`, Exe: "chrome.exe",
		Paths:    []string{`Google\Chrome\Application\chrome.exe`},
		Profiles: []string{`Google\Chrome\User Data`}, Scope: "chrome"},
	{ID: "edge", Name: "Microsoft Edge", Key: `Software\Microsoft\Edge\Extensions`, Exe: "msedge.exe",
		Paths:    []string{`Microsoft\Edge\Application\msedge.exe`},
		Profiles: []string{`Microsoft\Edge\User Data`}, Scope: "edge"},
	{ID: "yandex", Name: "Яндекс Браузер", Key: `Software\Yandex\YandexBrowser\Extensions`, Exe: "browser.exe",
		Paths:    []string{`Yandex\YandexBrowser\Application\browser.exe`},
		Profiles: []string{`Yandex\YandexBrowser\User Data`}, Scope: "browser"},
	// Opera / Brave / Vivaldi: ветки по устройству Chromium и таблице вендоров
	// Chromium; на живых браузерах не проверялись — если ветка не подхватится,
	// пакет в этом браузере просто не появится (ошибки не будет).
	{ID: "opera", Name: "Opera", Key: `Software\Opera Software\Extensions`, Exe: "opera.exe",
		Paths:    []string{`Programs\Opera\launcher.exe`, `Opera\Application\opera.exe`},
		Profiles: []string{`Opera Software\Opera Stable`, `Programs\Opera`}, Scope: "opera"},
	{ID: "brave", Name: "Brave", Key: `Software\BraveSoftware\Brave-Browser\Extensions`, Exe: "brave.exe",
		Paths:    []string{`BraveSoftware\Brave-Browser\Application\brave.exe`},
		Profiles: []string{`BraveSoftware\Brave-Browser\User Data`}, Scope: "brave"},
	{ID: "vivaldi", Name: "Vivaldi", Key: `Software\Vivaldi\Extensions`, Exe: "vivaldi.exe",
		Paths:    []string{`Vivaldi\Application\vivaldi.exe`},
		Profiles: []string{`Vivaldi\User Data`}, Scope: "vivaldi"},
}

// roots — где ищем браузеры. Пользовательские установки и HKCU не требуют
// прав администратора — установка работает из обычного пользователя.
var roots = []string{"${ProgramFiles}", "${ProgramFiles(x86)}", "${LOCALAPPDATA}"}

func byID(id string) (Browser, bool) {
	for _, b := range browsers {
		if b.ID == id {
			return b, true
		}
	}
	return Browser{}, false
}

// appPath — значение по умолчанию в App Paths (HKCU, затем HKLM).
func appPath(exe string) string {
	const sub = `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\`
	for _, hive := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		k, err := registry.OpenKey(hive, sub+exe, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		v, _, err := k.GetStringValue("")
		k.Close()
		if err != nil {
			continue
		}
		v = os.ExpandEnv(v)
		if st, serr := os.Stat(v); serr == nil && !st.IsDir() {
			return v
		}
	}
	return ""
}

// findExe — путь к браузеру: по подкаталогам в корнях, затем App Paths.
func (b Browser) findExe() string {
	for _, root := range roots {
		r := os.ExpandEnv(root)
		if r == "" {
			continue
		}
		for _, rel := range b.Paths {
			p := filepath.Join(r, filepath.FromSlash(rel))
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}
	return appPath(b.Exe)
}

// isRunning — запущен ли браузер: чтобы перечитать реестр, его надо перезапустить.
func (b Browser) isRunning() bool {
	out, err := executil.RunWithTimeout(5*time.Second, "tasklist", "/FI", "IMAGENAME eq "+b.Exe, "/NH", "/FO", "CSV")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(out), strings.ToLower(`"`+b.Exe+`"`))
}

func regKey(b Browser) string {
	return b.Key + `\` + ID
}

// regVersion — версия пакета, записанная в реестре для браузера ("" = не установлено).
func regVersion(b Browser) string {
	k, err := registry.OpenKey(registry.CURRENT_USER, regKey(b), registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue("version")
	if err != nil {
		return ""
	}
	return v
}

// PkgPath — components\extension\zpui-extension.crx; "" — пакета нет.
func PkgPath(exeDir string) string {
	path := filepath.Join(exeDir, "components", "extension", File)
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

// LocalDir — канонический каталог standalone-пакета установщика
// ZPUI-Extension-Setup: %LOCALAPPDATA%\ZPUI\extension.
func LocalDir() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "ZPUI", "extension")
}

// LocalPath — путь к standalone-пакету; "" — пакета нет.
func LocalPath() string {
	path := filepath.Join(LocalDir(), File)
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

// LocalVersion — версия standalone-пакета ("" — пакета нет).
func LocalVersion() string {
	p := LocalPath()
	if p == "" {
		return ""
	}
	v, _ := ReadVersion(p)
	return v
}

// ReadVersion — версия из manifest.json внутри CRX-пакета.
func ReadVersion(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return ReadVersionBytes(data)
}

// Version — версия пакета в установке приложения ("" — пакета нет).
func Version(exeDir string) string {
	p := PkgPath(exeDir)
	if p == "" {
		return ""
	}
	v, _ := ReadVersion(p)
	return v
}

// Status — состояние одного браузера для модалки. Selected — авто-выбор
// в диалоге: браузеры, где расширение уже стоит, выбраны сами.
type Status struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Found            bool   `json:"found"`   // браузер найден на диске
	Running          bool   `json:"running"` // запущен — нужен перезапуск
	HasExtension     bool   `json:"has_extension"`
	InstalledVersion string `json:"installed_version"`
	// State — состояние в самом браузере (по профилю): enabled | disabled |
	// "" — браузер ещё не подхватил регистрацию.
	State    string `json:"state"`
	Selected bool   `json:"selected"`
}

// Состояние плагина в самом браузере (из профиля, а не из реестра):
// расширение есть в профиле, даже когда пользователь его выключил.
const (
	StateEnabled  = "enabled"  // включён хотя бы в одном профиле
	StateDisabled = "disabled" // есть в профиле, везде выключен
)

var (
	// newer — строго ли версия a новее b (updater.CompareVersions);
	// аплинк через SetVersionCmp, чтобы не тянуть updater в пакет.
	newer     = func(a, b string) bool { return false }
	newerOnce sync.Once
)

// SetVersionCmp — функция сравнения версий (см. newer).
func SetVersionCmp(fn func(a, b string) int) {
	newerOnce.Do(func() { newer = func(a, b string) bool { return fn(a, b) > 0 } })
}

// Пакет плагина, с которым работает приложение: берём более новый из
// установки (components\extension) и standalone-каталога установщика
// ZPUI-Extension-Setup (%LOCALAPPDATA%\ZPUI\extension). Без сравнения
// (updater не подключён) предпочтение у пакета приложения.
func Local(exeDir string) (path, ver, source string) {
	appPath, appVer := PkgPath(exeDir), ""
	if appPath != "" {
		appVer, _ = ReadVersion(appPath)
	}
	locPath, locVer := LocalPath(), ""
	if locPath != "" {
		locVer, _ = ReadVersion(locPath)
	}
	if locVer != "" && (appVer == "" || newer(locVer, appVer)) {
		return locPath, locVer, "local"
	}
	if appPath != "" && appVer != "" {
		return appPath, appVer, "app"
	}
	return "", "", ""
}

var stateCache = struct {
	mu sync.Mutex
	m  map[string]stateCacheItem
}{m: map[string]stateCacheItem{}}

type stateCacheItem struct {
	mod  time.Time
	size int64
	res  profileState
}

// profileState — сводка по одному файлу Preferences: есть ли в нём запись
// о плагине и включён ли он.
type profileState struct {
	present int
	enabled bool
}

// readProfileState читает extensions.settings.<ID> из Preferences браузера.
// Файлы велики (мегабайты) и модалка их опрашивает — результат кэшируется
// по mtime+size и перечитывается только при изменении файла.
func readProfileState(path string) profileState {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return profileState{}
	}
	stateCache.mu.Lock()
	if c, ok := stateCache.m[path]; ok && c.mod.Equal(st.ModTime()) && c.size == st.Size() {
		stateCache.mu.Unlock()
		return c.res
	}
	stateCache.mu.Unlock()

	res := profileState{}
	data, err := os.ReadFile(path)
	if err == nil {
		var prefs struct {
			Extensions struct {
				Settings map[string]struct {
					State          *int  `json:"state"`
					DisableReasons []int `json:"disable_reasons"`
				} `json:"settings"`
			} `json:"extensions"`
		}
		if json.Unmarshal(data, &prefs) == nil {
			if e, ok := prefs.Extensions.Settings[ID]; ok {
				res.present++
				// state 1 без причин отключения = включён.
				if e.State != nil && *e.State == 1 && len(e.DisableReasons) == 0 {
					res.enabled = true
				}
			}
		}
	}

	stateCache.mu.Lock()
	stateCache.m[path] = stateCacheItem{mod: st.ModTime(), size: st.Size(), res: res}
	stateCache.mu.Unlock()
	return res
}

// browserState — состояние плагина в профилях браузера: StateEnabled /
// StateDisabled, либо "" (ни в одном профиле плагина нет).
func (b Browser) browserState() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return ""
	}
	present, enabled := 0, false
	for _, rel := range b.Profiles {
		root := filepath.Join(base, filepath.FromSlash(rel))
		if st, err := os.Stat(root); err != nil || !st.IsDir() {
			continue
		}
		// Сам каталог — профиль (Opera), либо подкаталоги Default / Profile*.
		prefs := []string{filepath.Join(root, "Preferences")}
		if entries, err := os.ReadDir(root); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					prefs = append(prefs, filepath.Join(root, e.Name(), "Preferences"))
				}
			}
		}
		for _, p := range prefs {
			ps := readProfileState(p)
			present += ps.present
			enabled = enabled || ps.enabled
		}
	}
	if enabled {
		return StateEnabled
	}
	if present > 0 {
		return StateDisabled
	}
	return ""
}

// regPathExists — существует ли пакет, на который ссылается запись реестра.
func regPathExists(b Browser) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, regKey(b), registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	p, _, err := k.GetStringValue("path")
	if err != nil || p == "" {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// List — браузеры на этой машине и состояние расширения в каждом.
// has_extension = расширение стоит в браузере: регистрация в реестре или
// запись в профиле (выключенное тоже считается установленным).
// state — состояние в профиле: enabled | disabled | "" (браузер ещё не
// подхватил регистрацию — нужен его перезапуск).
func List(exeDir string) []Status {
	out := make([]Status, 0, len(browsers))
	for _, b := range browsers {
		exe := b.findExe()
		ver, state := "", ""
		if exe != "" {
			// Регистрация есть, а пакета, на который она ссылается, уже нет —
			// браузер снимет расширение при старте, считаем не установленным.
			if v := regVersion(b); v != "" && regPathExists(b) {
				ver = v
			}
			state = b.browserState()
		}
		has := ver != "" || state != ""
		out = append(out, Status{
			ID:               b.ID,
			Name:             b.Name,
			Found:            exe != "",
			Running:          exe != "" && b.isRunning(),
			HasExtension:     has,
			InstalledVersion: ver,
			State:            state,
			Selected:         has,
		})
	}
	return out
}

// Install — регистрирует пакет приложения (components\extension\) в выбранных
// браузерах. Тонкая обёртка над InstallPath.
func Install(exeDir string, ids []string) map[string]string {
	path, ver, _ := Local(exeDir)
	return InstallPath(path, ver, ids)
}

// InstallPath — записывает в реестр выбранных браузеров явный путь к пакету
// и его версию (пакет приложения или standalone). Браузер подхватывает пакет
// при следующем старте. Ошибки — по браузерам.
func InstallPath(pkg, ver string, ids []string) map[string]string {
	fails := map[string]string{}
	for _, id := range ids {
		b, ok := byID(id)
		if !ok {
			fails[id] = "неизвестный браузер"
			continue
		}
		if pkg == "" {
			fails[id] = "пакет расширения не найден"
			continue
		}
		if err := write(b, pkg, ver); err != nil {
			fails[id] = err.Error()
		}
	}
	return fails
}

func write(b Browser, pkg, ver string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, regKey(b), registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetStringValue("path", pkg); err != nil {
		return err
	}
	return k.SetStringValue("version", ver)
}

// Uninstall — снимает регистрацию расширения в выбранных браузерах
// (браузер уберёт его при следующем старте).
func Uninstall(ids []string) map[string]string {
	fails := map[string]string{}
	for _, id := range ids {
		b, ok := byID(id)
		if !ok {
			fails[id] = "неизвестный браузер"
			continue
		}
		if err := registry.DeleteKey(registry.CURRENT_USER, regKey(b)); err != nil && !errors.Is(err, registry.ErrNotExist) {
			fails[id] = err.Error()
		}
	}
	return fails
}

// OpenExtensionsPage — открывает в браузере страницу расширений на карточке
// ZPUI: там пользователь включает расширение (один раз после установки).
func OpenExtensionsPage(id string) error {
	b, ok := byID(id)
	if !ok {
		return fmt.Errorf("неизвестный браузер: %s", id)
	}
	exe := b.findExe()
	if exe == "" {
		return fmt.Errorf("%s не найден", b.Name)
	}
	return executil.DetachedCmd(exe, fmt.Sprintf("%s://extensions/?id=%s", b.Scope, ID)).Start()
}

// WritePackage — атомарная замена пакета приложения при обновлении с сервера.
func WritePackage(exeDir string, data []byte) error {
	return WritePackageTo(filepath.Join(exeDir, "components", "extension"), data)
}

// WritePackageTo — атомарная замена пакета в произвольном каталоге
// (standalone-установщик пишет его в %LOCALAPPDATA%\ZPUI\extension).
func WritePackageTo(dir string, data []byte) error {
	if v, err := ReadVersionBytes(data); err != nil || v == "" {
		return errors.New("скачанный файл — не CRX-пакет расширения")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(dir, File+".part")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(dir, File)); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// WriteLocalPackage — атомарная запись standalone-пакета.
func WriteLocalPackage(data []byte) error {
	return WritePackageTo(LocalDir(), data)
}

// ReadVersionBytes — версия из manifest.json внутри CRX-пакета в памяти.
func ReadVersionBytes(data []byte) (string, error) {
	if len(data) < 12 || string(data[:4]) != "Cr24" {
		return "", errors.New("не CRX-пакет")
	}
	off := 12 + int(binary.LittleEndian.Uint32(data[8:12]))
	if off >= len(data) {
		return "", errors.New("битый CRX-заголовок")
	}
	zr, err := zip.NewReader(bytes.NewReader(data[off:]), int64(len(data)-off))
	if err != nil {
		return "", err
	}
	for _, f := range zr.File {
		if f.Name != "manifest.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()
		var m struct {
			Version string `json:"version"`
		}
		if err := json.NewDecoder(rc).Decode(&m); err != nil {
			return "", err
		}
		return m.Version, nil
	}
	return "", errors.New("в пакете нет manifest.json")
}
