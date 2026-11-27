package app

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/blockcheck"
	"zpui/internal/config"
	"zpui/internal/executil"
	"zpui/internal/isp"
	"zpui/internal/logger"
	"zpui/internal/mcpserver"
	"zpui/internal/monitor"
	"zpui/internal/notify"
	"zpui/internal/pkginst"
	"zpui/internal/proxy"
	"zpui/internal/syncclient"
	"zpui/internal/tgproxy"
	"zpui/internal/themes"
	"zpui/internal/xboxdns"
	"zpui/internal/zapret"
	"zpui/internal/zsvc"
)

// BuildNum — номер сборки (ldflags -X zpui/internal/app.BuildNum из
// build.bat, счётчик build\buildnumber.txt). Пусто — собрано без build.bat.
var BuildNum string

// App — главный контекст Wails-приложения.
// Все экспортируемые методы автоматически привязываются к frontend через Wails bindings.
type App struct {
	ctx     context.Context
	ctxMu   sync.RWMutex
	cfg     *config.Config
	log     *logger.Logger
	zapret  *zapret.Manager
	tgproxy *tgproxy.Manager
	socks   *proxy.SOCKS5Server
	mcp     *mcpserver.Server
	monitor *monitor.TrafficMonitor
	xboxDns *xboxdns.Manager
	version string
	exeDir  string

	// Канал готовности контекста (для tray, который ждёт пока Wails запустится)
	ctxReady chan struct{}

	// opGen — поколение операций с движком: деструктивные действия (стоп,
	// удаление службы, полная перестановка) инкрементируют счётчик, и любые
	// идущие попытки запуска/восстановления, начатые раньше, отменяются.
	opGen atomic.Int64

	// singleTestRunning/singleTestResult — состояние проверки одной стратегии
	// (ПКМ на карточке): фронтенд опрашивает GetSingleTestResult.
	singleTestMu      sync.Mutex
	singleTestRunning bool
	singleTestResult  map[string]interface{}

	// Отмена фазы быстрого свипа подбора (KeySweepNames): движковый флаг
	// отмены в этой фазе ещё не поднят — CancelAutoTest гасит свип отсюда.
	sweepMu     sync.Mutex
	sweepCancel context.CancelFunc

	// Отмена всего прогона подбора (prep/свип/финал/хвост): CancelAutoTest
	// гасит runCtx горутины RunAutoSelectStream — паттерн как у sweepCancel.
	selectionMu     sync.Mutex
	selectionCancel context.CancelFunc

	// selectionRunning — единый флаг «идёт подбор» на prep+свип+финал+5с-хвост:
	// автосмена, хук проверки после смены стратегии, health-recovery и смена
	// оператора на него гвардятся. Стартовый quick-путь и ПКМ его не ставят.
	selectionRunning atomic.Bool

	// Автосмена по доступности: сколько тактов подряд ниже порога и когда
	// была последняя автосмена (cooldown). История — logs/auto_switch_history.json.
	autoSwitchMu         sync.Mutex
	autoSwitchArmed      bool // лёгкий старт: серия возможна после готовности UI + 1 мин
	autoSwitchConfirming bool // идёт серия подтверждения (3×30с, вне графика)
	autoSwitchLastAt     time.Time

	// Троттл автоматических реакций на смену оператора (TTL 30 мин на ключ):
	// детект на пограничных сетях мигает между операторами — уведомление
	// пользователю и фоновый автоподбор не должны повторяться на каждом такте.
	opNotifyGate  opEventGate
	opAutoSelGate opEventGate

	// zapretExpected — «движок должен работать»: пока true, health-монитор
	// восстанавливает Запрет после любого незапланированного падения.
	// В false выставляется только явными действиями пользователя (стоп,
	// удаление службы, полная перестановка).
	zapretExpected atomic.Bool

	// lastRecoveryFail — unix-время последнего неудачного восстановления:
	// пауза перед повторной автоматической попыткой.
	lastRecoveryFail atomic.Int64

	// svcHintInstalled/svcHintAt — кэш факта «служба winws установлена» для
	// мгновенной отрисовки панели службы в настройках: настоящий чек в SCM
	// не чаще раза в 10 секунд, в остальное время отдаётся последнее значение.
	svcHintInstalled atomic.Bool
	svcHintAt        atomic.Int64

	// Кэш дорогих системных фактов для /api/status и /api/config: реестр DNS
	// адаптера, автозапуск, сетевые факты — настоящий опрос не чаще TTL.
	sysCacheMu       sync.Mutex
	sysXboxApplied   bool
	sysXboxAppliedAt time.Time
	sysAutostart     bool
	sysAutostartAt   time.Time
	sysIPs           []string
	sysMAC           string
	sysHostname      string
	sysNetAt         time.Time

	// DNS: применение тумблеров асинхронное — биндинг отвечает сразу,
	// работа (пробы + netsh) идёт в горутине; итог — событие dns:apply.
	dnsApplyMu    sync.Mutex
	dnsApplying   atomic.Bool
	dnsApplyScope atomic.Value

	// stopCh закрывается в shutdown() — сигнал горутинам завершиться
	stopCh chan struct{}

	// shutdownDone закрывается после выполнения shutdown() — для Quit()
	shutdownDone chan struct{}

	// once защищает shutdown()/Quit() от повторного выполнения (panic на close)
	shutdownOnce sync.Once
	quitOnce     sync.Once

	// CloseApp из трея: приложение закрывается, но движок запрета (режим
	// службы) оставляется работать — Shutdown пропускает гашение winws.
	keepEngineOnQuit atomic.Bool

	// Кэш доступности ресурсов (для tray)
	resourceCache     *blockcheck.BulkReport
	resourceCacheTime time.Time
	resourceCacheMu   sync.Mutex

	// checkingNow = true when a resource check is in progress (auto or manual).
	// Frontend uses this to disable button and show "updating...".
	checkingNow   bool
	checkingNowMu sync.Mutex

	// Гейт готовности для стартового экрана: закрывается, когда фоновый старт
	// (служба, автозапуск) и первый такт проверки ресурсов завершены —
	// до этого UI не показывается, вся тяжёлая работа идёт за стартовым экраном.
	startupReadyMu sync.Mutex
	startupReady   bool
	startupSvcOK   bool
	startupResOK   bool
	startupReadyCh chan struct{}
	svcInitDone    chan struct{}
	svcInitOnce    sync.Once
	resInitDone    chan struct{}
	resInitOnce    sync.Once

	// Стартовый подбор (mode=startup): заявка фронта и канал завершения.
	// Первый такт проверки ресурсов ждёт его — замер должен идти под
	// проверенной и применённой стратегией, а не под промежуточной перебора.
	startupSelectRequested atomic.Bool
	startupSelectOnce      sync.Once
	startupSelectDone      chan struct{}

	// prevResourceState tracks previous OK/!OK state per resource name.
	// Used to log only state CHANGES (not every failed check).
	prevResourceState   map[string]bool
	prevResourceStateMu sync.Mutex

	// dnsFailStreak — подряд идущие неудачные циклы здоровья DNS-провайдеров
	// (2 подряд — авто-выключение DNS); атомарно: циклы могут перекрываться.
	dnsFailStreak atomic.Int32

	// nxStreak — подряд идущие NXDOMAIN-сэмплы на хост; после 3 подряд хост
	// авто-исключается из проверок как мёртвый (см. trackNXDomains).
	nxStreakMu sync.Mutex
	nxStreak   map[string]int

	// SOCKS-статика: подсеть, на которой статика применена нами
	// (снимается в DHCP при смене сети). Сама статика включается
	// только вручную.
	appliedStaticMu     sync.Mutex
	appliedStaticSubnet string

	// DNS-источники: сколько циклов проверки подряд источник мёртв
	// (2 подряд — авто-выключение источника).
	dnsDeadMu     sync.Mutex
	dnsDeadStreak map[string]int

	// Видимость окна (для tray toggle)
	windowVisible bool
	windowMu      sync.Mutex

	// startHidden — окно запускается скрытым (start_minimized или флаг --hidden)
	startHidden bool

	// ispMonitor следит за текущим оператором связи
	ispMonitor *isp.Monitor

	// themes — хранилище кастомных тем (themes.json рядом с config.json)
	themes *themes.Store

	// startupLog — лог запуска приложения (logs/startup-*.log)
	startupLog *StartupLogger

	// sup — реестр супервизора фоновых модулей (app_supervisor.go):
	// статусы, рестарты после паник, лимит crash-loop'а.
	supMu sync.Mutex
	sup   *moduleRegistry

	// svc — IPC-клиент службы ZPUI (app_service.go): hello/exit-уведомления,
	// тумблеры автозапуска. Все вызовы best-effort с таймаутом.
	svc *zsvc.Client

	// syncRunner — контур ZPUI Sync (app_sync.go): nil до старта канала.
	syncRunnerMu sync.Mutex
	syncRunner   *syncclient.Runner

	// bootAt — момент старта процесса (uptime в data.status).
	bootAt time.Time

	// zapretUpAt — с какого момента движок считается непрерывно работающим
	// (uptime движка в data.runtime; 0 = не работает).
	zapretUpAt atomic.Int64
}

