package main

import (
	"embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	winsys "golang.org/x/sys/windows"

	zpuiapp "zpui/internal/app"
	"zpui/internal/bgstore"
	"zpui/internal/config"
	"zpui/internal/database"
	"zpui/internal/executil"
	"zpui/internal/logger"
	"zpui/internal/monitor"
	"zpui/internal/power"
	"zpui/internal/singleinstance"
	"zpui/internal/tray"
	"zpui/internal/xboxdns"
	"zpui/internal/zapret"
	"zpui/internal/zsvc"
)

// earlyLog writes a timestamped message to logs/boot.log in the app directory.
// Falls back to %TEMP%\zpui-boot.log if the app dir is not writable (e.g. non-admin).
func earlyLog(exeDir, msg string) {
	ts := time.Now().Format("2006-01-02 15:04:05.000")
	line := fmt.Sprintf("[%s] %s\n", ts, msg)
	fmt.Fprint(os.Stderr, line)

	logPath := filepath.Join(exeDir, "logs", "boot.log")
	if f, err := earlyLogWrite(logPath, line); err == nil {
		f.Close()
		return
	}

	tmpDir := os.TempDir()
	tmpPath := filepath.Join(tmpDir, "zpui-boot.log")
	if f, err := earlyLogWrite(tmpPath, line); err == nil {
		f.Close()
	}
}

func earlyLogWrite(path, line string) (*os.File, error) {
	os.MkdirAll(filepath.Dir(path), 0755)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	fmt.Fprint(f, line)
	f.Sync()
	return f, nil
}

//go:embed all:web/dist
var webFS embed.FS

//go:embed build/windows/icon.ico
var trayIcon []byte

var version = "1.0.0"

// Отладочные флаги запуска:
//
//	ZPUI.exe -devmode                     — окно DevTools + порт 9223 + консоль + debug-логи
//	ZPUI.exe -devtools                    — DevTools фронтенда на 127.0.0.1:9223 (порт, без окна)
//	ZPUI.exe -console                     — консоль с живыми логами бэкенда
//	ZPUI.exe -zapret-autostart=false      — отключить автостарт движка
//	ZPUI.exe -debug-log                   — уровень логов DEBUG
var (
	flDevtools     = flag.Bool("devtools", false, "DevTools фронтенда: remote debugging на 127.0.0.1:9223")
	flDevtoolsPort = flag.Int("devtools-port", 9223, "порт remote debugging (с -devtools)")
	flConsole      = flag.Bool("console", false, "консоль с живыми логами бэкенда")
	flDebugLog     = flag.Bool("debug-log", false, "уровень логов DEBUG")
	flZapretStart  = flag.String("zapret-autostart", "", "переопределить автостарт движка: true|false")
	flDevmode      = flag.Bool("devmode", false, "режим отладки: devtools + console + debug-log")
	flDiag         = flag.Bool("diag", false, "диагностика окружения в консоль, без GUI (админ не нужен)")
	// Обязаны быть объявлены: необъявленный аргумент flag.Parse завершает
	// процесс с кодом 2 до инициализации логов (так был сломан автозапуск).
	flHidden       = flag.Bool("hidden", false, "запуск с окном в трее (задача автозапуска)")
	flTray         = flag.Bool("tray", false, "алиас -hidden")
	flRunStartTask = flag.Bool("run-start-task", false, "релей автозапуска: скрыто запустить задачу ZPUI-Start и выйти")
)

