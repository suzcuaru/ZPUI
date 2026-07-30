//go:build windows

package winprogress

import (
	_ "embed"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

//go:embed fonts/Inter-Regular.ttf
var interFontData []byte

const (
	_wsCaption = 0x00C00000
	_wsSysmenu = 0x00080000
	_wsVisible = 0x10000000

	_wsExDlgModalFrame = 0x00000001
	_wsExTopmost       = 0x00000008
	_wsExAppWindow     = 0x00040000

	_swShow = 5

	_wmDestroy     = 0x0002
	_wmClose       = 0x0010
	_wmPaint       = 0x000F
	_wmEraseBkgnd  = 0x0014
	_wmTimer       = 0x0113
	_wmSize        = 0x0005
	_wmUser        = 0x0400
	_wmSetStatus   = _wmUser + 1
	_wmSetProgress = _wmUser + 2
	_wmSetTitle    = _wmUser + 3
	_wmSetDetail   = _wmUser + 4
	_wmSetPctVis   = _wmUser + 5

	_csHredraw = 0x0002
	_csVredraw = 0x0001

	_colorWindow = 5

	_smCxScreen = 0
	_smCyScreen = 1

	_dtCenter    = 0x0001
	_dtVCenter   = 0x0004
	_dtSingleline = 0x0020
	_dtNoPrefix  = 0x0800
	_dtRight     = 0x0002

	_timerID = 1001
	_cchNull = ^uintptr(0)
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	comctl32 = syscall.NewLazyDLL("comctl32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")

	procGetModuleHandleW  = kernel32.NewProc("GetModuleHandleW")
	procRegisterClassExW  = user32.NewProc("RegisterClassExW")
	procCreateWindowExW   = user32.NewProc("CreateWindowExW")
	procDefWindowProcW    = user32.NewProc("DefWindowProcW")
	procGetMessageW       = user32.NewProc("GetMessageW")
	procTranslateMessage  = user32.NewProc("TranslateMessage")
	procDispatchMessageW  = user32.NewProc("DispatchMessageW")
	procPostMessageW      = user32.NewProc("PostMessageW")
	procSetWindowTextW    = user32.NewProc("SetWindowTextW")
	procShowWindow        = user32.NewProc("ShowWindow")
	procUpdateWindow      = user32.NewProc("UpdateWindow")
	procDestroyWindow     = user32.NewProc("DestroyWindow")
	procPostQuitMessage   = user32.NewProc("PostQuitMessage")
	procGetSystemMetrics  = user32.NewProc("GetSystemMetrics")
	procLoadIconW         = user32.NewProc("LoadIconW")
	procLoadCursorW       = user32.NewProc("LoadCursorW")
	procGetClientRect     = user32.NewProc("GetClientRect")
	procBeginPaint        = user32.NewProc("BeginPaint")
	procEndPaint          = user32.NewProc("EndPaint")
	procInvalidateRect    = user32.NewProc("InvalidateRect")
	procSetTimer          = user32.NewProc("SetTimer")
	procKillTimer         = user32.NewProc("KillTimer")
	procDrawTextW         = user32.NewProc("DrawTextW")
	procFillRect          = user32.NewProc("FillRect")
	procSendMessageW      = user32.NewProc("SendMessageW")

	procCreateSolidBrush  = gdi32.NewProc("CreateSolidBrush")
	procCreatePen         = gdi32.NewProc("CreatePen")
	procDeleteObject      = gdi32.NewProc("DeleteObject")
	procSelectObject      = gdi32.NewProc("SelectObject")
	procSetBkMode         = gdi32.NewProc("SetBkMode")
	procSetTextColor      = gdi32.NewProc("SetTextColor")
	procCreateRoundRectRgn = gdi32.NewProc("CreateRoundRectRgn")
	procSelectClipRgn     = gdi32.NewProc("SelectClipRgn")
	procCreateFontIndirectW = user32.NewProc("CreateFontIndirectW")
	procAddFontMemResource = kernel32.NewProc("AddFontMemResourceEx")
)

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  uintptr
	lpszClassName uintptr
	hIconSm       uintptr
}

type tagMSG struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	ptX     int32
	ptY     int32
}

type rect struct {
	left, top, right, bottom int32
}

type paintStruct struct {
	hdc         uintptr
	fErase      int32
	rcPaint     rect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]uint8
}