// NewApp создаёт новый экземпляр приложения.

func NewApp(
	cfg *config.Config,
	logMgr *logger.Logger,
	zapretMgr *zapret.Manager,
	trafficMon *monitor.TrafficMonitor,
	xboxDnsMgr *xboxdns.Manager,
	version string,
	exeDir string,
) *App {
	notify.SetLogger(logMgr)
	a := &App{
		cfg:               cfg,
		log:               logMgr,
		zapret:            zapretMgr,
		tgproxy:           tgproxy.NewManager(cfg, logMgr),
		socks:             proxy.NewSOCKS5(cfg, logMgr),
		monitor:           trafficMon,
		xboxDns:           xboxDnsMgr,
		version:           version,
		exeDir:            exeDir,
		ctxReady:          make(chan struct{}),
		stopCh:            make(chan struct{}),
		shutdownDone:      make(chan struct{}),
		startupReadyCh:    make(chan struct{}),
		svcInitDone:       make(chan struct{}),
		resInitDone:       make(chan struct{}),
		startupSelectDone: make(chan struct{}),
		svc:               zsvc.NewClient(),
		themes:            themes.Load(filepath.Join(cfg.DataDir(), "themes.json")),
		bootAt:            time.Now(),
	}
	themes.EnsurePreset(a.themes, cfg.DataDir(), logMgr)
	// Стратегия применена вне автотеста (ручная смена, установка, восстановление)
	// — внеплановая проверка доступности: дашборд отражает новую стратегию
	// без ожидания планового такта.
	zapretMgr.SetOnStrategyApplied(func(string) {
		a.scheduleResourceCheckAfterStrategyChange()
	})
	// Автоподбор чинит DNS-подмену YouTube через этот хук (включает Xbox DNS).
	zapretMgr.SetYouTubeFixHook(a.enableXboxDNSForAutoselect)
	// Зеркало запрета: при недоступности GitHub версия и архив качаются
	// с сервера управления.
	zapretMgr.SetServerURLFn(func() string { return ServerURL })
	return a
}

