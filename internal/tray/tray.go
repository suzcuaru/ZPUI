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

	"github.com/getlantern/systray"
)

type App struct {
	cfg     *config.Config
	log     *logger.Logger
	zapret  *zapret.Manager
	proxy   *proxy.SOCKS5Server
	web     *web.Server
	version string
	visible bool

	mZapretStatus *systray.MenuItem
	mProxyStatus  *systray.MenuItem
	mStart        *systray.MenuItem
	mStop         *systray.MenuItem
	mRestart      *systray.MenuItem
	mPanel        *systray.MenuItem
	mConsole      *systray.MenuItem
	mQuit         *systray.MenuItem
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
		visible: false,
	}
}

func (a *App) Run() error {
	onReady := func() {
		systray.SetIcon(createIcon())
		systray.SetTooltip(fmt.Sprintf("ZPUI v%s", a.version))

		systray.AddMenuItem(fmt.Sprintf("ZPUI v%s | Zapret v%s", a.version, a.zapret.GetVersion()), "")
		systray.AddSeparator()

		a.mZapretStatus = systray.AddMenuItem("Запрет: ...", "")
		a.mZapretStatus.Disable()
		a.mProxyStatus = systray.AddMenuItem("Прокси: ...", "")
		a.mProxyStatus.Disable()
		systray.AddSeparator()

		a.mPanel = systray.AddMenuItem("Открыть панель", "")
		a.mStart = systray.AddMenuItem("Запустить", "")
		a.mStop = systray.AddMenuItem("Остановить", "")
		a.mRestart = systray.AddMenuItem("Перезапустить", "")
		systray.AddSeparator()

		a.mConsole = systray.AddMenuItem("Консоль", "")
		a.mQuit = systray.AddMenuItem("Выход", "")

		go a.updateLoop()
		go a.handleClicks()

		go func() {
			time.Sleep(500 * time.Millisecond)
			window.Open(a.cfg.Web.Port)
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

		zText := "Запрет: Остановлен"
		if zStatus == "running" {
			zText = "Запрет: Работает"
		}
		pText := "Прокси: Остановлен"
		if pRunning {
			pText = fmt.Sprintf("Прокси: Работает (:%d)", a.cfg.GetProxyConfig().Port)
		}

		a.mZapretStatus.SetTitle(zText)
		a.mProxyStatus.SetTitle(pText)

		if zStatus == "running" {
			a.mStart.Disable()
			a.mStop.Enable()
			a.mRestart.Enable()
		} else {
			a.mStart.Enable()
			a.mStop.Disable()
			a.mRestart.Disable()
		}
		if a.visible {
			a.mConsole.SetTitle("Скрыть консоль")
		} else {
			a.mConsole.SetTitle("Показать консоль")
		}
	}
}

func (a *App) handleClicks() {
	for {
		select {
		case <-a.mPanel.ClickedCh:
			window.Open(a.cfg.Web.Port)
		case <-a.mStart.ClickedCh:
			if err := a.zapret.Start(); err != nil {
				a.log.Error("tray", fmt.Sprintf("Ошибка: %v", err))
			}
		case <-a.mStop.ClickedCh:
			a.zapret.Stop()
		case <-a.mRestart.ClickedCh:
			if err := a.zapret.Restart(); err != nil {
				a.log.Error("tray", fmt.Sprintf("Ошибка: %v", err))
			}
		case <-a.mConsole.ClickedCh:
			if a.visible {
				HideConsole()
				a.visible = false
			} else {
				ShowConsole()
				a.visible = true
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