type logFont struct {
	lfHeight         int32
	lfWidth          int32
	lfEscapement     int32
	lfOrientation    int32
	lfWeight         int32
	lfItalic         byte
	lfUnderline      byte
	lfStrikeOut      byte
	lfCharSet        byte
	lfOutPrecision   byte
	lfClipPrecision  byte
	lfQuality        byte
	lfPitchAndFamily byte
	lfFaceName       [32]uint16
}

type palette struct {
	bg, title, status, detail   uint32
	track, bar1, bar2           uint32
}

func rgb(r, g, b uint8) uint32 {
	return uint32(r) | (uint32(g) << 8) | (uint32(b) << 16)
}

func lightPalette() palette {
	return palette{
		bg:     rgb(255, 255, 255),
		title:  rgb(26, 26, 34),
		status: rgb(80, 84, 98),
		detail: rgb(150, 156, 170),
		track:  rgb(228, 231, 238),
		bar1:   rgb(79, 143, 247),
		bar2:   rgb(120, 168, 252),
	}
}

func darkPalette() palette {
	return palette{
		bg:     rgb(32, 32, 42),
		title:  rgb(232, 233, 241),
		status: rgb(150, 154, 168),
		detail: rgb(96, 100, 114),
		track:  rgb(50, 50, 62),
		bar1:   rgb(107, 163, 255),
		bar2:   rgb(140, 186, 255),
	}
}

type ProgressWindow struct {
	mu            sync.Mutex
	hwnd          uintptr
	titleFont     uintptr
	textFont      uintptr
	smallFont     uintptr
	bgBrush       uintptr
	pal           palette

	title    string
	status   string
	detail   string

	targetPct   int
	displayPct  float64
	showPercent bool

	hwndReady chan struct{}
	done      chan struct{}
}

var (
	currentWindow *ProgressWindow
	callbackPtr   = syscall.NewCallback(wndProc)
	fontResOnce   sync.Once
	fontResHandle uintptr
)

func wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	w := currentWindow
	switch msg {
	case _wmSetStatus:
		invalidate(hwnd)
		return 0
	case _wmSetProgress:
		if w != nil {
			w.mu.Lock()
			w.targetPct = int(wParam)
			if w.targetPct < 0 {
				w.targetPct = 0
			}
			if w.targetPct > 100 {
				w.targetPct = 100
			}
			if w.displayPct == 0 && w.targetPct > 0 {
				w.displayPct = float64(w.targetPct) * 0.4
			}
			w.mu.Unlock()
			invalidate(hwnd)
		}
		return 0
	case _wmSetDetail:
		invalidate(hwnd)
		return 0
	case _wmSetTitle:
		if w != nil {
			w.mu.Lock()
			t := w.title
			w.mu.Unlock()
			ptr, _ := syscall.UTF16PtrFromString(t)
			procSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(ptr)))
			invalidate(hwnd)
		}
		return 0
	case _wmSetPctVis:
		if w != nil {
			w.mu.Lock()
			w.showPercent = wParam != 0
			w.mu.Unlock()
			invalidate(hwnd)
		}
		return 0
	case _wmTimer:
		if w != nil {
			w.mu.Lock()
			target := float64(w.targetPct)
			d := w.displayPct
			if d < target {
				d += (target - d) * 0.22
				if target-d < 0.4 {
					d = target
				}
			} else if d > target {
				d = target
			}
			changed := d != w.displayPct
			w.displayPct = d
			w.mu.Unlock()
			if changed {
				invalidate(hwnd)
			}
		}
		return 0
	case _wmEraseBkgnd:
		return 1
	case _wmPaint:
		w.paint(hwnd)
		return 0
	case _wmSize:
		invalidate(hwnd)
		return 0
	case _wmClose:
		return 0
	case _wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

func invalidate(hwnd uintptr) {
	procInvalidateRect.Call(hwnd, 0, 1)
}