// SetStartHidden управляет скрытым запуском окна (вызывается из main.go).

func (a *App) SetStartHidden(v bool) { a.startHidden = v }

// setCtx/getContext — гвард поля ctx: горутина трея стартует до wails.Run и

// читает его до и после записи в Startup.

func (a *App) setCtx(ctx context.Context) {
	a.ctxMu.Lock()
	a.ctx = ctx
	a.ctxMu.Unlock()
}

func (a *App) getContext() context.Context {
	a.ctxMu.RLock()
	defer a.ctxMu.RUnlock()
	return a.ctx
}

// emitSafe безопасно шлёт Wails-событие (если контекст готов).

func (a *App) emitSafe(name string, data interface{}) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, name, data)
	}
}

// startup вызывается Wails при запуске приложения.

func (a *App) Startup(ctx context.Context) {
	a.setCtx(ctx)
	a.log.Info("app", "Wails application started")

	sl, _ := NewStartupLogger(a.cfg.LogsDir())
	if sl != nil {
		sl.Write("BOOT", "Wails application started")
		sl.Writef("BOOT", "Version: %s", a.version)
		if BuildNum != "" {
			sl.Writef("BOOT", "Build: #%s", BuildNum)
		}
		sl.Writef("BOOT", "ExeDir: %s", a.exeDir)
		sl.Write("BOOT", "Загрузка конфигурации...")
	}
	a.startupLog = sl
	if BuildNum != "" {
		a.log.Info("app", "Build: #"+BuildNum)
	}

	// Синхронизируем флаг видимости окна с реальным стартовым состоянием
	a.windowMu.Lock()
	a.windowVisible = !a.startHidden
	a.windowMu.Unlock()

	// Старт с окном в трее (--hidden): фронт не знает о скрытом окне — глушим
	// его тики/анимации событием после подписки фронтенда.
	if a.startHidden {
		go func() {
			time.Sleep(1500 * time.Millisecond)
			if !a.WindowShown() {
				a.emitSafe("window:hidden", nil)
			}
		}()
	}

	// Initialize resource state map for change tracking
	a.prevResourceState = make(map[string]bool)

	// Ensure skip-resources.txt exists (shipped with app, but auto-create if missing).
	a.ensureSkipResourcesFile()
	a.ensureSkipResourcesSystemFile()
	// секционный формат: одна шапка авто-исключений вместо «# auto: …»
	// перед каждым доменом, дубликаты хостов убираются
	a.cfg.NormalizeSkipFile()

	// Flush boot log into the log database (captures startup crashes).
	a.log.FlushBootLog()

	// Hook logger errors to desktop notifications (if enabled by user).
	a.log.SetOnError(func(code, msg string) {
		if a.cfg.ShouldNotify("errors") {
			notify.Show("ZPUI \xd0\xbe\xd1\x88\xd0\xb8\xd0\xb1\xd0\xba\xd0\xb0", msg)
		}
	})

	// Hook zapret crash: уведомляем пользователя сразу и запускаем
	// восстановление службы. Восстановление НЕ отключается никогда: пока
	// движок «должен работать» (не остановлен пользователем явно), после
	// краша он поднимается снова; пауза между попытками растёт до 60 секунд.
	var crashMu sync.Mutex
	var crashWindow []time.Time
	var lastCrashNotify time.Time
	a.zapret.OnCrash = func() {
		count := a.cfg.GetServiceCrashCount() + 1
		a.cfg.SetServiceCrashCount(count)
		a.cfg.SetServiceLastCrash(time.Now().Unix())
		a.log.Warn("zapret", fmt.Sprintf("engine crashed (%d)", count))

		if !a.zapretExpected.Load() {
			a.log.Info("zapret", "crash ignored: engine was not expected to run")
			return
		}

		crashMu.Lock()
		now := time.Now()
		kept := crashWindow[:0]
		for _, ts := range crashWindow {
			if now.Sub(ts) < 60*time.Second {
				kept = append(kept, ts)
			}
		}
		crashWindow = append(kept, now)
		recent := len(crashWindow)
		// Троттл: не чаще раза в 2 минуты — серия мгновенных смертей движка
		// (перезапуск на незакрытом WinDivert) не должна превращаться в спам
		// уведомлений; recovery выполняется на каждый краш независимо.
		canNotify := now.Sub(lastCrashNotify) >= 2*time.Minute
		if canNotify {
			lastCrashNotify = now
		}
		crashMu.Unlock()

		// Уведомление о падении — сразу, но не чаще раза в 2 минуты.
		if canNotify {
			notify.Show("Запрет", "Движок неожиданно упал — восстанавливаем…")
		}

		// Backoff по числу недавних падений: 1s, 2s, 4s … 60s (cap).
		backoff := time.Duration(1<<uint(recent-1)) * time.Second
		if backoff > 60*time.Second {
			backoff = 60 * time.Second
		}
		a.log.Warn("zapret", fmt.Sprintf("waiting %v before recovery (recent crash #%d)", backoff, recent))
		time.Sleep(backoff)
		// Восстановление (с модальным окном и полной диагностикой) запускается
		// единым механизмом startZapretRecovery — он перезапустит службу и при
		// успехе сбросит счётчик крашей.
		a.startZapretRecovery(fmt.Sprintf("process_crash_%d", count))
	}

	close(a.ctxReady)

	// Реестр супервизора фоновых модулей — до запуска любых supervisedGo.
	a.supRegistry()

	a.supervisedGo("startup-cleanup", a.runStartupCleanup)
	// Свой мусор прошлых обновлений (%TEMP%\ZPUI-Setup-*.exe, zpui-user-* и
	// пр. + легаси в корне установки) — отложенно, чтобы не конкурировать со
	// стартовыми загрузками и не задерживать запуск.
	a.safeGo(func() {
		time.Sleep(2 * time.Minute)
		freed, removed, skipped := pkginst.CleanupUpdateLeftovers(a.exeDir)
		a.log.Info("cleanup", fmt.Sprintf("removed %d own temp files, freed %s, skipped %d",
			removed, fmtBytesRu(freed), skipped))
	})
	// Автозапуск (миграция схемы, запросы к schtasks — секунды) и служба ZPUI:
	// в фоне и ПОСЛЕДОВАТЕЛЬНО (hello службе передаёт актуальное состояние
	// автозапуска), чтобы IPC/SCM/schtasks не задерживали отзывчивость окна.
	a.supervisedGo("autostart-service", func() {
		a.syncAutostartSystem()
		a.svcStartup()
		a.svcInitOnce.Do(func() { close(a.svcInitDone) })
	})

	// Recovery: если прошлое обновление zapret прервалось (крах) — восстановить.
	needAutoStart := a.checkAndRecoverZapret()

	// «Движок должен работать» — стартовая точка из персистентного состояния:
	// если пользователь вручную остановил движок перед выходом/стопом, флаг
	// остаётся false, и health-монитор/стартовые проверки его не поднимают.
	a.zapretExpected.Store(needAutoStart || a.cfg.GetLastZapretState())

	a.supervisedGo("monitor", a.startDeviceTracker)
	a.supervisedGo("traffic", a.startTrafficSnapshots)
	a.supervisedGo("rotation", a.startDataRotation)
	a.supervisedGo("metrics", a.startMetricsCollector)
	a.supervisedGo("config-watchdog", a.watchConfigSaves)
	a.supervisedGo("resources", a.startResourceMonitor)
	a.supervisedGo("zapret-health", a.startZapretHealthMonitor)
	// Свежий архив запрета от установщика/обновлятора становится основным
	// только после того, как движок реально запустился: неудачное обновление
	// не должно затирать прошлую рабочую копию.
	a.supervisedGo("zapret-backup-promote", a.promoteZapretBackupLoop)
	a.supervisedGo("socks", a.startSocksProxy)
	a.supervisedGo("socks-subnets", a.startSocksSubnetWatch)
	// MCP-сервер управления ИИ-агентами: стартует только если включён
	// тумблером в модалке «MCP» (config.mcp.enabled).
	a.supervisedGo("mcp", a.startMcp)
	// Браузерный плагин идёт в дистрибутиве (components\extension): на
	// старте регистрируем его в найденных Chromium-браузерах сами —
	// после установки программы плагин «уже есть», без ручной установки.
	a.safeGo(a.autoInstallExtension)
	a.supervisedGo("server-ping", a.startServerPingLoop)
	// График «Трафик»: скорости копятся в бэкенде с самого старта — при
	// открытии монитора график уже с историей, а не с нуля.
	a.supervisedGo("traffic-history", a.trafficHistoryLoop)
	// Исключения доменов: системный/пользовательский файлы списков
	// существуют всегда; канал ZPUI Sync (секции данных + sync.pull со
	// списками/recommended/requests) стартует отдельным контуром.
	a.ensureExclusionFiles()
	a.ensureGoogleList()
	a.startSyncDataLoop()
	// Синхронизация устройств аккаунта: авто-пуш снапшота раз в сутки
	// (решение владельца 10.11.2026); перенос с другого устройства — модалка
	// после входа (SyncChoiceModal на фронте).
	a.safeGo(a.startDeviceSyncLoop)
	// Автосмена с «лёгким стартом»: активируется после готовности UI + 1 мин.
	a.safeGo(a.startAutoSwitchArm)

	a.startISPMonitor()
	a.checkZapretUpdateFlag()
	a.migrateIpsetMode()
	a.safeGo(a.syncStrategiesForOperator)
	a.watchStartupReady()

	// Модуль синхронизации между экземплярами ZPUI в ЛВС выведен из продукта:
	// не стартует (вкладка удалена), API сохранён на случай возврата.

	startedViaAutostart := a.startHidden
	// Автозапуск прокси/DNS управляется только тумблерами в настройках
	// (по умолчанию выключены) и работает при любом запуске приложения.
	// Мост теперь ТОЛЬКО дочерним компонентом: отсутствие exe — фича мертва,
	// пишем это в журнал сразу при старте, без ожидания включения тумблера.
	if !a.tgproxy.ComponentInstalled() {
		a.log.Warn("tgproxy", "bridge component not installed (components/tgproxy/ZPUI-TgProxy.exe) - Telegram bridge unavailable, reinstall the app")
		notify.Show("ZPUI", tr(a.cfg.GetLanguage(), "tgproxy_missing"))
	}
	var autoStartProxy, lastZapretState bool
	a.cfg.RView(func() {
		autoStartProxy = a.cfg.AutoStartProxy
		lastZapretState = a.cfg.LastZapretState
	})
	if autoStartProxy {
		a.safeGo(func() {
			if r := a.TgProxyStart(); r != nil {
				if err, ok := r["error"].(string); ok {
					a.log.Error("tgproxy", "Auto-start failed: "+err)
				}
			}
		})
	}

	// Автозапуск запрета при старте через автозапуск (задача планировщика).
	if !devNoZapretAutostart && startedViaAutostart && lastZapretState && !needAutoStart && !a.cfg.GetZapretSkipped() {
		a.safeGo(func() {
			time.Sleep(1 * time.Second)
			// Автозапуск тоже верифицирует живость и при неудаче запускает
			// восстановление (с логированием и десктоп-уведомлением).
			if err := a.zapretStartAndVerify(); err != nil {
				a.log.Warn("zapret", "Auto-start zapret failed, recovering: "+err.Error())
				a.startZapretRecovery("autostart_failed: " + err.Error())
			}
		})
	}

	// DNS персистентен в системе (netsh сохраняется между перезагрузками),
	// поэтому автозапуска у него нет: активность определяется фактом —
	// прописан ли наш DNS на первичном адаптере. Перезапуск приложения DNS
	// всегда снимает (Shutdown), поэтому при включённом тумблере переприменяем
	// сразу. Плюс починка артефактов старых версий и проверка живости:
	// не отвечают — снимаем DNS и флаг.
	a.supervisedGo("xboxdns", func() {
		a.dnsStartupRepair()
		if a.cfg.GetXboxDnsConfig().Enabled && !a.xboxDns.IsSetInSystem() {
			a.dnsReapplyStartup()
		}
		a.dnsVerifyApplied()
		a.dnsSyncApplied()
		// Сторож: пока наш DNS включён — периодическая проверка живости и
		// сверка набора на первичном адаптере (недостающие наши — добавить,
		// лишние наши — убрать); перестал отвечать — принудительное отключение.
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-a.stopCh:
				return
			case <-t.C:
			}
			if !a.cfg.GetXboxDnsConfig().Enabled || !a.xboxDns.IsSetInSystem() {
				continue
			}
			a.dnsVerifyApplied()
			a.dnsSyncApplied()
		}
	})

	// Серверный набор алгоритма подбора: применяем сохранённый при старте
	// (до любых подборов/тестов), свежая версия доедет по sync.pull/startup.
	autoselectLoadApplied()

	if a.cfg.AutoUpdateCheck {
		a.safeGo(a.checkUpdatesOnStartup)
	}
	// Периодические проверки обновлений во время работы: цикл сам уважает
	// тумблер auto_update_check (тумблер включили на ходу — проверки начнутся
	// без перезапуска приложения).
	a.supervisedGo("update-check", a.startUpdateCheckLoop)
	// Мониторинг доступности сети: пропажа/восстановление интернета —
	// системные уведомления (проверка по нескольким независимым целям).
	a.StartNetWatch()

	// Авто-обновление ipset-списка (старт + раз в 24 часа); автоподбор при
	// запуске делает стартовый экран (см. этап auto-select в StartupScreen).
	a.supervisedGo("lists-auto", a.startListsAutoUpdate)
	// Персистентные списки запрета: установщик/обновлятор/переустановка могут
	// их снести — после ПОЛНОГО запуска возвращаем из копии (только отсутствующие
	// и заглушки; правки пользователя свежее копии не перезаписываются) и
	// освежаем копию живыми данными.
	a.safeGo(func() {
		time.Sleep(25 * time.Second)
		if restored := a.zapret.RestoreListsFromBackup(); len(restored) > 0 {
			a.emitStrategiesChanged()
		}
		a.zapret.RefreshListsBackup()
	})

	// server.url: адрес инжектится при сборке в ServerURL и подключения идут
	// всегда через него; пустое значение в config.json (баг старых версий)
	// не должно оставаться в UI — синхронизируем при старте.
	if a.cfg.GetServerConfig().URL != ServerURL {
		if err := a.cfg.UpdateServerConfig(func(s *config.ServerConfig) {
			s.URL = ServerURL
		}); err != nil {
			a.log.Warn("server", "persist server url: "+err.Error())
		} else {
			a.log.Info("server", "config server.url synced to build-time address")
		}
	}
}