// runDiag — ZPUI.exe -diag: проверка окружения в консоль без GUI и без админа.
func runDiag() {
	out := os.Stdout
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	fmt.Fprintln(out, "=== ZPUI diag ===")
	fmt.Fprintf(out, "exe:          %s\n", exePath)
	fmt.Fprintf(out, "build:        v%s\n", version)

	configPath := filepath.Join(exeDir, "config.json")
	zapretDir := filepath.Join(exeDir, "zapret")
	dbDir := filepath.Join(exeDir, "databases")

	if _, err := os.Stat(configPath); err == nil {
		fmt.Fprintln(out, "config.json:  есть")
	} else {
		fmt.Fprintln(out, "config.json:  НЕТ (создастся при первом запуске)")
	}

	if fi, err := os.Stat(zapretDir); err == nil && fi.IsDir() {
		bats := 0
		if entries, err := os.ReadDir(zapretDir); err == nil {
			for _, e := range entries {
				if strings.HasSuffix(strings.ToLower(e.Name()), ".bat") {
					bats++
				}
			}
		}
		fmt.Fprintf(out, "zapret dir:   есть, стратегий (.bat): %d\n", bats)
	} else {
		fmt.Fprintln(out, "zapret dir:   ОТСУТСТВУЕТ — движку нечего запускать")
	}

	if fi, err := os.Stat(filepath.Join(dbDir, "zpui.db")); err == nil {
		fmt.Fprintf(out, "database:     %d байт\n", fi.Size())
	} else {
		fmt.Fprintln(out, "database:     НЕТ (создастся при первом запуске)")
	}

	ipv4 := []string{}
	if ifaces, err := net.Interfaces(); err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, _ := iface.Addrs()
			for _, a := range addrs {
				if ipnet, ok := a.(*net.IPNet); ok {
					if ip := ipnet.IP.To4(); ip != nil {
						ipv4 = append(ipv4, ip.String())
					}
				}
			}
		}
	}
	fmt.Fprintf(out, "IPv4 (UP):    %s\n", strings.Join(ipv4, ", "))
	if len(ipv4) == 0 {
		fmt.Fprintln(out, "  !! активных IPv4 нет — приложение покажет 127.0.0.1")
	}

	cmd := executil.HiddenCmd("net", "session")
	admin := cmd.Run() == nil
	fmt.Fprintf(out, "admin:        %v (движку и DNS:53 нужен)\n", admin)

	serverURL := zpuiapp.ServerURLValue()
	fmt.Fprintf(out, "server:       %s\n", serverURL)
	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Get(strings.TrimRight(serverURL, "/") + "/healthz")
	if err != nil {
		fmt.Fprintf(out, "server ping:  FAIL: %v\n", err)
	} else {
		resp.Body.Close()
		fmt.Fprintf(out, "server ping:  OK (HTTP %d)\n", resp.StatusCode)
	}

	fmt.Fprintln(out, "=== diag done ===")
}

// runStartTask — релей Run-записи автозапуска: GUI-exe вместо schtasks.exe,
// чтобы при входе в Windows не мелькало окно терминала. Без повышения прав,
// без одиночного экземпляра и без GUI: дернули задачу и вышли.
func runStartTask() {
	executil.HiddenCmd("schtasks", "/Run", "/TN", zsvc.StartTaskName).Run()
	os.Exit(0)
}

func applyDebugFlags() {
	flag.Parse()
	if *flDiag {
		runDiag()
		os.Exit(0)
	}
	if *flRunStartTask {
		runStartTask()
	}
	if *flDevmode {
		*flDevtools = true
		*flConsole = true
		*flDebugLog = true
	}
	// JS-таймеры (опрос кода входа в AuthModal каждые 2 с) не должны
	// замораживаться в свёрнутом/фоновом окне — иначе вход через код не
	// завершается, пока приложение на переднем плане.
	backgroundArgs := "--disable-background-timer-throttling --disable-backgrounding-occluded-windows --disable-features=IntensiveWakeUpThrottling"
	if prev := os.Getenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS"); prev != "" {
		backgroundArgs = prev + " " + backgroundArgs
	}
	os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", backgroundArgs)
	if *flDevtools {
		// WebView2 читает переменную при создании браузерного процесса.
		args := fmt.Sprintf("--remote-debugging-port=%d", *flDevtoolsPort)
		if *flDevmode {
			args += " --auto-open-devtools-for-tabs"
		}
		os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", backgroundArgs+" "+args)
	}
	if *flZapretStart == "true" || *flZapretStart == "false" {
		zpuiapp.SetDevNoZapretAutostart(*flZapretStart == "false")
	}
	if *flConsole {
		attachConsole()
	}
}

// attachConsole — GUI-приложение без консоли; создаём свою и переносим в неё
// stdout/stderr, чтобы ранние логи и хвост файлового журнала были видны живьём.
func attachConsole() {
	alloc := winsys.NewLazySystemDLL("kernel32.dll").NewProc("AllocConsole")
	alloc.Call()
	out, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0)
	if err == nil {
		os.Stdout = out
		os.Stderr = out
	}
	fmt.Fprintln(os.Stdout, "[ZPUI] консоль бэкенда (закроется вместе с приложением)")
}