// paint performs all custom drawing: background, title, rounded progress bar
// (centered vertically), percentage at the right end, status and detail lines.
func (w *ProgressWindow) paint(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	defer procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))

	var rc rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))

	clientW := rc.right - rc.left
	clientH := rc.bottom - rc.top

	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), w.bgBrush)

	pal := w.pal
	w.mu.Lock()
	title := w.title
	status := w.status
	detail := w.detail
	display := w.displayPct
	showPct := w.showPercent
	titleFont := w.titleFont
	textFont := w.textFont
	smallFont := w.smallFont
	w.mu.Unlock()

	pad := int32(30)
	contentW := clientW - pad*2
	if contentW < 120 {
		contentW = 120
	}
	barH := int32(14)
	centerY := clientH / 2
	barTop := centerY - barH/2
	barBottom := barTop + barH

	pctW := int32(46)
	gap := int32(12)
	barW := contentW - pctW - gap
	if showPct {
		barW = contentW - pctW - gap
	} else {
		barW = contentW
	}
	barLeft := pad
	barRight := barLeft + barW

	if title != "" {
		var prev uintptr
		if titleFont != 0 {
			prev, _, _ = procSelectObject.Call(hdc, titleFont)
		}
		procSetBkMode.Call(hdc, 1)
		procSetTextColor.Call(hdc, uintptr(pal.title))
		tr := rect{pad, barTop - 54, pad + contentW, barTop - 18}
		procDrawTextW.Call(hdc,
			uintptr(unsafe.Pointer(utf16Ptr(title))),
			uintptr(_cchNull), uintptr(unsafe.Pointer(&tr)),
			uintptr(_dtCenter|_dtVCenter|_dtSingleline|_dtNoPrefix))
		if prev != 0 {
			procSelectObject.Call(hdc, prev)
		}
	}

	drawBar(hdc, pal, barLeft, barTop, barRight, barBottom, barH, int32(display+0.5))

	if showPct {
		var prev uintptr
		if smallFont != 0 {
			prev, _, _ = procSelectObject.Call(hdc, smallFont)
		}
		procSetBkMode.Call(hdc, 1)
		procSetTextColor.Call(hdc, uintptr(pal.title))
		pct := int(display + 0.5)
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}
		txt := pctString(pct)
		pr := rect{barRight + gap, barTop - 2, barRight + gap + pctW, barBottom + 2}
		procDrawTextW.Call(hdc,
			uintptr(unsafe.Pointer(utf16Ptr(txt))),
			uintptr(_cchNull), uintptr(unsafe.Pointer(&pr)),
			uintptr(_dtRight|_dtVCenter|_dtSingleline|_dtNoPrefix))
		if prev != 0 {
			procSelectObject.Call(hdc, prev)
		}
	}

	if status != "" {
		var prev uintptr
		if textFont != 0 {
			prev, _, _ = procSelectObject.Call(hdc, textFont)
		}
		procSetBkMode.Call(hdc, 1)
		procSetTextColor.Call(hdc, uintptr(pal.status))
		sr := rect{pad, barBottom + 16, pad + contentW, barBottom + 40}
		procDrawTextW.Call(hdc,
			uintptr(unsafe.Pointer(utf16Ptr(status))),
			uintptr(_cchNull), uintptr(unsafe.Pointer(&sr)),
			uintptr(_dtCenter|_dtVCenter|_dtSingleline|_dtNoPrefix))
		if prev != 0 {
			procSelectObject.Call(hdc, prev)
		}
	}

	if detail != "" {
		var prev uintptr
		if smallFont != 0 {
			prev, _, _ = procSelectObject.Call(hdc, smallFont)
		}
		procSetBkMode.Call(hdc, 1)
		procSetTextColor.Call(hdc, uintptr(pal.detail))
		dr := rect{pad, barBottom + 38, pad + contentW, barBottom + 60}
		procDrawTextW.Call(hdc,
			uintptr(unsafe.Pointer(utf16Ptr(detail))),
			uintptr(_cchNull), uintptr(unsafe.Pointer(&dr)),
			uintptr(_dtCenter|_dtVCenter|_dtSingleline|_dtNoPrefix))
		if prev != 0 {
			procSelectObject.Call(hdc, prev)
		}
	}
}

// drawBar draws a rounded track and a vertically-gradient fill clipped to the
// track's rounded shape, so the fill always has clean rounded ends.
func drawBar(hdc uintptr, pal palette, left, top, right, bottom, barH, pct int32) {
	trackBrush, _, _ := procCreateSolidBrush.Call(uintptr(pal.track))
	penNull, _, _ := procCreatePen.Call(0, 0, uintptr(pal.track))
	prevPen, _, _ := procSelectObject.Call(hdc, penNull)
	prevBrush, _, _ := procSelectObject.Call(hdc, trackBrush)

	rgn, _, _ := procCreateRoundRectRgn.Call(
		uintptr(left), uintptr(top),
		uintptr(right+1), uintptr(bottom+1),
		uintptr(barH), uintptr(barH))
	procSelectClipRgn.Call(hdc, rgn)

	trackRect := rect{left, top, right + 1, bottom + 1}
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&trackRect)), trackBrush)

	fillW := int32(float64(right-left) * float64(pct) / 100.0)
	if fillW > 0 {
		var c1r, c1g, c1b uint8 = channel(pal.bar1)
		var c2r, c2g, c2b uint8 = channel(pal.bar2)
		for y := int32(0); y < barH; y++ {
			t := float64(y) / float64(barH)
			r := lerpU8(c1r, c2r, t)
			g := lerpU8(c1g, c2g, t)
			b := lerpU8(c1b, c2b, t)
			br, _, _ := procCreateSolidBrush.Call(uintptr(rgb(r, g, b)))
			line := rect{left, top + y, left + fillW + 1, top + y + 1}
			procFillRect.Call(hdc, uintptr(unsafe.Pointer(&line)), br)
			procDeleteObject.Call(br)
		}
	}

	procSelectClipRgn.Call(hdc, 0)
	procDeleteObject.Call(rgn)
	procSelectObject.Call(hdc, prevBrush)
	procSelectObject.Call(hdc, prevPen)
	procDeleteObject.Call(trackBrush)
	procDeleteObject.Call(penNull)
}