// checkUpdatesOnStartup проверяет обновления через 10с после старта.

// startUpdateCheckLoop — те же проверки, что и на старте, но раз в час во

// время работы. Случайный сдвиг ±5 минут разводит клиентов по времени, чтобы

// они не били в сервер синхронно; в экономном режиме интервал растягивается

// в 5 раз. Повторные находки не спамят: тост дедуплицируется по версии

// (GetLastNotifiedVersion), флаги трея просто обновляются.

// runUpdateCheck — одна итерация проверки обновлений (ZPUI: сервер управления

// → Яндекс.Диск; zapret: GitHub → зеркало сервера), общая для старта и

// фонового цикла. Эмитит Wails event "update:available" если найдено

// обновление, выставляет флаги для трея и синхронизирует версии в БД.

// Уведомления дедуплицируются: тост показывается один раз на каждую новую

// версию, так что повторные проверки не спамят.

// scheduleResourceCheckAfterStrategyChange — проверка ресурсов после смены

// стратегии: движку нужно несколько секунд применить стратегию, потом сразу

// меряем доступность, чтобы дашборд отразил новую стратегию без ожидания

// планового такта. Повторные вызовы во время идущей проверки или подбора

// бездействуют (подбор проверяет ресурсы сам по завершении).

// watchStartupReady — закрывает гейт готовности, когда фоновый старт завершён:

// приветствие службы + первый такт проверки ресурсов (он же — стартовый замер

// Интернетметра). Потолок 2 минуты: недоступная служба/сеть не должны держать

// стартовый экран вечно. С этого момента фронтенду можно показывать UI.

// GetStartupReady — фронтенд-гейт стартового экрана: ready — можно открывать

// UI; svc/resources — какие этапы фонового старта уже завершены (фронтенд

// показывает, чего именно ждёт, пока ползёт финальный отрезок 90→100).

// startDnsWatchCycle — один цикл проверки DNS-серверов xbox-dns.ru. Вызывается

// из startResourceMonitor в ТОМ ЖЕ цикле, что и проверка доступности ресурсов

// (тот же настроенный период), сразу после неё. Снимок здоровья читает вкладка

// DNS; если серверы исчезли во время работы — DNS отключается сам

// (восстановление адресов) с уведомлением.

// doResourceCheckAndSave performs a full resource check (bypassing cache),

// saves snapshots to DB, and logs results. Used by both auto-monitor

// and manual refresh. О старте/завершении сообщается событием

// resource-check:started/done — дашборд обновляет панели сразу, без опроса.

// getResourceStatusForced does a force check (bypasses cache) and returns the raw report.

// IsCheckingNow returns whether a resource check is currently in progress.

