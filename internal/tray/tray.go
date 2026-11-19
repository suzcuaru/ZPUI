package tray

import (
	"fmt"
	"time"

	"zpui/internal/config"
	"zpui/internal/logger"
	"zpui/internal/xboxdns"
	"zpui/internal/zapret"

	"fyne.io/systray"
	"golang.org/x/sys/windows"
)

// Controller — интерфейс для управления окном и приложением.
// Реализуется main.App (Wails runtime).
type Controller interface {
	GetCachedResourcePercent() int
	GetCachedUserPercent() int
	ToggleWindow()
	ShowWindow()
	Quit()
	CloseApp()
	Restart()
	ToggleProxy() error
	ProxyRunning() bool
	ZapretRunning() bool
	HasZPUIUpdateAvailable() bool
	HasZapretUpdateAvailable() bool
	StartTrayUpdate(component string)
}

type App struct {
	cfg        *config.Config
	log        *logger.Logger
	zapret     *zapret.Manager
	xboxDns    *xboxdns.Manager
	controller Controller
	version    string
	iconData   []byte

	mOpen       *systray.MenuItem
	mZapretInfo *systray.MenuItem
	mProxy      *systray.MenuItem
	mUpdateZPUI *systray.MenuItem
	mUpdateZapr *systray.MenuItem
	mRestart    *systray.MenuItem
	mClose      *systray.MenuItem
	mQuit       *systray.MenuItem
}

func New(
	cfg *config.Config,
	log *logger.Logger,
	zapretMgr *zapret.Manager,
	xboxDnsMgr *xboxdns.Manager,
	controller Controller,
	version string,
	iconData []byte,
) *App {
	return &App{
		cfg:        cfg,
		log:        log,
		zapret:     zapretMgr,
		xboxDns:    xboxDnsMgr,
		controller: controller,
		version:    version,
		iconData:   iconData,
	}
}

func (a *App) Run() error {
	onReady := func() {
		systray.SetIcon(a.iconData)

		a.mOpen = systray.AddMenuItem("Открыть ZPUI", "")
		systray.AddSeparator()

		a.mZapretInfo = systray.AddMenuItem("Запрет: ...", "")
		a.mProxy = systray.AddMenuItem("ТГ-прокси: ...", "")
		a.mUpdateZPUI = systray.AddMenuItem("Обновление ZPUI — скачать", "Скачивает установщик новой версии ZPUI")
		a.mUpdateZPUI.Hide()
		a.mUpdateZapr = systray.AddMenuItem("Обновление запрета — скачать", "Скачивает и устанавливает новую версию запрета")
		a.mUpdateZapr.Hide()
		systray.AddSeparator()

		a.mRestart = systray.AddMenuItem("Перезапустить", "")
		a.mClose = systray.AddMenuItem("Закрыть приложение", "Закрывает ZPUI; движок запрета (режим службы) продолжает работать")
		a.mQuit = systray.AddMenuItem("Выход", "Полный выход: выключает движок запрета и закрывает ZPUI")

		systray.SetOnTapped(func() {
			a.controller.ToggleWindow()
		})

		go a.handleClicks()
		go a.updateInfoLoop()
	}

	systray.Run(onReady, func() {
		a.log.Info("tray", "Tray quit")
	})
	return nil
}

func (a *App) updateInfoLoop() {
	for {
		// После спящего режима Windows иногда теряет иконку (перезапуск
		// explorer, смена DPI) — переустанавливаем её на каждом такте.
		systray.SetIcon(a.iconData)

		strategy := a.cfg.GetCurrentStrategy()
		stdPct := a.controller.GetCachedResourcePercent()

		if strategy == "" {
			strategy = "не выбрана"
		}

		// Tooltip: версия + стратегия + доступность
		tooltip := fmt.Sprintf("ZPUI v%s | %s", a.version, strategy)
		if stdPct >= 0 {
			tooltip += fmt.Sprintf(" | %d%%", stdPct)
		}
		systray.SetTooltip(tooltip)

		// Инфо-строка: Запрет
		zState := "выключен"
		if a.controller.ZapretRunning() {
			zState = "работает"
		}
		a.mZapretInfo.SetTitle(fmt.Sprintf("Запрет: %s (%s)", zState, strategy))

		// Тумблер прокси
		proxyState := "выключен"
		if a.controller.ProxyRunning() {
			proxyState = "работает"
		}
		a.mProxy.SetTitle(fmt.Sprintf("Прокси: %s", proxyState))

		// Обновления — пункт виден только если есть новая версия
		if a.controller.HasZPUIUpdateAvailable() {
			a.mUpdateZPUI.Show()
		} else {
			a.mUpdateZPUI.Hide()
		}
		if a.controller.HasZapretUpdateAvailable() {
			a.mUpdateZapr.Show()
		} else {
			a.mUpdateZapr.Hide()
		}

		time.Sleep(5 * time.Second)
	}
}

func (a *App) handleClicks() {
	for {
		select {
		case <-a.mOpen.ClickedCh:
			a.log.Info("tray", "Show window requested")
			a.controller.ShowWindow()

		case <-a.mProxy.ClickedCh:
			a.log.Info("tray", "Proxy toggle requested")
			if err := a.controller.ToggleProxy(); err != nil {
				a.log.Warn("tray", "Proxy toggle failed: "+err.Error())
			}

		case <-a.mUpdateZPUI.ClickedCh:
			a.log.Info("tray", "ZPUI update download requested")
			a.controller.StartTrayUpdate("ZPUI")

		case <-a.mUpdateZapr.ClickedCh:
			a.log.Info("tray", "Zapret update download requested")
			a.controller.StartTrayUpdate("zapret")

		case <-a.mRestart.ClickedCh:
			a.log.Info("tray", "Application restart requested")
			a.controller.Restart()
			return

		case <-a.mClose.ClickedCh:
			a.log.Info("tray", "Close app requested from tray (engine stays)")
			go a.controller.CloseApp()
			return

		case <-a.mQuit.ClickedCh:
			if confirmEngineStop() {
				a.log.Info("tray", "Full quit requested from tray")
				go a.controller.Quit()
				return
			}
			a.log.Info("tray", "Full quit cancelled by user")
		}
	}
}

// confirmEngineStop — подтверждение полного выхода: диалог предупреждает,
// что движок запрета будет выключен. true — пользователь согласился.
func confirmEngineStop() bool {
	title, _ := windows.UTF16PtrFromString("ZPUI — Выход")
	msg, _ := windows.UTF16PtrFromString(
		"Полный выход выключит движок Запрета — обход блокировок остановится.\n\n" +
			"Если нужно оставить работающий Запрет — выберите «Закрыть приложение».\n\n" +
			"Закрыть ZPUI и выключить движок?",
	)
	btn, err := windows.MessageBox(windows.HWND(0), msg, title,
		windows.MB_YESNO|windows.MB_ICONWARNING|windows.MB_TOPMOST|windows.MB_SETFOREGROUND)
	const idYes = 6
	return err == nil && btn == idYes
}