func channel(c uint32) (uint8, uint8, uint8) {
	return uint8(c), uint8(c >> 8), uint8(c >> 16)
}

func lerpU8(a, b uint8, t float64) uint8 {
	return uint8(float64(a) + (float64(b)-float64(a))*t)
}

func pctString(pct int) string {
	return itoa(pct) + "%"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func utf16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	if p == nil {
		p = &emptyUTF16
	}
	return p
}

var emptyUTF16 uint16

func registerClass(hInstance uintptr, className string) bool {
	clsName, _ := syscall.UTF16PtrFromString(className)
	icon, _, _ := procLoadIconW.Call(0, uintptr(32512))
	cursor, _, _ := procLoadCursorW.Call(0, uintptr(32512))

	wc := wndClassEx{
		cbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		style:         _csHredraw | _csVredraw,
		lpfnWndProc:   callbackPtr,
		hInstance:     hInstance,
		hIcon:         icon,
		hCursor:       cursor,
		hbrBackground: 0,
		lpszClassName: uintptr(unsafe.Pointer(clsName)),
		hIconSm:       icon,
	}

	ret, _, _ := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	return ret != 0
}

// isDarkTheme reads the Windows registry to detect dark mode (AppsUseLightTheme).
func isDarkTheme() bool {
	var hKey uintptr
	root, _ := syscall.UTF16PtrFromString(`SOFTWARE\Microsoft\Windows\CurrentVersion\Themes\Personalize`)
	regOpenKey := advapi32.NewProc("RegOpenKeyExW")
	regQuery := advapi32.NewProc("RegQueryValueExW")
	regClose := advapi32.NewProc("RegCloseKey")
	hkRoot := uintptr(0x80000001)
	ret, _, _ := regOpenKey.Call(hkRoot, uintptr(unsafe.Pointer(root)), 0, 0x20019, uintptr(unsafe.Pointer(&hKey)))
	if ret != 0 {
		return false
	}
	defer regClose.Call(hKey)
	valName, _ := syscall.UTF16PtrFromString("AppsUseLightTheme")
	var data [4]byte
	var dataSize uint32 = 4
	var valType uint32
	ret, _, _ = regQuery.Call(hKey,
		uintptr(unsafe.Pointer(valName)),
		uintptr(unsafe.Pointer(&valType)),
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(unsafe.Pointer(&dataSize)))
	if ret != 0 {
		return false
	}
	return data[0] == 0
}

func (w *ProgressWindow) ensureFontResource() {
	fontResOnce.Do(func() {
		if len(interFontData) == 0 {
			return
		}
		nFonts := uint32(0)
		handle, _, _ := procAddFontMemResource.Call(
			uintptr(unsafe.Pointer(&interFontData[0])),
			uintptr(len(interFontData)),
			0,
			uintptr(unsafe.Pointer(&nFonts)))
		fontResHandle = handle
	})
}

func makeFont(height, weight int32) uintptr {
	w := currentWindow
	if w != nil {
		w.ensureFontResource()
	}
	lf := logFont{
		lfHeight:       height,
		lfWeight:       weight,
		lfCharSet:      1,
		lfOutPrecision: 0,
		lfClipPrecision: 0,
		lfQuality:      5,
		lfPitchAndFamily: 0x22,
	}
	if fontResHandle != 0 {
		setFaceName(&lf, "Inter")
	} else {
		setFaceName(&lf, "Segoe UI")
	}
	f, _, _ := procCreateFontIndirectW.Call(uintptr(unsafe.Pointer(&lf)))
	return f
}