// safeGo запускает функцию в горутине с защитой от panic.

// Panic логируется через Error и попадает в errors/ срез.

func (a *App) safeGo(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				a.log.Error("app", fmt.Sprintf("PANIC (goroutine): %v\n%s", r, debug.Stack()))
			}
		}()
		fn()
	}()
}

// checkAndRecoverZapret проверяет, есть ли в базе невостановленный слепок

// состояния zapret (признак прерванного обновления). Если zapret не работает —

// запускает восстановление в фоновой горутине и возвращает true (автозапуск не нужен).

// shutdown вызывается Wails при завершении приложения.

// Тайминги этапов — чтобы зависание завершения (например, после выхода

// из сна) было видно в журнале по шагам, а не как «процесс умер странно».

func (a *App) Shutdown(ctx context.Context) {
	a.shutdownOnce.Do(func() {
		total := time.Now()
		stage := total
		step := func(name string) {
			a.log.Info("app", fmt.Sprintf("Shutdown: %s (%s)", name, time.Since(stage).Round(time.Millisecond)))
			stage = time.Now()
		}

		a.log.Info("app", "Shutting down...")

		// Страховка Quit: при закрытии окна без close-to-tray Shutdown идёт
		// напрямую; повторный app-exiting для службы безвреден.
		a.svcNotifying("exiting")

		if a.startupLog != nil {
			a.startupLog.Close()
			a.startupLog = nil
		}
		step("startup log closed")

		close(a.stopCh)
		step("background loops stopped")

		if a.ispMonitor != nil {
			a.ispMonitor.Stop()
		}
		step("isp monitor stopped")

		if a.tgproxy != nil {
			a.tgproxy.Stop()
		}
		a.monitor.Stop()
		if a.mcp != nil {
			a.mcp.Stop()
		}
		step("tgproxy + monitor + mcp stopped")

		// Сначала восстанавливаем системный DNS через netsh/dnsapi (БЛОКИРУЮЩИЙ).
		// netsh-настройки персистентны — если не восстановить, система
		// останется с кастомным DNS после выхода.
		if a.xboxDns.IsEnabled() {
			a.log.Info("app", "Restoring system DNS before shutdown...")
			done := make(chan struct{}, 1)
			go func() {
				if err := a.xboxDns.Disable(); err != nil {
					a.log.Error("app", "xboxDns.Disable error: "+err.Error())
				} else {
					// Снимок устарел после восстановления — обновляем, иначе
					// следующий Enable/restore откатит систему к старым адресам.
					a.persistDnsPrevious()
				}
				close(done)
			}()
			select {
			case <-done:
				a.log.Info("app", "System DNS restored")
			case <-time.After(10 * time.Second):
				a.log.Error("app", "xboxDns.Disable timed out after 10s — network may be broken")
			}
			step("system DNS restored")
		}

		// «Закрыть приложение» из трея: в режиме службы движок самостоятелен —
		// оставляем winws работать. Полный выход и процессный режим гасят движок.
		if a.keepEngineOnQuit.Load() && a.HasSystemZapretService() {
			a.log.Info("app", "Shutdown: keepEngine — zapret service stays running")
		} else {
			go executil.HiddenCmd("taskkill", "/IM", "winws.exe", "/F").Run()
		}

		a.log.Info("app", fmt.Sprintf("Shutdown complete (total %s)", time.Since(total).Round(time.Millisecond)))
		close(a.shutdownDone)
	})
}

