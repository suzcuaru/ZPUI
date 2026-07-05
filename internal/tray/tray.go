package tray

import (
	"encoding/binary"
	"fmt"
	"time"

	"zpui/internal/config"
	"zpui/internal/logger"
	"zpui/internal/proxy"
	"zpui/internal/zapret"

	"fyne.io/systray"
)

// Controller — интерфейс для управления окном и приложением.
// Реализуется main.App (Wails runtime).
type Controller interface {
	GetCachedResourcePercent() int
	ToggleWindow()
	ShowWindow()
	Quit()
}

type App struct {
	cfg        *config.Config
	log        *logger.Logger
	zapret     *zapret.Manager
	proxy      *proxy.SOCKS5Server
	controller Controller
	version    string

	mOpen     *systray.MenuItem
	mRestart  *systray.MenuItem
	mQuit     *systray.MenuItem
}

func New(
	cfg *config.Config,
	log *logger.Logger,
	zapretMgr *zapret.Manager,
	proxySrv *proxy.SOCKS5Server,
	controller Controller,
	version string,
) *App {
	return &App{
		cfg:        cfg,
		log:        log,
		zapret:     zapretMgr,
		proxy:      proxySrv,
		controller: controller,
		version:    version,
	}
}

func (a *App) Run() error {
	onReady := func() {
		systray.SetIcon(createIcon())
		systray.SetTooltip(fmt.Sprintf("ZPUI v%s", a.version))

		a.mOpen = systray.AddMenuItem("Открыть ZPUI", "")
		a.mRestart = systray.AddMenuItem("Перезапустить", "")
		systray.AddSeparator()
		a.mQuit = systray.AddMenuItem("Выход", "")

		systray.SetOnTapped(func() {
			a.controller.ToggleWindow()
		})

		go a.handleClicks()

		go func() {
			time.Sleep(800 * time.Millisecond)
			if !a.cfg.StartMinimized {
				a.controller.ShowWindow()
			}
		}()
	}

	systray.Run(onReady, func() {
		a.log.Info("tray", "Tray quit")
		a.proxy.Stop()
	})
	return nil
}

func (a *App) handleClicks() {
	for {
		select {
		case <-a.mOpen.ClickedCh:
			a.log.Info("tray", "Show window requested")
			a.controller.ShowWindow()
		case <-a.mRestart.ClickedCh:
			a.log.Info("tray", "Restart requested")
			go func() {
				a.proxy.Stop()
				a.zapret.Stop()
				time.Sleep(2 * time.Second)
				if a.cfg.LastProxyState {
					a.proxy.Start()
				}
				if a.cfg.LastZapretState {
					a.zapret.Start()
				}
			}()
		case <-a.mQuit.ClickedCh:
			a.log.Info("tray", "Quit requested from tray")
			a.controller.Quit()
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

			bgR, bgG, bgB, bgA := byte(0x6C), byte(0x5C), byte(0xE7), byte(0xFF)
			fgR, fgG, fgB, fgA := byte(0xFF), byte(0xFF), byte(0xFF), byte(0xFF)

			if !isInRoundedSquare(x, y, w, h, 3) {
				bgA = 0x00
				fgA = 0x00
			}

			r, g, b, a := bgR, bgG, bgB, bgA
			if isZPixel(x, y) {
				r, g, b, a = fgR, fgG, fgB, fgA
			}

			pixels[idx+0] = b
			pixels[idx+1] = g
			pixels[idx+2] = r
			pixels[idx+3] = a
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

func isInRoundedSquare(x, y, w, h, r int) bool {
	if x < r && y < r {
		dx := r - x
		dy := r - y
		return dx*dx+dy*dy <= r*r
	}
	if x >= w-r && y < r {
		dx := x - (w - r - 1)
		dy := r - y
		return dx*dx+dy*dy <= r*r
	}
	if x < r && y >= h-r {
		dx := r - x
		dy := y - (h - r - 1)
		return dx*dx+dy*dy <= r*r
	}
	if x >= w-r && y >= h-r {
		dx := x - (w - r - 1)
		dy := y - (h - r - 1)
		return dx*dx+dy*dy <= r*r
	}
	return true
}

func isZPixel(x, y int) bool {
	if x < 4 || x > 11 {
		return false
	}
	if y >= 3 && y <= 4 {
		return true
	}
	if y >= 11 && y <= 12 {
		return true
	}
	switch y {
	case 5:
		return x >= 8 && x <= 11
	case 6:
		return x >= 7 && x <= 10
	case 7:
		return x >= 6 && x <= 9
	case 8:
		return x >= 5 && x <= 8
	case 9:
		return x >= 4 && x <= 7
	case 10:
		return x >= 4 && x <= 6
	}
	return false
}