// tailLogsForConsole — после инициализации logMgr печатает новые строки
// файлового журнала в отладочную консоль.
func tailLogsForConsole(logMgr *logger.Logger, logsDir string) {
	if !*flConsole {
		return
	}
	_ = logMgr
	go func() {
		var path string
		var offset int64
		for {
			files, _ := filepath.Glob(filepath.Join(logsDir, "*.log"))
			if len(files) > 0 {
				newest := files[0]
				for _, f := range files {
					if fi, err := os.Stat(f); err == nil {
						if nfi, err2 := os.Stat(newest); err2 != nil || fi.ModTime().After(nfi.ModTime()) {
							newest = f
						}
					}
				}
				if path != newest {
					path = newest
					offset = 0
				}
				if f, err := os.Open(path); err == nil {
					f.Seek(offset, io.SeekStart)
					buf := make([]byte, 8192)
					n, _ := f.Read(buf)
					f.Close()
					if n > 0 {
						offset += int64(n)
						fmt.Fprint(os.Stdout, string(buf[:n]))
					}
				}
			}
			time.Sleep(600 * time.Millisecond)
		}
	}()
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			exe, _ := os.Executable()
			earlyLog(filepath.Dir(exe), fmt.Sprintf("FATAL PANIC: %v\n%s", r, debug.Stack()))
			panic(r)
		}
	}()

	applyDebugFlags()

	// Генерация биндингов wails: перечисляем методы App и выходим, без
	// повышения прав и тяжёлого старта (см. bindings_tag_on.go).
	if wailsBindingsMode {
		runBindingsGeneration()
		return
	}

	executil.SetProcessAppID("ZPUI")

	exePath, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	exeDir := filepath.Dir(exePath)

	earlyLog(exeDir, "ZPUI starting...")

	if !ensureAdmin() {
		earlyLog(exeDir, "ensureAdmin failed, exiting")
		return
	}

	// Проверка единственного экземпляра
	if cleanup, err := singleinstance.Check(exePath); err != nil {
		earlyLog(exeDir, "singleinstance check failed: "+err.Error())
		fmt.Println("ZPUI:", err)
		return
	} else if cleanup != nil {
		defer cleanup()
	}

	configPath := filepath.Join(exeDir, "config.json")
	zapretDir := filepath.Join(exeDir, "zapret")
	dbDir := filepath.Join(exeDir, "databases")

	if err := os.MkdirAll(dbDir, 0755); err != nil {
		earlyLog(exeDir, "mkdir databases failed: "+err.Error())
		log.Fatalf("Failed to create databases directory: %v", err)
	}

	cfg := config.Load(configPath, zapretDir)
	cfg.SetZapretPath(zapretDir)
	cfg.ModVersion = version
	power.Set(cfg.PowerSaving, cfg.PowerLevel, cfg.PowerThreshold)

	logMgr, err := logger.New(cfg.LogsDir(), dbDir)
	if err != nil {
		earlyLog(exeDir, "logger.New failed: "+err.Error())
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logMgr.Close()

	startHidden := *flHidden || *flTray
	mode := "Wails GUI"
	if startHidden {
		mode = "Wails GUI, hidden (--hidden)"
	}
	logMgr.Info("main", fmt.Sprintf("ZPUI v%s starting (%s)...", version, mode))
	logMgr.Info("main", fmt.Sprintf("Zapret dir: %s", cfg.ZapretPath))
	logMgr.Info("main", fmt.Sprintf("Go %s %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH))

	tailLogsForConsole(logMgr, cfg.LogsDir())

	// Ранние строки: старт этого запуска до готовности логгера и хвост
	// аварийно завершившегося прошлого запуска (раньше стирались ротацией
	// boot.log — причины незапуска автозапуска терялись безвозвратно).
	for _, line := range logMgr.BootPrefix() {
		logMgr.Info("boot", line)
	}

	dbPath := filepath.Join(dbDir, "zpui.db")
	if err := database.Init(dbPath); err != nil {
		logMgr.Error("main", "database.Init failed: "+err.Error())
		earlyLog(exeDir, "database.Init failed: "+err.Error())
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()
	logMgr.Info("main", "Database initialized")

	zapretMgr := zapret.NewManager(cfg, logMgr)
	trafficMonitor := monitor.NewTrafficMonitor(logMgr)
	xboxDnsMgr := xboxdns.NewManager(logMgr)

	// Создаём Wails-приложение
	app := zpuiapp.NewApp(cfg, logMgr, zapretMgr, trafficMonitor, xboxDnsMgr, version, exeDir)

	// Окно скрыто ТОЛЬКО при явном флаге --hidden/--tray (задача автозапуска
	// Windows передаёт его, если включена настройка «Запускать свёрнутым»).
	// Ручной запуск всегда открывает окно.
	app.SetStartHidden(startHidden)

	// Создаём tray (контроллер = app, управляет окном через Wails runtime)
	trayApp := tray.New(cfg, logMgr, zapretMgr, xboxDnsMgr, app, version, trayIcon)
	go func() {
		runtime.LockOSThread()
		defer func() {
			if r := recover(); r != nil {
				logMgr.Error("tray", fmt.Sprintf("Tray panic: %v\n%s", r, debug.Stack()))
			}
		}()
		trayApp.Run()
	}()

	// Готовим embedded assets для Wails
	distFS, err := fs.Sub(webFS, "web/dist")
	if err != nil {
		log.Fatalf("Failed to create dist sub-FS: %v", err)
	}

	// Запускаем Wails (блокирует)
	winW, winH := defaultWindowSize()
	logMgr.Info("main", "Starting Wails application...")
	err = wails.Run(&options.App{
		Title:     "ZPUI",
		Width:     winW,
		Height:    winH,
		MinWidth:  900,
		MinHeight: 650,
		// В dev-сборке (wails dev) инспектор открывается сам; в prod опция
		// игнорируется, там DevTools включают флагами -devmode/-devtools.
		Debug: options.Debug{OpenInspectorOnStartup: true},
		// Безрамочный режим: единственный тайтлбар — собственная панель
		// приложения (нативный заголовок Windows давал двойной тайтлбар).
		Frameless: true,
		// Фон WebView2 в цвет тёмной темы: при ресайзе не мигает белым.
		BackgroundColour: &options.RGBA{R: 0x14, G: 0x14, B: 0x18, A: 0xff},
		AssetServer: &assetserver.Options{
			Assets: distFS,
			// Фоновые картинки кастомных тем: /api/bg/* и /bg/<файл> из <dataDir>/bg.
			Handler: bgstore.Handler(filepath.Dir(configPath)),
			Middleware: func(next http.Handler) http.Handler {
				// Запрещаем кэширование: WebView2 иначе кэширует index.html,
				// и после обновления грузит старый бандл (серый экран).
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Cache-Control", "no-store, must-revalidate")
					next.ServeHTTP(w, r)
				})
			},
		},
		OnStartup:     app.Startup,
		OnShutdown:    app.Shutdown,
		OnBeforeClose: app.BeforeClose,
		StartHidden:   startHidden,
		Bind:          []interface{}{app},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})

	if err != nil {
		logMgr.Error("main", fmt.Sprintf("Wails error: %v", err))
		log.Fatal(err)
	}
}

func ensureAdmin() bool {
	cmd := executil.HiddenCmd("net", "session")
	if cmd.Run() == nil {
		return true
	}

	exe, _ := os.Executable()
	// Пробрасываем ВСЕ стартовые флаги (в т.ч. отладочные -devmode/-devtools
	// и --hidden/--tray из автозапуска), чтобы повторно запущенный с
	// повышением процесс не терял режим запуска.
	argList := ""
	if len(os.Args) > 1 {
		quoted := make([]string, len(os.Args[1:]))
		for i, a := range os.Args[1:] {
			quoted[i] = "'" + a + "'"
		}
		argList = " -ArgumentList " + strings.Join(quoted, ",")
	}
	cmd = executil.HiddenCmd("powershell", "-NoProfile", "-Command",
		fmt.Sprintf("Start-Process '\"%s\"' -Verb RunAs%s", exe, argList))
	cmd.Run()
	return false
}
