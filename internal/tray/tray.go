package tray

import (
	"encoding/binary"
	"fmt"
	"os"
	"time"

	"zpui/internal/config"
	"zpui/internal/logger"
	"zpui/internal/proxy"
	"zpui/internal/web"
	"zpui/internal/window"
	"zpui/internal/zapret"

	"fyne.io/systray"
)

type App struct {
	cfg     *config.Config
	log     *logger.Logger
	zapret  *zapret.Manager
	proxy   *proxy.SOCKS5Server
	web     *web.Server
	version string

	mTitle     *systray.MenuItem
	mZapret    *systray.MenuItem
	mProxy     *systray.MenuItem
	mStrategy  *systray.MenuItem
	mResource  *systray.MenuItem
	mPanel     *systray.MenuItem
	mRestart   *systray.MenuItem
	mQuit      *systray.MenuItem
}

func New(
	cfg *config.Config,
	log *logger.Logger,
	zapretMgr *zapret.Manager,
	proxySrv *proxy.SOCKS5Server,
	webSrv *web.Server,
	version string,
) *App {
	return &App{
		cfg:     cfg,
		log:     log,
		zapret:  zapretMgr,
		proxy:   proxySrv,
		web:     webSrv,
		version: version,
	}
}

func (a *App) Run() error {
	onReady := func() {
		systray.SetIcon(createIcon())
		systray.SetTooltip("ZPUI — Управление Zapret")

		a.mTitle = systray.AddMenuItem("ZPUI", "")
		a.mTitle.Disable()
		systray.AddSeparator()

		// Left-click on tray icon → toggle window
		systray.SetOnTapped(func() {
			url := a.web.GetURL()
			if url != "" {
				window.Toggle(url)
			}
		})

		// Status items (non-clickable)
		a.mZapret = systray.AddMenuItem("● Запрет: проверка...", "")
		a.mZapret.Disable()
		a.mProxy = systray.AddMenuItem("● Прокси: проверка...", "")
		a.mProxy.Disable()
		a.mStrategy = systray.AddMenuItem("Стратегия: ...", "")
		a.mStrategy.Disable()
		a.mResource = systray.AddMenuItem("Доступность: ...", "")
		a.mResource.Disable()
		systray.AddSeparator()

		// Actions
		a.mPanel = systray.AddMenuItem("📊 Показать/Скрыть окно", "")
		a.mRestart = systray.AddMenuItem("🔄 Перезапустить запрет", "")
		systray.AddSeparator()

		a.mQuit = systray.AddMenuItem("❌ Выход", "")

		go a.updateLoop()
		go a.handleClicks()

		// Open window with delay to let web UI render
		go func() {
			time.Sleep(1500 * time.Millisecond)
			url := a.web.GetURL()
			if url != "" {
				window.Open(url)
			}
		}()
	}
	systray.Run(onReady, func() {
		a.log.Info("tray", "Завершение работы")
		window.Close()
		a.proxy.Stop()
		a.web.Stop()
		os.Exit(0)
	})
	return nil
}

func (a *App) updateLoop() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		zStatus := string(a.zapret.GetStatus())
		pRunning := a.proxy.IsRunning()

		// Zapret status — clickable toggle
		if zStatus == "running" {
			a.mZapret.SetTitle("✅ Запрет: Работает (нажмите чтобы остановить)")
			a.mZapret.Enable()
			a.mRestart.Enable()
		} else {
			a.mZapret.SetTitle("⛔ Запрет: Остановлен (нажмите чтобы запустить)")
			a.mZapret.Enable()
			a.mRestart.Disable()
		}

		// Proxy status — clickable toggle
		if pRunning {
			a.mProxy.SetTitle(fmt.Sprintf("✅ Прокси: Работает :%d (нажмите чтобы остановить)", a.cfg.GetProxyConfig().Port))
			a.mProxy.Enable()
		} else {
			a.mProxy.SetTitle("⛔ Прокси: Остановлен (нажмите чтобы запустить)")
			a.mProxy.Enable()
		}

		// Strategy
		strategy := a.zapret.GetCurrentStrategy()
		if strategy == "" {
			strategy = "не выбрана"
		}
		a.mStrategy.SetTitle(fmt.Sprintf("📋 Стратегия: %s", strategy))

		// Resource %
		pct := a.web.GetCachedResourcePercent()
		resText := "📊 Доступность: ..."
		if pct >= 0 {
			if pct >= 80 {
				resText = fmt.Sprintf("📊 Доступность: %d%% ✅", pct)
			} else if pct >= 50 {
				resText = fmt.Sprintf("📊 Доступность: %d%% ⚠️", pct)
			} else {
				resText = fmt.Sprintf("📊 Доступность: %d%% ❌", pct)
			}
		}
		a.mResource.SetTitle(resText)
	}
}