func setFaceName(lf *logFont, name string) {
	runes := []rune(name)
	n := len(runes)
	if n > 31 {
		n = 31
	}
	for i := 0; i < n; i++ {
		lf.lfFaceName[i] = uint16(runes[i])
	}
}

func (w *ProgressWindow) run(title string) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hInst, _, _ := procGetModuleHandleW.Call(0)

	className := "ZPUIProgressWindow"
	registerClass(hInst, className)

	clsName, _ := syscall.UTF16PtrFromString(className)
	titlePtr, _ := syscall.UTF16PtrFromString(title)

	cxScreen, _, _ := procGetSystemMetrics.Call(_smCxScreen)
	cyScreen, _, _ := procGetSystemMetrics.Call(_smCyScreen)

	winW := uintptr(480)
	winH := uintptr(250)
	x := (int32(cxScreen) - int32(winW)) / 2
	y := (int32(cyScreen) - int32(winH)) / 2

	style := uintptr(_wsCaption | _wsSysmenu | _wsVisible)
	exStyle := uintptr(_wsExDlgModalFrame | _wsExTopmost | _wsExAppWindow)

	hwnd, _, _ := procCreateWindowExW.Call(
		exStyle,
		uintptr(unsafe.Pointer(clsName)),
		uintptr(unsafe.Pointer(titlePtr)),
		style,
		uintptr(x), uintptr(y), winW, winH,
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		close(w.hwndReady)
		return
	}

	if isDarkTheme() {
		w.pal = darkPalette()
	} else {
		w.pal = lightPalette()
	}
	w.bgBrush, _, _ = procCreateSolidBrush.Call(uintptr(w.pal.bg))

	w.ensureFontResource()
	w.titleFont = makeFont(-22, 700)
	w.textFont = makeFont(-15, 400)
	w.smallFont = makeFont(-14, 600)

	w.mu.Lock()
	w.hwnd = hwnd
	w.title = title
	w.mu.Unlock()

	procSetTimer.Call(hwnd, _timerID, 16, 0)

	procShowWindow.Call(hwnd, _swShow)
	procUpdateWindow.Call(hwnd)

	close(w.hwndReady)

	var m tagMSG
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if ret == 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}

	procKillTimer.Call(hwnd, _timerID)
	if w.titleFont != 0 {
		procDeleteObject.Call(w.titleFont)
	}
	if w.textFont != 0 {
		procDeleteObject.Call(w.textFont)
	}
	if w.smallFont != 0 {
		procDeleteObject.Call(w.smallFont)
	}
	if w.bgBrush != 0 {
		procDeleteObject.Call(w.bgBrush)
	}

	close(w.done)
}

func New(title string) *ProgressWindow {
	w := &ProgressWindow{
		hwndReady:    make(chan struct{}),
		done:         make(chan struct{}),
		showPercent:  true,
		pal:          lightPalette(),
	}

	currentWindow = w

	go w.run(title)

	<-w.hwndReady
	return w
}

func (w *ProgressWindow) SetStatus(text string) {
	w.mu.Lock()
	w.status = text
	w.mu.Unlock()
	if w.hwnd != 0 {
		procPostMessageW.Call(w.hwnd, _wmSetStatus, 0, 0)
	}
}

func (w *ProgressWindow) SetDetail(text string) {
	w.mu.Lock()
	w.detail = text
	w.mu.Unlock()
	if w.hwnd != 0 {
		procPostMessageW.Call(w.hwnd, _wmSetDetail, 0, 0)
	}
}

func (w *ProgressWindow) SetProgress(percent int) {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	w.mu.Lock()
	w.targetPct = percent
	w.mu.Unlock()
	if w.hwnd != 0 {
		procPostMessageW.Call(w.hwnd, _wmSetProgress, uintptr(percent), 0)
	}
}

func (w *ProgressWindow) SetPercentVisible(visible bool) {
	w.mu.Lock()
	w.showPercent = visible
	w.mu.Unlock()
	if w.hwnd != 0 {
		v := uintptr(0)
		if visible {
			v = 1
		}
		procPostMessageW.Call(w.hwnd, _wmSetPctVis, v, 0)
	}
}

func (w *ProgressWindow) SetTitle(title string) {
	w.mu.Lock()
	w.title = title
	w.mu.Unlock()
	if w.hwnd != 0 {
		procPostMessageW.Call(w.hwnd, _wmSetTitle, 0, 0)
	}
}

func (w *ProgressWindow) Close() {
	if w.hwnd != 0 {
		procDestroyWindow.Call(w.hwnd)
	}
}

func (w *ProgressWindow) WaitClosed() {
	<-w.done
}