// beforeClose вызывается при закрытии окна (X).

// Если включён close-to-tray — сворачивает окно в трей вместо выхода.

// Для выхода используйте tray → Выход.

func (a *App) BeforeClose(ctx context.Context) bool {
	if a.cfg.GetCloseToTray() {
		a.log.Info("app", "Window close requested — hiding to tray")
		a.windowMu.Lock()
		a.windowVisible = false
		a.windowMu.Unlock()
		runtime.WindowHide(ctx)
		// Фронт глушит тики/анимации, пока окно скрыто (иначе GPU грузится
		// невидимым дашбордом).
		a.emitSafe("window:hidden", nil)
		return true
	}
	a.log.Info("app", "Window close requested — quitting")
	go a.Quit()
	return true
}

// Quit — принудительное завершение приложения (вызывается из tray и при закрытии окна).

// ShowWindow — показать окно (из tray) и сбросить страницу на главную.

// HideWindow — скрыть окно (из tray).

// ToggleWindow — переключить видимость окна (из tray).

// GetCachedResourcePercent — процент доступности ресурсов для tray.

// Restart перезапускает приложение (новый процесс, завершение текущего).

// Помощник ждёт завершения СТАРОГО процесса по PID и только затем стартует

// новый: фиксированный sleep раньше гонялся с медленным Quit, и перезапуск