func (a *App) handleClicks() {
	for {
		select {
		case <-a.mZapret.ClickedCh:
			// Toggle: if running → stop, if stopped → start
			zStatus := string(a.zapret.GetStatus())
			if zStatus == "running" {
				a.zapret.Stop()
			} else {
				if err := a.zapret.Start(); err != nil {
					a.log.Error("tray", fmt.Sprintf("Ошибка: %v", err))
				}
			}
		case <-a.mProxy.ClickedCh:
			// Toggle: if running → stop, if stopped → start
			if a.proxy.IsRunning() {
				a.proxy.Stop()
			} else {
				if err := a.proxy.Start(); err != nil {
					a.log.Error("tray", fmt.Sprintf("Ошибка прокси: %v", err))
				}
			}
		case <-a.mPanel.ClickedCh:
			url := a.web.GetURL()
			if url != "" {
				window.Toggle(url)
			}
		case <-a.mRestart.ClickedCh:
			if err := a.zapret.Restart(); err != nil {
				a.log.Error("tray", fmt.Sprintf("Ошибка перезапуска: %v", err))
			}
		case <-a.mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

func createIcon() []byte {
	const w = 16
	const h = 16
	const bpp = 32
	const pixelBytes = w * h * 4
	const maskRowSize = ((w + 31) / 32) * 4
	const maskBytes = maskRowSize * h
	const bmpHeaderSize = 40
	const imageDataSize = bmpHeaderSize + pixelBytes + maskBytes
	const headerOffset = 6 + 16

	icondir := make([]byte, 6)
	binary.LittleEndian.PutUint16(icondir[0:2], 0)
	binary.LittleEndian.PutUint16(icondir[2:4], 1)
	binary.LittleEndian.PutUint16(icondir[4:6], 1)

	entry := make([]byte, 16)
	entry[0] = byte(w)
	entry[1] = byte(h)
	entry[2] = 0
	entry[3] = 0
	binary.LittleEndian.PutUint16(entry[4:6], 1)
	binary.LittleEndian.PutUint16(entry[6:8], bpp)
	binary.LittleEndian.PutUint32(entry[8:12], imageDataSize)
	binary.LittleEndian.PutUint32(entry[12:16], headerOffset)

	bmpHeader := make([]byte, bmpHeaderSize)
	binary.LittleEndian.PutUint32(bmpHeader[0:4], bmpHeaderSize)
	binary.LittleEndian.PutUint32(bmpHeader[4:8], w)
	binary.LittleEndian.PutUint32(bmpHeader[8:12], h*2)
	binary.LittleEndian.PutUint16(bmpHeader[12:14], 1)
	binary.LittleEndian.PutUint16(bmpHeader[14:16], bpp)
	binary.LittleEndian.PutUint32(bmpHeader[20:24], pixelBytes+maskBytes)

	pixels := make([]byte, pixelBytes)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dstY := h - 1 - y
			idx := (dstY*w + x) * 4
			dx := x - w/2
			dy := y - h/2
			dist := dx*dx + dy*dy
			if dist < (w/2-1)*(w/2-1) {
				if dist < 9 {
					pixels[idx+0] = 0xFF
					pixels[idx+1] = 0xD4
					pixels[idx+2] = 0x00
					pixels[idx+3] = 0xFF
				} else {
					pixels[idx+0] = 0x20
					pixels[idx+1] = 0x10
					pixels[idx+2] = 0x10
					pixels[idx+3] = 0xFF
				}
			} else {
				pixels[idx+3] = 0x00
			}
		}
	}

	mask := make([]byte, maskBytes)
	result := make([]byte, 0, headerOffset+imageDataSize)
	result = append(result, icondir...)
	result = append(result, entry...)
	result = append(result, bmpHeader...)
	result = append(result, pixels...)
	result = append(result, mask...)
	return result
}