// периодически упирался в single-instance мьютекс.

// Свернутое в трей приложение перезапускается свернутым (--hidden) — иначе

// после автозапуска и перезапуска окно разворачивалось само.

// WindowShown — видно ли окно сейчас.

// GetCachedUserPercent — процент доступности пользовательских ресурсов для tray.

// startTrafficSnapshots — периодическое сохранение снапшотов трафика.

// Каждый тик — запись в SQLite; в экономном режиме интервал растягивается.

// measureAndStoreNet — один замер качества сети (Интернетметр):

// запись в БД + pruning замеров старше часа.

// measureNetIfDue — замер Интернетметра выполняется ВНУТРИ цикла проверки

// ресурсов (вызов из doResourceCheckAndSave), поэтому в экономном режиме

// отдельно не глушится — следует за расписанием проверок. Свежий замер

// (<2 мин: ручные обновления подряд) не дублируем — полная проверка

// скорости стоит ~5 секунд параллельного трафика.

// startDataRotation — ротация старых данных (каждый час).

// ensureSkipResourcesFile creates skip-resources.txt next to config.json

// if it does not exist yet. The file is pre-populated with a list of

// known always-down resources. User can edit it manually afterwards.

// ensureSkipResourcesSystemFile создаёт skip-resources-system.txt рядом с

// skip-resources.txt, если его ещё нет (до первого синка с сервером).

// startZapretHealthMonitor периодически проверяет, жива ли служба zapret.

// OnCrash срабатывает только для дочернего процесса (режим прямого запуска);

// когда zapret работает как служба Windows и падает, OnCrash не вызывается.

// Этот монитор заполняет пробел: если служба была RUNNING, а стала STOPPED

// не через Stop() — запускает восстановление.

// startISPMonitor запускает фоновый мониторинг текущего оператора связи.

// При смене оператора пушит Wails-событие "operator:changed".

// syncStrategiesForOperator перечисляет все стратегии из папки zapret и

// синхронизирует их в БД для текущего оператора. Новые стратегии добавляются,

// существующие сохраняют свои данные. Текущая отмечается как активная.

// checkZapretUpdateFlag проверяет флаг zapret_just_updated в БД и пушит

// событие для показа модалки автоподбора.

func (a *App) ToggleProxy() error {
	if a.tgproxy.IsRunning() {
		a.tgproxy.Stop()
		return nil
	}
	return a.tgproxy.Start()
}

func (a *App) ProxyRunning() bool {
	return a.tgproxy.IsRunning()
}

func (a *App) ZapretRunning() bool {
	return a.zapret.GetStatus() == zapret.StatusRunning
}

// StartTrayUpdate открывает окно приложения с модалкой обновления

// и запускает скачивание. component: "ZPUI" или "zapret".

// CloseApp — «Закрыть приложение» из трея: приложение завершается, но движок

// запрета в режиме службы продолжает работать (в отличие от полного «Выхода»).
